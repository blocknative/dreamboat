package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/blocknative/dreamboat/structs"
	"github.com/prometheus/client_golang/prometheus"
)

// t := prometheus.NewTimer(b.m.Timing.WithLabelValues("/eth/v1/beacon/blocks", "POST"))
// defer t.ObserveDuration()
func PublishBlock(ctx context.Context, address string, block []byte) error {

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address+"/eth/v1/beacon/blocks", bytes.NewReader(block))
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
		return fmt.Errorf("beacon error: %s", ec.Message)
	}

	return nil
}

func PublishV2Block(ctx context.Context, block structs.SignedBeaconBlock) error {
	buff := bytes.NewBuffer(nil)
	enc := json.NewEncoder(buff)
	if err := enc.Encode(block); err != nil {
		return fmt.Errorf("fail to marshal block: %w", err)
	}

	// UNSUPPORTED BY BEACONS RIGHT NOW - exists in documentation
	// payload, err := block.MarshalSSZ()
	// if err != nil {
	//	return fmt.Errorf("fail to encode block: %w", err)
	// }

	t := prometheus.NewTimer(b.m.Timing.WithLabelValues("/eth/v2/beacon/blocks", "POST"))
	defer t.ObserveDuration()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.beaconEndpoint.String()+"/eth/v2/beacon/blocks?broadcast_validation=consensus_and_equivocation", buff)
	if err != nil {
		return fmt.Errorf("fail to publish block: %w", err)
	}
	/// UNSUPPORTED BY BEACONS RIGHT NOW - exists in documentation
	// req.Header.Set("Content-Type", "application/octet-stream")

	req.Header.Set("Eth-Consensus-Version", block.ConsensusVersion())
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
