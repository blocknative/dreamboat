package relay

import (
	"context"
	"io"

	"github.com/blocknative/dreamboat/structs"
)

func (r *Relay) GetPayloadDelivered(ctx context.Context, w io.Writer, query structs.PayloadTraceQuery) error {
	return r.das.GetDeliveredPayloads(ctx, w, uint64(r.beaconState.HeadSlot()), query)
}

func (r *Relay) GetBlockReceived(ctx context.Context, w io.Writer, query structs.SubmissionTraceQuery) error {
	return r.das.GetBuilderBlockSubmissions(ctx, w, uint64(r.beaconState.HeadSlot()), query)
}
