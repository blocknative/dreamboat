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
	"time"

	"github.com/lthibault/log"
	"github.com/r3labs/sse/v2"
	uberatomic "go.uber.org/atomic"
)

var (
	SlotsPerEpoch        Slot         = 32
	DurationPerSlot                   = time.Second * 12
	DurationPerEpoch                  = DurationPerSlot * time.Duration(SlotsPerEpoch)
	_                    BeaconClient = (*beaconClient)(nil) // type constraint
	ErrHTTPErrorResponse              = errors.New("got an HTTP error response")
	ErrNodesUnavailable               = errors.New("beacon nodes are unavailable")
)

type BeaconClient interface {
	SubscribeToHeadEvents(ctx context.Context, slotC chan HeadEvent)
	GetProposerDuties(Epoch) (*RegisteredProposersResponse, error)
	SyncStatus() (*SyncStatusPayloadData, error)
	KnownValidators(Slot) (AllValidatorsResponse, error)
	Endpoint() string
}

type MultiBeaconClient struct {
	Log     log.Logger
	Clients []BeaconClient

	bestBeaconIndex uberatomic.Int64
}

func NewMultiBeaconClient(l log.Logger, clients []BeaconClient) BeaconClient {
	if l == nil {
		l = log.New().WithField("service", "multi-beacon client")
	}
	return &MultiBeaconClient{Log: l, Clients: clients}
}

func (b *MultiBeaconClient) SubscribeToHeadEvents(ctx context.Context, slotC chan HeadEvent) {
	for _, client := range b.Clients {
		go client.SubscribeToHeadEvents(ctx, slotC)
	}
}

func (b *MultiBeaconClient) GetProposerDuties(epoch Epoch) (*RegisteredProposersResponse, error) {
	// return the first successful beacon node response
	clients := b.clientsByLastResponse()

	for i, client := range clients {
		log := b.Log.WithField("endpoint", client.Endpoint())

		duties, err := client.GetProposerDuties(epoch)
		if err != nil {
			log.WithError(err).Error("failed to get proposer duties")
			continue
		}

		b.bestBeaconIndex.Store(int64(i))

		// Received successful response. Set this index as last successful beacon node
		return duties, nil
	}

	return nil, ErrNodesUnavailable
}

func (b *MultiBeaconClient) SyncStatus() (*SyncStatusPayloadData, error) {
	var bestSyncStatus *SyncStatusPayloadData
	var foundSyncedNode bool

	// Check each beacon-node sync status
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, instance := range b.Clients {
		wg.Add(1)
		go func(client BeaconClient) {
			defer wg.Done()
			log := b.Log.WithField("endpoint", client.Endpoint())

			syncStatus, err := client.SyncStatus()
			if err != nil {
				log.WithError(err).Error("failed to get sync status")
				return
			}

			mu.Lock()
			defer mu.Unlock()

			if foundSyncedNode {
				return
			}

			if bestSyncStatus == nil {
				bestSyncStatus = syncStatus
			}

			if !syncStatus.IsSyncing {
				bestSyncStatus = syncStatus
				foundSyncedNode = true
			}
		}(instance)
	}

	// Wait for all requests to complete...
	wg.Wait()

	if !foundSyncedNode {
		return nil, ErrBeaconNodeSyncing
	}

	if bestSyncStatus == nil {
		return nil, ErrNodesUnavailable
	}

	return bestSyncStatus, nil
}

func (b *MultiBeaconClient) KnownValidators(headSlot Slot) (AllValidatorsResponse, error) {
	// return the first successful beacon node response
	clients := b.clientsByLastResponse()

	for i, client := range clients {
		log := b.Log.WithField("endpoint", client.Endpoint())

		validators, err := client.KnownValidators(headSlot)
		if err != nil {
			log.WithError(err).Error("failed to fetch validators")
			continue
		}

		b.bestBeaconIndex.Store(int64(i))

		// Received successful response. Set this index as last successful beacon node
		return validators, nil
	}

	return AllValidatorsResponse{}, ErrNodesUnavailable
}

func (b *MultiBeaconClient) Endpoint() string {
	return b.clientsByLastResponse()[0].Endpoint()
}

// beaconInstancesByLastResponse returns a list of beacon clients that has the client
// with the last successful response as the first element of the slice
func (b *MultiBeaconClient) clientsByLastResponse() []BeaconClient {
	index := b.bestBeaconIndex.Load()
	if index == 0 {
		return b.Clients
	}

	instances := make([]BeaconClient, len(b.Clients))
	copy(instances, b.Clients)
	instances[0], instances[index] = instances[index], instances[0]

	return instances
}

type beaconClient struct {
	beaconEndpoint *url.URL
	log            log.Logger
	Config
}

func NewBeaconClient(endpoint string, config Config) (*beaconClient, error) {
	u, err := url.Parse(endpoint)

	bc := &beaconClient{
		beaconEndpoint: u,
		log:            config.Log.WithField("beaconEndpoint", endpoint),
		Config:         config,
	}

	return bc, err
}

func (b *beaconClient) SubscribeToHeadEvents(ctx context.Context, slotC chan HeadEvent) {
	logger := b.log.WithField("method", "SubscribeToHeadEvents")

	eventsURL := fmt.Sprintf("%s/eth/v1/events?topics=head", b.beaconEndpoint.String())

	go func() {
		defer logger.Debug("head events subscription stopped")

		for {
			client := sse.NewClient(eventsURL)
			err := client.SubscribeRawWithContext(ctx, func(msg *sse.Event) {
				var head HeadEvent
				if err := json.Unmarshal(msg.Data, &head); err != nil {
					logger.WithError(err).Debug("event subscription failed")
				}

				select {
				case <-ctx.Done():
					return
				case slotC <- head:
					logger.
						With(head).
						Debug("read head subscription")
				}
			})

			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}

			logger.WithError(err).Debug("beacon subscription failed, restarting...")
		}
	}()
}

// Returns proposer duties for every slot in this epoch
func (b *beaconClient) GetProposerDuties(epoch Epoch) (*RegisteredProposersResponse, error) {
	u := *b.beaconEndpoint
	// https://ethereum.github.io/beacon-APIs/#/Validator/getProposerDuties
	u.Path = fmt.Sprintf("/eth/v1/validator/duties/proposer/%d", epoch)
	resp := new(RegisteredProposersResponse)
	err := b.queryBeacon(&u, "GET", resp)
	return resp, err
}

// SyncStatus returns the current node sync-status
func (b *beaconClient) SyncStatus() (*SyncStatusPayloadData, error) {
	u := *b.beaconEndpoint
	// https://ethereum.github.io/beacon-APIs/#/ValidatorRequiredApi/getSyncingStatus
	u.Path = "/eth/v1/node/syncing"
	resp := new(SyncStatusPayload)
	err := b.queryBeacon(&u, "GET", resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

func (b *beaconClient) KnownValidators(headSlot Slot) (AllValidatorsResponse, error) {
	u := *b.beaconEndpoint
	u.Path = fmt.Sprintf("/eth/v1/beacon/states/%d/validators", headSlot)
	q := u.Query()
	q.Add("status", "active,pending")
	u.RawQuery = q.Encode()

	var vd AllValidatorsResponse
	err := b.queryBeacon(&u, "GET", &vd)

	return vd, err
}

func (b *beaconClient) Endpoint() string {
	return b.beaconEndpoint.String()
}

func (b *beaconClient) queryBeacon(u *url.URL, method string, dst any) error {
	logger := b.log.
		WithField("method", "QueryBeacon").
		WithField("URL", u.RequestURI())
	timeStart := time.Now()

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

	logger.
		WithField("processingTimeMs", time.Since(timeStart).Milliseconds()).
		WithField("bytesAmount", len(bodyBytes)).
		Debug("beacon queried")

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

// HeadEvent is emitted when subscribing to head events
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

// RegisteredProposersResponse is the response for querying proposer duties
type RegisteredProposersResponse struct {
	Data []RegisteredProposersResponseData
}

type RegisteredProposersResponseData struct {
	PubKey PubKey `json:"pubkey"`
	Slot   uint64 `json:"slot,string"`
}

// AllValidatorsResponse is the response for querying active validators
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
