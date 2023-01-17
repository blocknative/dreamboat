package relay

import (
	"context"
	"errors"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"

	ds "github.com/ipfs/go-datastore"
)

var (
	DurationPerSlot = time.Second * 12
)

func (r *Relay) GetPayloadDelivered(ctx context.Context, query structs.PayloadTraceQuery) ([]structs.BidTraceExtended, error) {
	return r.evStore.GetDelivered(ctx, uint64(r.beaconState.Beacon().HeadSlot()), query)
}

func (r *Relay) GetBlockReceived(ctx context.Context, query structs.HeaderTraceQuery) ([]structs.BidTraceWithTimestamp, error) {
	var (
		events []structs.HeaderAndTrace
		err    error
	)

	if query.HasSlot() {
		events, err = r.d.GetHeadersBySlot(ctx, uint64(query.Slot))
	} else if query.HasBlockHash() {
		events, err = r.d.GetHeadersByBlockHash(ctx, query.BlockHash)
	} else if query.HasBlockNum() {
		events, err = r.d.GetHeadersByBlockNum(ctx, query.BlockNum)
	} else {
		events, err = r.d.GetLatestHeaders(ctx, query.Limit, uint64(r.config.TTL/DurationPerSlot))
	}

	if err == nil {
		traces := make([]structs.BidTraceWithTimestamp, 0, len(events))
		for _, event := range events {
			traces = append(traces, *event.Trace)
		}
		return traces, err
	} else if errors.Is(err, ds.ErrNotFound) {
		return []structs.BidTraceWithTimestamp{}, nil
	}
	return nil, err
}
