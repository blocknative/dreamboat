package badger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"math/rand"
	"testing"
	"time"

	"github.com/blocknative/dreamboat/structs"
	"github.com/flashbots/go-boost-utils/types"

	dbbadger "github.com/ipfs/go-ds-badger2"

	"github.com/blocknative/dreamboat/datastore/evidence/badger"
	"github.com/stretchr/testify/require"
)

func TestPutGetHeaderDelivered2(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := dbbadger.NewDatastore("/tmp/EvidenceBadger", &dbbadger.DefaultOptions)
	require.NoError(t, err)
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

	// Create an io.Writer for encoding the data
	var (
		buf       bytes.Buffer
		gotHeader []structs.BidTraceWithTimestamp
	)

	// get
	buf.Reset()
	err = d.GetDeliveredPayloads(ctx, &buf, uint64(slotInt+1), structs.PayloadTraceQuery{Slot: slot})
	require.NoError(t, err)
	err = json.Unmarshal(buf.Bytes(), &gotHeader)
	require.NoError(t, err)
	require.Len(t, gotHeader, 0)

	// get by block hash
	buf.Reset()
	err = d.GetDeliveredPayloads(ctx, &buf, uint64(slotInt+1), structs.PayloadTraceQuery{BlockHash: dt.Trace.BlockHash})
	require.NoError(t, err)
	err = json.Unmarshal(buf.Bytes(), &gotHeader)
	require.NoError(t, err)
	require.Len(t, gotHeader, 0)

	// get by block number
	buf.Reset()
	err = d.GetDeliveredPayloads(ctx, &buf, uint64(slotInt+1), structs.PayloadTraceQuery{BlockNum: dt.BlockNumber})
	require.NoError(t, err)
	err = json.Unmarshal(buf.Bytes(), &gotHeader)
	require.NoError(t, err)
	require.Len(t, gotHeader, 0)

	// get by proposer pubkey
	buf.Reset()
	err = d.GetDeliveredPayloads(ctx, &buf, uint64(slotInt+1), structs.PayloadTraceQuery{ProposerPubkey: dt.Trace.ProposerPubkey})
	require.NoError(t, err)
	err = json.Unmarshal(buf.Bytes(), &gotHeader)
	require.NoError(t, err)
	require.Len(t, gotHeader, 0)

	// set as delivered and retrieve again
	err = d.PutDelivered(ctx, slot, dt)
	require.NoError(t, err)

	buf.Reset()

	// Perform the GetDeliveredPayloads operation by encoding to the writer
	err = d.GetDeliveredPayloads(ctx, &buf, uint64(slotInt+1), structs.PayloadTraceQuery{Slot: slot})
	require.NoError(t, err)

	// Decode the written data into []structs.BidTraceWithTimestamp
	err = json.Unmarshal(buf.Bytes(), &gotHeader)
	require.NoError(t, err)

	// Perform the checks
	require.EqualValues(t, dt.Trace.Value, gotHeader[0].BidTrace.Value)
	require.EqualValues(t, dt.Trace.BlockHash, gotHeader[0].BidTrace.BlockHash)
	require.EqualValues(t, dt.Trace.ProposerPubkey, gotHeader[0].BidTrace.ProposerPubkey)
	require.EqualValues(t, dt.Trace.Slot, gotHeader[0].BidTrace.Slot)
}

func random32Bytes() (b [32]byte) {
	rand.Read(b[:])
	return b
}

func random48Bytes() (b [48]byte) {
	rand.Read(b[:])
	return b
}
