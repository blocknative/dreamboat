package relay

import (
	"context"
	"errors"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"golang.org/x/exp/constraints"

	ds "github.com/ipfs/go-datastore"
)

var (
	DurationPerSlot = time.Second * 12
)

func (r *Relay) GetPayloadDelivered(ctx context.Context, query structs.PayloadTraceQuery) ([]structs.BidTraceExtended, error) {
	var (
		event structs.BidTraceWithTimestamp
		err   error
	)

	if query.HasSlot() {
		event, err = r.d.GetDelivered(ctx, structs.PayloadQuery{Slot: query.Slot})
	} else if query.HasBlockHash() {
		event, err = r.d.GetDelivered(ctx, structs.PayloadQuery{BlockHash: query.BlockHash})
	} else if query.HasBlockNum() {
		event, err = r.d.GetDelivered(ctx, structs.PayloadQuery{BlockNum: query.BlockNum})
	} else if query.HasPubkey() {
		event, err = r.d.GetDelivered(ctx, structs.PayloadQuery{PubKey: query.Pubkey})
	} else {
		return r.getTailDelivered(ctx, query.Limit, query.Cursor)
	}

	if err == nil {
		return []structs.BidTraceExtended{{BidTrace: event.BidTrace, BlockNumber: event.BlockNumber, NumTx: event.NumTx}}, err
	} else if errors.Is(err, ds.ErrNotFound) {
		return []structs.BidTraceExtended{}, nil
	}
	return nil, err
}

func (r *Relay) getTailDelivered(ctx context.Context, limit, cursor uint64) ([]structs.BidTraceExtended, error) {
	headSlot := r.beaconState.HeadSlot()
	start := headSlot
	if cursor != 0 {
		start = min(headSlot, structs.Slot(cursor))
	}

	stop := start - min(structs.Slot(r.config.TTL/DurationPerSlot), start)

	batch := make([]structs.BidTraceWithTimestamp, 0, limit)
	queries := make([]structs.PayloadQuery, 0, limit)

	for highSlot := start; len(batch) < int(limit) && stop <= highSlot; highSlot -= min(structs.Slot(limit), highSlot) {
		queries = queries[:0]
		for s := highSlot; highSlot-structs.Slot(limit) < s && stop <= s; s-- {
			queries = append(queries, structs.PayloadQuery{Slot: s})
		}

		nextBatch, err := r.d.GetDeliveredBatch(ctx, queries)
		if err != nil {
			r.l.WithError(err).Warn("failed getting header batch")
		} else {
			batch = append(batch, nextBatch[:min(int(limit)-len(batch), len(nextBatch))]...)
		}
	}

	events := make([]structs.BidTraceExtended, 0, len(batch))
	for _, event := range batch {
		events = append(events, event.BidTraceExtended)
	}
	return events, nil
}

func (r *Relay) GetBlockReceived(ctx context.Context, query structs.HeaderTraceQuery) ([]structs.BidTraceWithTimestamp, error) {
	var (
		events []structs.HeaderAndTrace
		err    error
	)

	if query.HasSlot() {
		events, err = r.d.GetHeadersBySlot(ctx, r.beaconState.ForkVersion(query.Slot) ,uint64(query.Slot))
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
			tr := event.Trace()
			traces = append(traces, tr)
		}
		return traces, err
	} else if errors.Is(err, ds.ErrNotFound) {
		return []structs.BidTraceWithTimestamp{}, nil
	}
	return nil, err
}

func min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}
