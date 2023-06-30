package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/blocknative/dreamboat/structs"
)

var (
	ErrNodesUnavailable = errors.New("beacon nodes are unavailable")
	ErrBlockPublish202  = errors.New("the block failed validation, but was successfully broadcast anyway. It was not integrated into the beacon node's database")
	ErrFailedToPublish  = errors.New("failed to publish")
)

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

type ClientError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

//tCtx, cancel := context.WithTimeout(ctx, timeout) // b.c.BeaconQueryTimeout)
//defer cancel()
// t := prometheus.NewTimer(b.m.Timing.WithLabelValues("/eth/v1/beacon/genesis", "GET"))
// t.ObserveDuration()

func Genesis(ctx context.Context, address string) (gi structs.GenesisInfo, err error) {

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address+"/eth/v1/beacon/genesis", nil)
	if err != nil {
		return gi, fmt.Errorf("invalid request for %s: %w", address, err)
	}
	req.Header.Set("accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return gi, fmt.Errorf("error querying beacon %s: %w", address, err)
	}
	defer resp.Body.Close()

	if err := checkForFailure(resp.Body, resp.StatusCode); err != nil {
		return gi, fmt.Errorf("error in beacon response %s: %w", address, err)
	}

	gen := &GenesisResponse{}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(gen); err != nil {
		return gi, fmt.Errorf("could not unmarshal response forom %s %w", address, err)
	}

	return gen.Data, err
}

//	tCtx, cancel := context.WithTimeout(ctx, timeout) // b.c.BeaconQueryTimeout)
//	defer cancel()
// t := prometheus.NewTimer(b.m.Timing.WithLabelValues("/eth/v1/validator/duties/proposer", "GET"))
// t.ObserveDuration()

// https://ethereum.github.io/beacon-APIs/#/Validator/getProposerDuties
func GetProposerDuties(ctx context.Context, address string, epoch uint64) (gi *RegisteredProposersResponse, err error) {

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/eth/v1/validator/duties/proposer/%d", address, epoch), nil)
	if err != nil {
		return nil, fmt.Errorf("invalid request for %s: %w", address, err)
	}
	req.Header.Set("accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error querying beacon %s: %w", address, err)
	}
	defer resp.Body.Close()

	if err := checkForFailure(resp.Body, resp.StatusCode); err != nil {
		return nil, fmt.Errorf("error in beacon response %s: %w", address, err)
	}

	regProp := &RegisteredProposersResponse{}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(regProp); err != nil {
		return nil, fmt.Errorf("could not unmarshal response forom %s %w", address, err)
	}

	return regProp, err
}

// t := prometheus.NewTimer(b.m.Timing.WithLabelValues("/eth/v1/node/syncing", "GET"))
// defer t.ObserveDuration()

// SyncStatus returns the current node sync-status
// https://ethereum.github.io/beacon-APIs/#/ValidatorRequiredApi/getSyncingStatus
func SyncStatus(ctx context.Context, address string) (sspd SyncStatusPayloadData, err error) {

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/eth/v1/node/syncing", nil)
	if err != nil {
		return sspd, fmt.Errorf("invalid request for %s: %w", address, err)
	}
	req.Header.Set("accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return sspd, fmt.Errorf("error querying beacon %s: %w", address, err)
	}
	defer resp.Body.Close()

	if err := checkForFailure(resp.Body, resp.StatusCode); err != nil {
		return sspd, fmt.Errorf("error in beacon response %s: %w", address, err)
	}

	ssp := &SyncStatusPayload{}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(ssp); err != nil {
		return sspd, fmt.Errorf("could not unmarshal response forom %s %w", address, err)
	}

	return ssp.Data, err
}

// t := prometheus.NewTimer(b.m.Timing.WithLabelValues("/eth/v1/beacon/states/validators", "GET"))
// defer t.ObserveDuration()
func KnownValidators(ctx context.Context, address string, slot uint64) (avr AllValidatorsResponse, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/eth/v1/beacon/states/%d/validators?status=active,pending", address, slot), nil)
	if err != nil {
		return avr, fmt.Errorf("invalid request for %s: %w", address, err)
	}
	req.Header.Set("accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return avr, fmt.Errorf("error querying beacon %s: %w", address, err)
	}
	defer resp.Body.Close()

	if err := checkForFailure(resp.Body, resp.StatusCode); err != nil {
		return avr, fmt.Errorf("error in beacon response %s: %w", address, err)
	}

	avr = AllValidatorsResponse{}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&avr); err != nil {
		return avr, fmt.Errorf("could not unmarshal response forom %s %w", address, err)
	}

	return avr, err
}

// GetWithdrawals - /eth/v1/beacon/states/<slot>/withdrawals
//
//	t := prometheus.NewTimer(b.m.Timing.WithLabelValues("/eth/v1/beacon/states/withdrawals", "GET"))
//	defer t.ObserveDuration()
func GetWithdrawals(ctx context.Context, address string, slot uint64) (gwr GetWithdrawalsResponse, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/eth/v1/beacon/states/%d/withdrawals", address, slot), nil)
	if err != nil {
		return gwr, fmt.Errorf("invalid request for %s: %w", address, err)
	}
	req.Header.Set("accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return gwr, fmt.Errorf("error querying beacon %s: %w", address, err)
	}
	defer resp.Body.Close()

	if err := checkForFailure(resp.Body, resp.StatusCode); err != nil {
		return gwr, fmt.Errorf("error in beacon response %s: %w", address, err)
	}

	gwr = GetWithdrawalsResponse{}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&gwr); err != nil {
		return gwr, fmt.Errorf("could not unmarshal response forom %s %w", address, err)
	}

	return gwr, err
}

// Randao - /eth/v1/beacon/states/<slot>/randao
//
// t := prometheus.NewTimer(b.m.Timing.WithLabelValues("/eth/v1/beacon/states/randao", "GET"))
// defer t.ObserveDuration()
func Randao(ctx context.Context, address string, slot uint64) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/eth/v1/beacon/states/%d/randao", address, slot), nil)
	if err != nil {
		return "", fmt.Errorf("invalid request for %s: %w", address, err)
	}
	req.Header.Set("accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error querying beacon %s: %w", address, err)
	}
	defer resp.Body.Close()

	if err := checkForFailure(resp.Body, resp.StatusCode); err != nil {
		return "", fmt.Errorf("error in beacon response %s: %w", address, err)
	}

	rresp := &GetRandaoResponse{}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&rresp); err != nil {
		return "", fmt.Errorf("could not unmarshal response forom %s %w", address, err)
	}

	return rresp.Data.Randao, err
}

// GetForkSchedule - https://ethereum.github.io/beacon-APIs/#/Config/getForkSchedule
//
//	t := prometheus.NewTimer(b.m.Timing.WithLabelValues("/eth/v1/config/fork_schedule", "GET"))
//	defer t.ObserveDuration()
func GetForkSchedule(ctx context.Context, address string) (spec *GetForkScheduleResponse, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address+"/eth/v1/config/fork_schedule", nil)
	if err != nil {
		return nil, fmt.Errorf("invalid request for %s: %w", address, err)
	}
	req.Header.Set("accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error querying beacon %s: %w", address, err)
	}
	defer resp.Body.Close()

	if err := checkForFailure(resp.Body, resp.StatusCode); err != nil {
		return nil, fmt.Errorf("error in beacon response %s: %w", address, err)
	}

	spec = &GetForkScheduleResponse{}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&spec); err != nil {
		return nil, fmt.Errorf("could not unmarshal response forom %s %w", address, err)
	}

	return spec, err
}

// Returns the latest header from the beacon chain
//
//	t := prometheus.NewTimer(b.m.Timing.WithLabelValues("/eth/v1/beacon/headers", "GET"))
//	defer t.ObserveDuration()
func Headers(ctx context.Context, address string) (spec *HeaderRootObject, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address+"/eth/v1/beacon/headers", nil)
	if err != nil {
		return nil, fmt.Errorf("invalid request for %s: %w", address, err)
	}
	req.Header.Set("accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error querying beacon %s: %w", address, err)
	}
	defer resp.Body.Close()

	if err := checkForFailure(resp.Body, resp.StatusCode); err != nil {
		return nil, fmt.Errorf("error in beacon response %s: %w", address, err)
	}

	spec = &HeaderRootObject{}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&spec); err != nil {
		return nil, fmt.Errorf("could not unmarshal response forom %s %w", address, err)
	}

	return spec, err
}

func checkForFailure(body io.Reader, statusCode int) error {
	if statusCode >= 404 {
		return fmt.Errorf("failure querying beacon node")
	}

	if statusCode >= 300 {
		dec := json.NewDecoder(body)

		ce := &ClientError{}
		if err := dec.Decode(ce); err != nil {
			return fmt.Errorf("error reading beacon error response: %w", err)
		}
		return fmt.Errorf("error querying beacon: %s", ce.Message)
	}
	return nil
}
