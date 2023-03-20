package relay

import (
	"context"

	"github.com/blocknative/dreamboat/structs"
)

func (r *Relay) GetPayloadDelivered(ctx context.Context, query structs.PayloadTraceQuery) ([]structs.BidTraceExtended, error) {
	return r.das.GetDeliveredPayloads(ctx, uint64(r.beaconState.HeadSlot()), query)
}

func (r *Relay) GetBlockReceived(ctx context.Context, query structs.SubmissionTraceQuery) ([]structs.BidTraceWithTimestamp, error) {
	return r.das.GetBuilderBlockSubmissions(ctx, uint64(r.beaconState.HeadSlot()), query)
}
