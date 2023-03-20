package badger_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/blocknative/dreamboat/structs"
	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"

	dbbadger "github.com/ipfs/go-ds-badger2"

	"github.com/blocknative/dreamboat/pkg/datastore/evidence/badger"
	"github.com/stretchr/testify/require"
)

func TestPutGetHeaderDelivered(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := dbbadger.NewDatastore("/tmp/EvidenceBadger", &dbbadger.DefaultOptions)
	require.NoError(t, err)
	d := badger.NewDatastore(store, store.DB, time.Hour)

	h, _ := types.PayloadToPayloadHeader(&types.ExecutionPayload{
		BlockNumber: uint64(rand.Int()),
	})
	v := types.U256Str{}
	v.UnmarshalText([]byte("12395274185417459875"))
	header := structs.HeaderAndTrace{
		Header: h,
		Trace: &structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: types.BidTrace{
					Value:          v,
					BlockHash:      types.Hash(random32Bytes()),
					ProposerPubkey: types.PublicKey(random48Bytes()),
				},
				BlockNumber: h.BlockNumber,
				NumTx:       999,
			},
			Timestamp: uint64(time.Now().UnixMicro()),
		},
	}
	slotInt := rand.Int()
	slot := structs.Slot(slotInt)
	header.Trace.Slot = uint64(slotInt)

	// get
	_, err = d.GetDeliveredPayloads(ctx, uint64(slotInt+1), structs.PayloadTraceQuery{Slot: slot})
	require.ErrorIs(t, err, ds.ErrNotFound)

	// get by block hash
	_, err = d.GetDeliveredPayloads(ctx, uint64(slotInt+1), structs.PayloadTraceQuery{BlockHash: header.Trace.BlockHash})
	require.ErrorIs(t, err, ds.ErrNotFound)

	// get by block number
	_, err = d.GetDeliveredPayloads(ctx, uint64(slotInt+1), structs.PayloadTraceQuery{BlockNum: header.Header.BlockNumber})
	require.ErrorIs(t, err, ds.ErrNotFound)

	_, err = d.GetDeliveredPayloads(ctx, uint64(slotInt+1), structs.PayloadTraceQuery{Pubkey: header.Trace.ProposerPubkey})
	require.ErrorIs(t, err, ds.ErrNotFound)

	// set as delivered and retrieve again
	err = d.PutDelivered(ctx, slot, structs.DeliveredTrace{Trace: *header.Trace, BlockNumber: header.Header.BlockNumber}, time.Minute)
	require.NoError(t, err)

	// get
	gotHeader, err := d.GetDeliveredPayloads(ctx, uint64(slotInt+1), structs.PayloadTraceQuery{Slot: slot})
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader[0].BidTrace.Value)

	// get by block hash
	gotHeader, err = d.GetDeliveredPayloads(ctx, uint64(slotInt+1), structs.PayloadTraceQuery{BlockHash: header.Trace.BlockHash})
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader[0].BidTrace.Value)

	// get by block number
	gotHeader, err = d.GetDeliveredPayloads(ctx, uint64(slotInt+1), structs.PayloadTraceQuery{BlockNum: header.Header.BlockNumber})
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader[0].BidTrace.Value)

	gotHeader, err = d.GetDeliveredPayloads(ctx, uint64(slotInt+1), structs.PayloadTraceQuery{Pubkey: header.Trace.ProposerPubkey})
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader[0].BidTrace.Value)
}

func random32Bytes() (b [32]byte) {
	rand.Read(b[:])
	return b
}

func random48Bytes() (b [48]byte) {
	rand.Read(b[:])
	return b
}
