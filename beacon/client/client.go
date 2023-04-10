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
	"time"

	"github.com/blocknative/dreamboat/metrics"
	"github.com/blocknative/dreamboat/structs"
	"github.com/lthibault/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/r3labs/sse/v2"
)

var (
	ErrHTTPErrorResponse = errors.New("got an HTTP error response")
	ErrNodesUnavailable  = errors.New("beacon nodes are unavailable")
	ErrBlockPublish202   = errors.New("the block failed validation, but was successfully broadcast anyway. It was not integrated into the beacon node's database")
)

type beaconClient struct {
	beaconEndpoint *url.URL
	log            log.Logger
	m              BeaconMetrics
	c              BeaconConfig
}

type BeaconMetrics struct {
	Timing *prometheus.HistogramVec
}

type BeaconConfig struct {
	BeaconEventTimeout time.Duration
	BeaconEventRestart int
	BeaconQueryTimeout time.Duration
}

func NewBeaconClient(l log.Logger, endpoint string, config BeaconConfig) (*beaconClient, error) {
	u, err := url.Parse(endpoint)

	bc := &beaconClient{
		beaconEndpoint: u,
		log: l.With(log.F{
			"beaconEndpoint":           endpoint,
			"beaconEventTimeout":       config.BeaconEventTimeout,
			"BeaconEventRestart": config.BeaconEventRestart,
			"beaconQueryTimeout":       config.BeaconQueryTimeout,
		}),
		c: config,
	}

	bc.initMetrics()

	return bc, err
}

func (b *beaconClient) SubscribeToHeadEvents(ctx context.Context, slotC chan HeadEvent) {
	logger := b.log.WithField("method", "SubscribeToHeadEvents")
	defer logger.Debug("head events subscription stopped")

	for {
		loopCtx, cancelLoop := context.WithCancel(ctx)
		timer := time.NewTimer(b.c.BeaconEventTimeout)
		go b.runNewHeadSubscriptionLoop(loopCtx, logger, timer, slotC)

		lastTimeout := time.Time{} // zeroed value time.Time
		timeoutCounter := 0
	EventSelect:
		for {
			select {
			case <-timer.C:
				logger.Debug("timed out head events subscription, manually querying latest header")

				go b.manuallyFetchLatestHeader(ctx, logger, slotC)

				// to prevent disconnection due to multiple timeouts occurring very close in time, the subscription loop is canceled and restarted
				if time.Since(lastTimeout) <= b.c.BeaconEventTimeout*2 {
					timeoutCounter++
					if timeoutCounter >= b.c.BeaconEventRestart {
						logger.WithField("timeoutCounter", timeoutCounter).Warn("restarting subcription")
						cancelLoop()
						break EventSelect
					}
				} else {
					timeoutCounter = 0
				}

				lastTimeout = time.Now()
				timer.Reset(b.c.BeaconEventTimeout)
			case <-ctx.Done():
				cancelLoop()
				return
			}
		}
	}
}

func (b *beaconClient) runNewHeadSubscriptionLoop(ctx context.Context, logger log.Logger, timer *time.Timer, slotC chan<- HeadEvent) {
	logger.Debug("subscription loop started")
	defer logger.Warn("subscription loop exited")

	for {
		client := sse.NewClient(fmt.Sprintf("%s/eth/v1/events?topics=head", b.beaconEndpoint.String()))
		err := client.SubscribeRawWithContext(ctx, func(msg *sse.Event) {
			var head HeadEvent
			if err := json.Unmarshal(msg.Data, &head); err != nil {
				logger.WithError(err).Warn("event subscription failed")
				return
			}

			timer.Reset(b.c.BeaconEventTimeout)

			select {
			case slotC <- head:
			case <-time.After(structs.DurationPerSlot / 2): // relief pressure
				logger.WithField("timeout", structs.DurationPerSlot/2).Warn("timeout waiting to consume head event")
				return
			case <-ctx.Done():
				logger.WithError(ctx.Err()).Warn("context cancelled waiting to consume head event after manual querying")
				return
			}
		})

		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}

		logger.WithError(err).Warn("beacon subscription failed, restarting...")
	}
}

func (b *beaconClient) manuallyFetchLatestHeader(ctx context.Context, logger log.Logger, slotC chan HeadEvent) {
	// fetch head slot manully
	header, err := b.queryLatestHeader()
	if err != nil {
		logger.WithError(err).Warn("failed querying latest header manually")
		return
	} else if len(header.Data) != 1 {
		logger.Warnf("failed to query latest header manually, unexpected amount of header: expected 1 - got %d", len(header.Data))
		return
	}

	// send slot to beacon manager to consume async
	event := HeadEvent{Slot: header.Data[0].Header.Message.Slot}
	logger.With(event).Debug("manually fetched latest beacon header")

	select {
	case slotC <- event:
		return
	case <-time.After(structs.DurationPerSlot / 2):
		logger.WithField("timeout", structs.DurationPerSlot/2).Warn("timeout waiting to consume head event after manual querying")
		return
	case <-ctx.Done():
		logger.WithError(ctx.Err()).Warn("context cancelled waiting to consume head event after manual querying")
		return
	}
}

// Returns the latest header from the beacon chain
func (b *beaconClient) queryLatestHeader() (*HeaderRootObject, error) {
	u := *b.beaconEndpoint
	u.Path = fmt.Sprintf("/eth/v1/beacon/headers")
	resp := new(HeaderRootObject)

	t := prometheus.NewTimer(b.m.Timing.WithLabelValues("/eth/v1/beacon/headers", "GET"))
	defer t.ObserveDuration()

	err := b.queryBeacon(&u, "GET", resp)
	return resp, err
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

// GetWithdrawals - /eth/v1/beacon/states/<slot>/withdrawals
func (b *beaconClient) GetWithdrawals(slot structs.Slot) (*GetWithdrawalsResponse, error) {
	resp := new(GetWithdrawalsResponse)
	u := *b.beaconEndpoint
	// https://ethereum.github.io/beacon-APIs/#/ValidatorRequiredApi/getSyncingStatus
	t := prometheus.NewTimer(b.m.Timing.WithLabelValues("/eth/v1/beacon/states/withdrawals", "GET"))
	defer t.ObserveDuration()

	u.Path = fmt.Sprintf("/eth/v1/beacon/states/%d/withdrawals", slot)
	err := b.queryBeacon(&u, "GET", &resp)
	return resp, err
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

// GetForkSchedule - https://ethereum.github.io/beacon-APIs/#/Config/getForkSchedule
func (b *beaconClient) GetForkSchedule() (spec *GetForkScheduleResponse, err error) {
	resp := new(GetForkScheduleResponse)
	u := *b.beaconEndpoint

	t := prometheus.NewTimer(b.m.Timing.WithLabelValues("/eth/v1/config/fork_schedule", "GET"))
	defer t.ObserveDuration()

	u.Path = "/eth/v1/config/fork_schedule"
	err = b.queryBeacon(&u, "GET", &resp)
	return resp, err
}

func (b *beaconClient) PublishBlock(ctx context.Context, block structs.SignedBeaconBlock) error {
	buff := bytes.NewBuffer(nil)
	enc := json.NewEncoder(buff)
	if err := enc.Encode(block); err != nil {
		return fmt.Errorf("fail to marshal block: %w", err)
	}

	t := prometheus.NewTimer(b.m.Timing.WithLabelValues("/eth/v1/beacon/blocks", "POST"))
	defer t.ObserveDuration()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.beaconEndpoint.String()+"/eth/v1/beacon/blocks", buff)
	if err != nil {
		return fmt.Errorf("fail to publish block: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fail to publish block: %w", err)
	}

	if resp.StatusCode == 202 { // https://ethereum.github.io/beacon-APIs/#/Beacon/publishBlock
		return ErrBlockPublish202
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
	ctx, cancel := context.WithTimeout(context.Background(), b.c.BeaconQueryTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
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

	if resp.StatusCode >= 404 {
		// BUG(l): do something with unsupported
		return nil
	} else if resp.StatusCode >= 300 {
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
	Slot uint64 `json:"slot,string"`
	// Block string `json:"block"`
	// State string `json:"state"`
}

func (h HeadEvent) Loggable() map[string]any {
	return map[string]any{
		"slot": h.Slot,
		// "block": h.Block,
		// "state": h.State,
	}
}

// Header is the block header from the beacon chain
type Message struct {
	Slot          uint64 `json:"slot,string"`
	ProposerIndex string `json:"proposer_index"`
	ParentRoot    string `json:"parent_root"`
	StateRoot     string `json:"state_root"`
	BodyRoot      string `json:"body_root"`
}

type Header struct {
	Message   Message `json:"message"`
	Signature string  `json:"signature"`
}

type Data struct {
	Root      string `json:"root"`
	Canonical bool   `json:"canonical"`
	Header    Header `json:"header"`
}

type HeaderRootObject struct {
	ExecutionOptimistic bool   `json:"execution_optimistic"`
	Data                []Data `json:"data"`
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

type GetWithdrawalsResponse struct {
	Data struct {
		Withdrawals structs.Withdrawals `json:"withdrawals"`
	}
}

// GetRandaoResponse is the response for querying randao from beacon
type GetRandaoResponse struct {
	Data struct {
		Randao string `json:"randao"`
	}
}

type GetForkScheduleResponse struct {
	Data []struct {
		PreviousVersion string `json:"previous_version"`
		CurrentVersion  string `json:"current_version"`
		Epoch           uint64 `json:"epoch,string"`
	}
}
