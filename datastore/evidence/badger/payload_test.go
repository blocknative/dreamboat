package badger_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/blocknative/dreamboat/structs"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/google/uuid"
	ds "github.com/ipfs/go-datastore"

	"github.com/blocknative/dreamboat/datastore/evidence/badger"
	tBadger "github.com/blocknative/dreamboat/datastore/transport/badger"
	"github.com/stretchr/testify/require"
)

func TestPutGetHeaderDelivered(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := tBadger.Open("/tmp/" + t.Name() + uuid.New().String())
	require.NoError(t, err)
	defer store.Close()

	d := badger.NewDatastore(store, store.DB, time.Hour)
	blockNum := uint64(rand.Int())

	v := types.U256Str{}
	v.UnmarshalText([]byte("12395274185417459875"))

	dt := structs.DeliveredTrace{
		Trace: structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: types.BidTrace{
					Value:          v,
					BlockHash:      types.Hash(random32Bytes()),
					ProposerPubkey: types.PublicKey(random48Bytes()),
				},
				BlockNumber: blockNum,
				NumTx:       999,
			},
			Timestamp: uint64(time.Now().UnixMicro()),
		},
		BlockNumber: blockNum,
	}

	slotInt := rand.Int()
	slot := structs.Slot(slotInt)
	dt.Trace.Slot = uint64(slotInt)

	// get
	_, err = d.GetDeliveredPayloads(ctx, uint64(slotInt+1), structs.PayloadTraceQuery{Slot: slot})
	require.ErrorIs(t, err, ds.ErrNotFound)

	// get by block hash
	_, err = d.GetDeliveredPayloads(ctx, uint64(slotInt+1), structs.PayloadTraceQuery{BlockHash: dt.Trace.BlockHash})
	require.ErrorIs(t, err, ds.ErrNotFound)

	// get by block number
	_, err = d.GetDeliveredPayloads(ctx, uint64(slotInt+1), structs.PayloadTraceQuery{BlockNum: dt.BlockNumber})
	require.ErrorIs(t, err, ds.ErrNotFound)

	_, err = d.GetDeliveredPayloads(ctx, uint64(slotInt+1), structs.PayloadTraceQuery{Pubkey: dt.Trace.ProposerPubkey})
	require.ErrorIs(t, err, ds.ErrNotFound)

	// set as delivered and retrieve again
	err = d.PutDelivered(ctx, slot, dt)
	require.NoError(t, err)

	// get
	gotHeader, err := d.GetDeliveredPayloads(ctx, uint64(slotInt+1), structs.PayloadTraceQuery{Slot: slot})
	require.NoError(t, err)
	require.EqualValues(t, dt.Trace.Value, gotHeader[0].BidTrace.Value)

	// get by block hash
	gotHeader, err = d.GetDeliveredPayloads(ctx, uint64(slotInt+1), structs.PayloadTraceQuery{BlockHash: dt.Trace.BlockHash})
	require.NoError(t, err)
	require.EqualValues(t, dt.Trace.Value, gotHeader[0].BidTrace.Value)

	// get by block number
	gotHeader, err = d.GetDeliveredPayloads(ctx, uint64(slotInt+1), structs.PayloadTraceQuery{BlockNum: dt.BlockNumber})
	require.NoError(t, err)
	require.EqualValues(t, dt.Trace.Value, gotHeader[0].BidTrace.Value)

	gotHeader, err = d.GetDeliveredPayloads(ctx, uint64(slotInt+1), structs.PayloadTraceQuery{Pubkey: dt.Trace.ProposerPubkey})
	require.NoError(t, err)
	require.EqualValues(t, dt.Trace.Value, gotHeader[0].BidTrace.Value)
}

func random32Bytes() (b [32]byte) {
	rand.Read(b[:])
	return b
}

func random48Bytes() (b [48]byte) {
	rand.Read(b[:])
	return b
}
