//go:generate mockgen -source=beacon.go -destination=../internal/mock/pkg/beacon.go -package=mock_relay
package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"
	"github.com/r3labs/sse"
	"golang.org/x/sync/errgroup"
)

var (
	SlotsPerEpoch    Slot         = 32
	DurationPerSlot               = time.Second * 12
	DurationPerEpoch              = DurationPerSlot * time.Duration(SlotsPerEpoch)
	_                BeaconClient = (*beaconClient)(nil) // type constraint
	ErrOldSlot                    = errors.New("old slot")
)

type BeaconClient interface {
	SubscribeToHeadEvents(context.Context) <-chan HeadEvent
	GetProposerDuties(Epoch) (*RegisteredProposersResponse, error)
	SyncStatus() (*SyncStatusPayloadData, error)
	UpdateProposerDuties(context.Context, Slot, Datastore) error
	ProcessNewSlot(context.Context, Slot, Datastore) error
	GetProposerByIndex(uint64) (types.PubkeyHex, error)
	GetValidatorsMap() BuilderGetValidatorsResponseEntrySlice
	IsValidator(PubKey) bool
	HeadSlot() Slot
}

type ValidatorData struct {
	PubKey       PubKey
	FeeRecipient types.Address `json:"feeRecipient"`
	GasLimit     uint64        `json:"gasLimit"`
	Timestamp    uint64        `json:"timestamp"`
}

type beaconClient struct {
	beaconEndpoint *url.URL
	Config

	proposerDutiesSlot Slot

	slotProposerMap             map[uint64]PubKey
	registeredProposersLock     sync.RWMutex
	registeredProposersResponse BuilderGetValidatorsResponseEntrySlice

	headSlot     Slot
	currentEpoch Epoch

	index      atomic.Value
	validators atomic.Value
	lastUpdate time.Time
}

func NewBeaconClient(config Config) (BeaconClient, error) {
	u, err := url.Parse(config.BeaconEndpoint)

	bc := &beaconClient{
		beaconEndpoint:  u,
		Config:          config,
		slotProposerMap: make(map[uint64]PubKey),
	}

	bc.validators.Store(make(map[types.PubkeyHex]struct{}))
	bc.index.Store(make(map[uint64]types.PubkeyHex))

	return bc, err
}

func (b *beaconClient) IsValidator(pubkey PubKey) bool {
	knownValidators := b.knownValidators()
	_, ok := knownValidators[pubkey.PublicKey.PubkeyHex()]
	if !ok && !b.Config.CheckKnownValidator {
		b.Log.
			WithField("pubkey", pubkey).
			WithField("validatorsNum", len(knownValidators)).
			Debug("known validator check will fail")
	}
	return ok
}

func (b *beaconClient) knownValidators() map[types.PubkeyHex]struct{} {
	return b.validators.Load().(map[types.PubkeyHex]struct{})
}

func (b *beaconClient) knownValidatorsByIndex() map[uint64]types.PubkeyHex {
	return b.index.Load().(map[uint64]types.PubkeyHex)
}

// Returned channel automatically closes when underlying beacon client disconnects.
func (b *beaconClient) SubscribeToHeadEvents(ctx context.Context) <-chan HeadEvent {
	slotC := make(chan HeadEvent)
	eventsURL := fmt.Sprintf("%s/eth/v1/events?topics=head", b.beaconEndpoint.String())
	client := sse.NewClient(eventsURL)
	go func() {
		defer close(slotC)
		var head HeadEvent

		for {
			err := client.SubscribeRawWithContext(ctx, func(msg *sse.Event) {

				if err := json.Unmarshal(msg.Data, &head); err != nil {
					b.Log.WithError(err).Debug("event unmarshall failed")
					return
				}
				select {
				case <-ctx.Done():
					return
				case slotC <- head:
					b.Log.
						WithField("beaconEndpoint", b.beaconEndpoint.String()).
						Debug("read head subscription")
				}
			})

			if err != nil {
				b.Log.WithError(err).Debug("beacon subscription failed")
				return
			}
		}
	}()

	return slotC
}

func (b *beaconClient) GetProposerByIndex(index uint64) (types.PubkeyHex, error) {
	pk, ok := b.knownValidatorsByIndex()[index]
	if !ok {
		return types.PubkeyHex(""), errors.New("missing index")
	}
	return pk, nil
}

// Returns proposer duties for every slot in this epoch
func (b *beaconClient) GetProposerDuties(epoch Epoch) (*RegisteredProposersResponse, error) {
	u := *b.beaconEndpoint
	// https://ethereum.github.io/beacon-APIs/#/Validator/getProposerDuties
	u.Path = fmt.Sprintf("/eth/v1/validator/duties/proposer/%d", epoch)
	resp := new(RegisteredProposersResponse)
	err := fetchBeacon(&u, "GET", resp)
	return resp, err
}

func (b *beaconClient) HeadSlot() Slot {
	return Slot(atomic.LoadUint64((*uint64)(&b.headSlot)))
}

func (b *beaconClient) setHeadSlot(headSlot Slot) {
	atomic.StoreUint64((*uint64)(&b.headSlot), uint64(headSlot))
}

var ErrHTTPErrorResponse = errors.New("got an HTTP error response")

func fetchBeacon(u *url.URL, method string, dst any) error {
	req, err := http.NewRequest(method, u.String(), nil)
	if err != nil {
		return fmt.Errorf("invalid request for %s: %w", u, err)
	}
	req.Header.Set("accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("client refused for %s: %w", u, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("could not read response body for %s: %w", u, err)
	}

	if resp.StatusCode >= 300 {
		ec := &struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{}
		if err = json.Unmarshal(bodyBytes, ec); err != nil {
			return fmt.Errorf("could not unmarshal error response from beacon node for %s from %s: %w", u, string(bodyBytes), err)
		}
		return fmt.Errorf("%w: %s", ErrHTTPErrorResponse, ec.Message)
	}

	err = json.Unmarshal(bodyBytes, dst)
	if err != nil {
		return fmt.Errorf("could not unmarshal response for %s from %s: %w", u, string(bodyBytes), err)
	}

	return nil
}

// SyncStatusPayload is the response payload for /eth/v1/node/syncing
type SyncStatusPayload struct {
	Data SyncStatusPayloadData
}

type SyncStatusPayloadData struct {
	HeadSlot  uint64 `json:"head_slot,string"`
	IsSyncing bool   `json:"is_syncing"`
}

// SyncStatus returns the current node sync-status
func (b *beaconClient) SyncStatus() (*SyncStatusPayloadData, error) {
	u := *b.beaconEndpoint
	// https://ethereum.github.io/beacon-APIs/#/ValidatorRequiredApi/getSyncingStatus
	u.Path = "/eth/v1/node/syncing"
	resp := new(SyncStatusPayload)
	err := fetchBeacon(&u, "GET", resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Update list of proposers registered with builder network for current and next epoch
func (b *beaconClient) UpdateProposerDuties(ctx context.Context, headSlot Slot, store Datastore) error {
	logger := b.Log.WithField("method", "UpdateProposerDuties")
	timeStart := time.Now()

	b.registeredProposersLock.Lock()
	defer b.registeredProposersLock.Unlock()

	epoch := headSlot.Epoch()

	// Query current epoch
	current, err := b.GetProposerDuties(epoch)
	if err != nil {
		return fmt.Errorf("current epoch: get proposer duties: %w", err)
	}
	entries := current.Data

	// Query next epoch
	next, err := b.GetProposerDuties(epoch + 1)
	if err != nil {
		return fmt.Errorf("next epoch: get proposer duties: %w", err)
	}
	entries = append(entries, next.Data...)

	b.registeredProposersResponse = b.registeredProposersResponse[:0]
	b.proposerDutiesSlot = headSlot
	// intersect registered validators with block proposers for current and next epoch
	for _, e := range entries {
		reg, err := store.GetRegistration(ctx, e.PubKey)
		if err == nil {
			b.registeredProposersResponse = append(b.registeredProposersResponse, types.BuilderGetValidatorsResponseEntry{
				Slot:  e.Slot,
				Entry: &reg,
			})
		}
	}

	logger.With(log.F{
		"epochFrom":        epoch,
		"epochTo":          epoch + 1,
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
	}).With(b.registeredProposersResponse).Debug("proposer duties updated")
	return nil
}

func (b *beaconClient) ProcessNewSlot(ctx context.Context, headSlot Slot, store Datastore) error {
	logger := b.Log.WithField("method", "ProcessNewSlot")
	timeStart := time.Now()

	current := b.HeadSlot()
	if headSlot <= current {
		return ErrOldSlot
	}

	if current > 0 {
		for s := b.headSlot + 1; s < headSlot; s++ {
			b.Log.Warnf("missedSlot %d", s)
		}
	}

	b.setHeadSlot(headSlot)
	b.currentEpoch = headSlot.Epoch()
	logger.With(log.F{
		"epoch":              b.currentEpoch,
		"slotHead":           headSlot,
		"slotStartNextEpoch": Slot(b.currentEpoch+1) * SlotsPerEpoch,
	},
	).Infof("updated headSlot to %d", headSlot)

	// update in the background
	g := new(errgroup.Group)

	g.Go(func() error {
		return b.updateKnownValidators(Slot(headSlot))
	})

	g.Go(func() error {
		return b.UpdateProposerDuties(ctx, headSlot, store)
	})

	g.Wait()

	logger.With(log.F{
		"epoch":              b.currentEpoch,
		"slotHead":           headSlot,
		"slotStartNextEpoch": Slot(b.currentEpoch+1) * SlotsPerEpoch,
		"slot":               uint64(headSlot),
		"processingTimeMs":   time.Since(timeStart).Milliseconds(),
	}).Info("updated head slot")

	return nil

}

func (b *beaconClient) GetValidatorsMap() BuilderGetValidatorsResponseEntrySlice {
	b.registeredProposersLock.RLock()
	defer b.registeredProposersLock.RUnlock()
	return b.registeredProposersResponse
}

func (b *beaconClient) updateKnownValidators(headSlot Slot) error {
	logger := b.Log.WithField("method", "UpdateProposerDuties")
	timeStart := time.Now()

	if time.Since(b.lastUpdate) < (DurationPerEpoch / 2) { // only update every half DurationPerEpoch
		return nil
	}

	u := *b.beaconEndpoint
	u.Path = fmt.Sprintf("/eth/v1/beacon/states/%d/validators", headSlot)
	q := u.Query()
	q.Add("status", "active,pending")
	u.RawQuery = q.Encode()

	vd := new(AllValidatorsResponse)
	err := fetchBeacon(&u, "GET", vd)
	if err != nil {
		return err
	}

	validators := make(map[types.PubkeyHex]struct{})
	index := make(map[uint64]types.PubkeyHex)
	for _, vs := range vd.Data {
		validators[types.NewPubkeyHex(vs.Validator.Pubkey)] = struct{}{}
		index[vs.Index] = types.PubkeyHex(vs.Validator.Pubkey)
	}

	b.validators.Store(validators)
	b.index.Store(index)
	b.lastUpdate = time.Now()

	logger.With(log.F{
		"numValidators":    len(validators),
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
	}).Debug("updated known validators")

	return nil
}

// RegisteredProposersResponse is received when querying proposer duties
type RegisteredProposersResponse struct {
	Data []RegisteredProposersResponseData
}

type RegisteredProposersResponseData struct {
	PubKey PubKey `json:"pubkey"`
	Slot   uint64 `json:"slot,string"`
}

// AllValidatorsResponse is received when querying the active validators
type AllValidatorsResponse struct {
	Data []ValidatorResponseEntry
}

type ValidatorResponseEntry struct {
	Index     uint64                         `json:"index,string"` // Index of validator in validator registry.
	Balance   string                         `json:"balance"`      // Current validator balance in gwei.
	Status    string                         `json:"status"`
	Validator ValidatorResponseValidatorData `json:"validator"`
}

type ValidatorResponseValidatorData struct {
	Pubkey string `json:"pubkey"`
}

// HeadEvent is emitted on new slots in beacon client
type HeadEvent struct {
	Slot  uint64 `json:"slot,string"`
	Block string `json:"block"`
	State string `json:"state"`
}

func (h HeadEvent) Loggable() map[string]any {
	return map[string]any{
		"slot":  h.Slot,
		"block": h.Block,
		"state": h.State,
	}
}
