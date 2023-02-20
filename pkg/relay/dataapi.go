package relay

import (
	"context"

	"github.com/blocknative/dreamboat/pkg/structs"
)

func (r *Relay) GetPayloadDelivered(ctx context.Context, query structs.PayloadTraceQuery) ([]structs.BidTraceExtended, error) {
	return r.dastore.GetDeliveredPayloads(ctx, uint64(r.beaconState.Beacon().HeadSlot()), query)
}

func (r *Relay) GetBlockReceived(ctx context.Context, query structs.SubmissionTraceQuery) ([]structs.BidTraceWithTimestamp, error) {
	return r.dastore.GetBuilderBlockSubmissions(ctx, uint64(r.beaconState.Beacon().HeadSlot()), query)
}
