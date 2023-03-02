package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/blocknative/dreamboat/metrics"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/r3labs/sse/v2"
)

var (
	ErrHTTPErrorResponse = errors.New("got an HTTP error response")
	ErrNodesUnavailable  = errors.New("beacon nodes are unavailable")
)

type beaconClient struct {
	beaconEndpoint *url.URL
	log            log.Logger
	m              BeaconMetrics
}

type BeaconMetrics struct {
	Timing *prometheus.HistogramVec
}

func NewBeaconClient(l log.Logger, endpoint string) (*beaconClient, error) {
	u, err := url.Parse(endpoint)

	bc := &beaconClient{
		beaconEndpoint: u,
		log:            l.WithField("beaconEndpoint", endpoint),
	}

	bc.initMetrics()

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
func (b *beaconClient) GetProposerDuties(epoch structs.Epoch) (*RegisteredProposersResponse, error) {
	u := *b.beaconEndpoint
	// https://ethereum.github.io/beacon-APIs/#/Validator/getProposerDuties
	u.Path = fmt.Sprintf("/eth/v1/validator/duties/proposer/%d", epoch)
	resp := new(RegisteredProposersResponse)

	t := prometheus.NewTimer(b.m.Timing.WithLabelValues("/eth/v1/validator/duties/proposer", "GET"))
	defer t.ObserveDuration()

	err := b.queryBeacon(&u, "GET", resp)
	return resp, err
}

// SyncStatus returns the current node sync-status
func (b *beaconClient) SyncStatus() (*SyncStatusPayloadData, error) {
	u := *b.beaconEndpoint
	// https://ethereum.github.io/beacon-APIs/#/ValidatorRequiredApi/getSyncingStatus
	u.Path = "/eth/v1/node/syncing"
	resp := new(SyncStatusPayload)

	t := prometheus.NewTimer(b.m.Timing.WithLabelValues("/eth/v1/node/syncing", "GET"))
	defer t.ObserveDuration()

	err := b.queryBeacon(&u, "GET", resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

func (b *beaconClient) KnownValidators(headSlot structs.Slot) (AllValidatorsResponse, error) {
	u := *b.beaconEndpoint
	u.Path = fmt.Sprintf("/eth/v1/beacon/states/%d/validators", headSlot)
	q := u.Query()
	q.Add("status", "active,pending")
	u.RawQuery = q.Encode()

	t := prometheus.NewTimer(b.m.Timing.WithLabelValues("/eth/v1/beacon/states/validators", "GET"))
	defer t.ObserveDuration()

	var vd AllValidatorsResponse
	err := b.queryBeacon(&u, "GET", &vd)

	return vd, err
}

func (b *beaconClient) Genesis() (structs.GenesisInfo, error) {
	resp := new(GenesisResponse)
	u := *b.beaconEndpoint
	// https://ethereum.github.io/beacon-APIs/#/ValidatorRequiredApi/getSyncingStatus
	t := prometheus.NewTimer(b.m.Timing.WithLabelValues("/eth/v1/beacon/genesis", "GET"))
	defer t.ObserveDuration()

	u.Path = "/eth/v1/beacon/genesis"
	err := b.queryBeacon(&u, "GET", &resp)
	return resp.Data, err
}

func (b *beaconClient) Randao(slot structs.Slot) (string, error) {
	resp := new(GetRandaoResponse)
	u := *b.beaconEndpoint
	u.Path = fmt.Sprintf("/eth/v1/beacon/states/%d/randao", slot)
	
	t := prometheus.NewTimer(b.m.Timing.WithLabelValues("/eth/v1/beacon/states/randao", "GET"))
	defer t.ObserveDuration()
	
	err := b.queryBeacon(&u, "GET", &resp)
	return resp.Data.Randao, err
}

func (b *beaconClient) PublishBlock(block *types.SignedBeaconBlock) error {
	bb, err := json.Marshal(block)
	if err != nil {
		return fmt.Errorf("fail to marshal block: %w", err)
	}

	t := prometheus.NewTimer(b.m.Timing.WithLabelValues("/eth/v1/beacon/blocks", "POST"))
	defer t.ObserveDuration()

	resp, err := http.Post(b.beaconEndpoint.String()+"/eth/v1/beacon/blocks", "application/json", bytes.NewBuffer(bb))
	if err != nil {
		return fmt.Errorf("fail to publish block: %w", err)
	}

	if resp.StatusCode == 202 { // https://ethereum.github.io/beacon-APIs/#/Beacon/publishBlock
		return fmt.Errorf("the block failed validation, but was successfully broadcast anyway. It was not integrated into the beacon node's database")
	} else if resp.StatusCode >= 300 {
		ec := &struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{}
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("fail to read read error response body: %w", err)
		}

		if err = json.Unmarshal(bodyBytes, ec); err != nil {
			return fmt.Errorf("fail to unmarshal error response: %w", err)
		}
		return fmt.Errorf("%w: %s", ErrHTTPErrorResponse, ec.Message)
	}

	return nil
}

func (b *beaconClient) Endpoint() string {
	return b.beaconEndpoint.String()
}

func (b *beaconClient) initMetrics() {
	b.m.Timing = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "dreamboat",
		Subsystem: "beacon",
		Name:      "timing",
		Help:      "Duration of requests per endpoint",
	}, []string{"endpoint", "method"})
}

func (b *beaconClient) AttachMetrics(m *metrics.Metrics) {
	m.Register(b.m.Timing)
}

func (b *beaconClient) queryBeacon(u *url.URL, method string, dst any) error {
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
	PubKey structs.PubKey `json:"pubkey"`
	Slot   uint64         `json:"slot,string"`
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

type GenesisResponse struct {
	Data structs.GenesisInfo
}

// GetRandaoResponse is the response for querying randao from beacon
type GetRandaoResponse struct {
	Data struct {
		Randao string `json:"randao"`
	}
}
