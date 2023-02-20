package datastore_test

import (
	"context"
	"encoding/json"
	"math/big"
	"math/rand"
	"sort"
	"sync"
	"testing"
	"time"

	datastore "github.com/blocknative/dreamboat/pkg/datastore"
	"github.com/flashbots/go-boost-utils/types"

	"github.com/blocknative/dreamboat/pkg/structs"
	badger "github.com/ipfs/go-ds-badger2"
	"github.com/stretchr/testify/require"
)

func TestPutGetHeader(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := badger.NewDatastore("/tmp/BadgerBatcher1", &badger.DefaultOptions)
	require.NoError(t, err)

	hc := datastore.NewHeaderController(200, time.Hour)
	ds, err := datastore.NewDatastore(store, store.DB, hc, 100)
	require.NoError(t, err)

	header := randomHeaderAndTrace()
	slotInt := rand.Int()
	slot := structs.Slot(slotInt)

	header.Trace.Slot = uint64(slotInt)

	// put
	jsHeader, _ := json.Marshal(header)
	err = ds.PutHeader(ctx, structs.HeaderData{
		Slot:           slot,
		HeaderAndTrace: header,
		Marshaled:      jsHeader,
	}, time.Minute)
	require.NoError(t, err)

	// get
	gotHeader, err := ds.GetHeadersBySlot(ctx, uint64(slot))
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader[0].Trace.Value)
	require.EqualValues(t, *header.Header, *gotHeader[0].Header)

	// get by block hash
	gotHeader, err = ds.GetHeadersByBlockHash(ctx, header.Header.BlockHash)
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader[0].Trace.Value)
	require.EqualValues(t, *header.Header, *gotHeader[0].Header)
	require.Len(t, gotHeader, 1)
	require.EqualValues(t, *header.Header, *gotHeader[0].Header)

	// get by block number
	gotHeader, err = ds.GetHeadersByBlockNum(ctx, header.Header.BlockNumber)
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader[0].Trace.Value)
	require.EqualValues(t, *header.Header, *gotHeader[0].Header)
}

func TestPutGetHeaderDuplicate(t *testing.T) {
	t.Parallel()

	const N = 10

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := badger.NewDatastore("/tmp/BadgerBatcher2", &badger.DefaultOptions)
	require.NoError(t, err)

	hc := datastore.NewHeaderController(200, time.Hour)
	ds, err := datastore.NewDatastore(store, store.DB, hc, 100)
	require.NoError(t, err)

	header := randomHeaderAndTrace()
	slotInt := rand.Int()
	slot := structs.Slot(slotInt)

	header.Trace.Slot = uint64(slotInt)
	for i := 0; i < N; i++ {
		// put
		jsHeader, _ := json.Marshal(header)
		err = ds.PutHeader(ctx, structs.HeaderData{
			Slot:           slot,
			HeaderAndTrace: header,
			Marshaled:      jsHeader,
		}, time.Minute)

		require.NoError(t, err)
	}

	// get
	gotHeaders, err := ds.GetHeadersBySlot(ctx, uint64(slot))
	require.NoError(t, err)
	require.Len(t, gotHeaders, 1)
	require.EqualValues(t, header.Trace.Value, gotHeaders[0].Trace.Value)
	require.EqualValues(t, *header.Header, *gotHeaders[0].Header)
}

func TestPutGetHeaders(t *testing.T) {
	t.Parallel()

	const N = 100

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := badger.NewDatastore("/tmp/BadgerBatcher3", &badger.DefaultOptions)
	require.NoError(t, err)

	hc := datastore.NewHeaderController(200, time.Hour)
	ds, err := datastore.NewDatastore(store, store.DB, hc, 100)
	require.NoError(t, err)

	headers := make([]structs.HeaderAndTrace, N)
	slots := make([]structs.Slot, N)

	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		go func(i int) {
			header := randomHeaderAndTrace()
			slotInt := rand.Int()
			slot := structs.Slot(slotInt)

			header.Trace.Slot = uint64(slotInt)

			jsHeader, _ := json.Marshal(header)
			err = ds.PutHeader(ctx, structs.HeaderData{
				Slot:           slot,
				HeaderAndTrace: header,
				Marshaled:      jsHeader,
			}, time.Minute)

			require.NoError(t, err)
			headers[i] = header
			slots[i] = slot
			wg.Done()
		}(i)
		wg.Add(1)
	}

	wg.Wait()

	for i := 0; i < N; i++ {
		slot := slots[i]
		header := headers[i]

		// get
		gotHeader, err := ds.GetHeadersBySlot(ctx, uint64(slot))
		require.NoError(t, err)
		require.EqualValues(t, header.Trace.Value, gotHeader[0].Trace.Value)
		require.EqualValues(t, *header.Header, *gotHeader[0].Header)

		// get by block hash
		gotHeader, err = ds.GetHeadersByBlockHash(ctx, header.Header.BlockHash)
		require.NoError(t, err)
		require.EqualValues(t, header.Trace.Value, gotHeader[0].Trace.Value)
		require.EqualValues(t, *header.Header, *gotHeader[0].Header)

		// get by block number
		gotHeader, err = ds.GetHeadersByBlockNum(ctx, header.Header.BlockNumber)
		require.NoError(t, err)
		require.EqualValues(t, header.Trace.Value, gotHeader[0].Trace.Value)
		require.EqualValues(t, *header.Header, *gotHeader[0].Header)
	}
}

func TestPutGetHeaderBatch(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const N = 10

	batch := make([]structs.HeaderAndTrace, 0)
	slots := make([]structs.Slot, 0)

	slotA := 123455678

	for i := 0; i < N; i++ {
		header := randomHeaderAndTrace()
		slotInt := slotA + i*1000
		slot := structs.Slot(slotInt)

		header.Trace.Slot = uint64(slotInt)

		batch = append(batch, header)
		slots = append(slots, slot)
	}

	sort.Slice(batch, func(i, j int) bool {
		return batch[i].Trace.Slot < batch[j].Trace.Slot
	})

	t.Run("Mock", func(t *testing.T) {
		store, err := badger.NewDatastore("/tmp/BadgerBatcher5", &badger.DefaultOptions)
		require.NoError(t, err)

		hc := datastore.NewHeaderController(200, time.Hour)
		ds, err := datastore.NewDatastore(store, store.DB, hc, 100)
		require.NoError(t, err)
		for i, payload := range batch {
			jsHeader, _ := json.Marshal(payload)
			err := ds.PutHeader(ctx, structs.HeaderData{
				Slot:           slots[i],
				HeaderAndTrace: payload,
				Marshaled:      jsHeader,
			}, time.Minute)
			require.NoError(t, err)
		}
		// get
		gotBatch, err := ds.GetLatestHeaders(ctx, 400, uint64(24*time.Hour/time.Second*12))
		require.NoError(t, err)
		sort.Slice(gotBatch, func(i, j int) bool {
			return gotBatch[i].Trace.Slot < gotBatch[j].Trace.Slot
		})
		require.Len(t, gotBatch, len(batch))
		require.EqualValues(t, batch, gotBatch)
	})

	t.Run("DatastoreBatcher", func(t *testing.T) {

		store, err := badger.NewDatastore("/tmp/BadgerBatcher6", &badger.DefaultOptions)
		require.NoError(t, err)
		hc := datastore.NewHeaderController(200, time.Hour)
		ds, err := datastore.NewDatastore(store, store.DB, hc, 100)
		require.NoError(t, err)

		for i, payload := range batch {
			jsHeader, _ := json.Marshal(payload)
			err := ds.PutHeader(ctx, structs.HeaderData{
				Slot:           slots[i],
				HeaderAndTrace: payload,
				Marshaled:      jsHeader,
			}, time.Minute)
			require.NoError(t, err)
		}

		//get
		gotBatch, err := ds.GetLatestHeaders(ctx, 500, uint64(24*time.Hour/time.Second*12))
		require.NoError(t, err)
		sort.Slice(gotBatch, func(i, j int) bool {
			return gotBatch[i].Trace.Slot < gotBatch[j].Trace.Slot
		})
		require.Len(t, gotBatch, len(batch))
		require.EqualValues(t, batch, gotBatch)
	})
}

func TestPutGetHeaderBatchDelivered(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const N = 10

	headers := make([]structs.HeaderAndTrace, 0)
	batch := make([]structs.BidTraceWithTimestamp, 0)
	queries := make([]structs.PayloadQuery, 0)

	for i := 0; i < N; i++ {
		header := randomHeaderAndTrace()
		slot := structs.Slot(header.Trace.Slot)

		headers = append(headers, header)
		batch = append(batch, *header.Trace)
		queries = append(queries, structs.PayloadQuery{Slot: slot})
	}

	sort.Slice(batch, func(i, j int) bool {
		return batch[i].Slot < batch[j].Slot
	})

	store, err := badger.NewDatastore("/tmp/BadgerBatcher7", &badger.DefaultOptions)
	require.NoError(t, err)
	hc := datastore.NewHeaderController(200, time.Hour)
	ds, err := datastore.NewDatastore(store, store.DB, hc, 100)
	require.NoError(t, err)

	for i, header := range headers {
		jsHeader, _ := json.Marshal(header)
		err = ds.PutHeader(ctx, structs.HeaderData{
			Slot:           queries[i].Slot,
			HeaderAndTrace: header,
			Marshaled:      jsHeader,
		}, time.Minute)
		require.NoError(t, err)
	}
	// get
	gotBatch, _ := ds.GetDeliveredBatch(ctx, queries)
	require.Len(t, gotBatch, 0)

	for i, header := range headers {
		trace := structs.DeliveredTrace{Trace: *header.Trace, BlockNumber: header.Header.BlockNumber}
		err := ds.PutDelivered(ctx, queries[i].Slot, trace, time.Minute)
		require.NoError(t, err)
	}

	gotBatch, err = ds.GetDeliveredBatch(ctx, queries)
	require.NoError(t, err)
	sort.Slice(gotBatch, func(i, j int) bool {
		return gotBatch[i].BidTrace.Slot < gotBatch[j].BidTrace.Slot
	})
	require.Len(t, gotBatch, len(batch))
	require.EqualValues(t, batch, gotBatch)

}

func TestGetPutComplex(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := badger.NewDatastore("/tmp/Badger1", &badger.DefaultOptions)
	require.NoError(t, err)

	hc := datastore.NewHeaderController(0, 0) // remove immediately
	ds, err := datastore.NewDatastore(store, store.DB, hc, 100)
	require.NoError(t, err)

	header := randomHeaderAndTrace()
	slotInt := rand.Int()
	slot := structs.Slot(slotInt)

	BPk := header.Trace.BuilderPubkey

	// put first element
	v1 := &types.U256Str{}
	v1.FromBig(big.NewInt(10))
	header.Trace.Value = *v1
	header.Trace.Slot = uint64(slotInt)
	jsHeader, _ := json.Marshal(header)
	err = ds.PutHeader(ctx, structs.HeaderData{
		Slot:           slot,
		HeaderAndTrace: header,
		Marshaled:      jsHeader,
	}, time.Hour)
	require.NoError(t, err)

	// put second element
	header2 := randomHeaderAndTrace()
	header2.Trace.Slot = uint64(slotInt)
	v2 := &types.U256Str{}
	v2.FromBig(big.NewInt(12))
	header2.Trace.Value = *v2
	jsHeader, _ = json.Marshal(header2)
	err = ds.PutHeader(ctx, structs.HeaderData{
		Slot:           slot,
		HeaderAndTrace: header2,
		Marshaled:      jsHeader,
	}, time.Hour)
	require.NoError(t, err)

	// put third element
	header3 := randomHeaderAndTrace()
	header3.Trace.Slot = uint64(slotInt)
	header3.Trace.BlockHash = header3.Header.BlockHash
	header3.Trace.BuilderPubkey = BPk
	v3 := &types.U256Str{}
	err = v3.FromBig(big.NewInt(14))
	require.NoError(t, err)
	header3.Trace.Value = *v3
	jsHeader, _ = json.Marshal(header3)

	err = ds.PutHeader(ctx, structs.HeaderData{
		Slot:           slot,
		HeaderAndTrace: header3,
		Marshaled:      jsHeader,
	}, time.Hour)
	require.NoError(t, err)

	// put fourth element
	header4 := randomHeaderAndTrace()
	header4.Trace.Slot = uint64(slotInt)
	header4.Trace.BuilderPubkey = BPk
	v4 := &types.U256Str{}
	v4.FromBig(big.NewInt(3))
	header4.Trace.Value = *v4
	jsHeader, _ = json.Marshal(header4)
	err = ds.PutHeader(ctx, structs.HeaderData{
		Slot:           slot,
		HeaderAndTrace: header4,
		Marshaled:      jsHeader,
	}, time.Hour)
	require.NoError(t, err)

	// get
	gotHeader, err := ds.GetHeadersBySlot(ctx, uint64(slot))
	require.NoError(t, err)

	require.EqualValues(t, *v1, gotHeader[0].Trace.Value)
	require.EqualValues(t, *header.Header, *gotHeader[0].Header)

	require.EqualValues(t, *v2, gotHeader[1].Trace.Value)
	require.EqualValues(t, *header2.Header, *gotHeader[1].Header)

	require.EqualValues(t, *v3, gotHeader[2].Trace.Value)
	require.EqualValues(t, *header3.Header, *gotHeader[2].Header)

	require.EqualValues(t, *v4, gotHeader[3].Trace.Value)
	require.EqualValues(t, *header4.Header, *gotHeader[3].Header)

	// get by block hash
	gotHeader, err = ds.GetHeadersByBlockHash(ctx, header.Header.BlockHash)
	require.NoError(t, err)
	require.EqualValues(t, *v1, gotHeader[0].Trace.Value)
	require.EqualValues(t, *header.Header, *gotHeader[0].Header)

	// get by block number
	gotHeader, err = ds.GetHeadersByBlockNum(ctx, header.Header.BlockNumber)
	require.NoError(t, err)
	require.EqualValues(t, *v1, gotHeader[0].Trace.Value)
	require.EqualValues(t, *header.Header, *gotHeader[0].Header)

	//check right profit
	hnt, err := ds.GetMaxProfitHeader(ctx, uint64(slot))
	require.NoError(t, err)
	require.EqualValues(t, *v2, hnt.Trace.Value)
	require.EqualValues(t, *header2.Header, *hnt.Header)

	// purged from memory
	hnt2, ok := hc.GetMaxProfit(uint64(slot))
	require.True(t, ok)
	require.EqualValues(t, *v2, hnt2.Trace.Value)

	el, max, rev, err := hc.GetSingleSlot(uint64(slot))
	require.NoError(t, err)
	require.Len(t, el, 4)
	require.EqualValues(t, header2.Trace.BlockHash, max)
	require.Equal(t, uint64(4), rev)

	//datastore.InMemorySlotLag = 0
	//datastore.InMemorySlotTimeLag = 0
	time.Sleep(time.Second * 2)
	//InitiateClenup
	slots, ok := hc.CheckForRemoval()
	require.True(t, ok)
	require.Contains(t, slots, uint64(slot))

	err = ds.SaveHeaders(ctx, slots, time.Hour)
	require.NoError(t, err)

	gotHeader, err = ds.GetHeadersBySlot(ctx, uint64(slot))
	require.NoError(t, err)

	require.EqualValues(t, *v1, gotHeader[0].Trace.Value)
	require.EqualValues(t, *header.Header, *gotHeader[0].Header)

	require.EqualValues(t, *v2, gotHeader[1].Trace.Value)
	require.EqualValues(t, *header2.Header, *gotHeader[1].Header)

	require.EqualValues(t, *v3, gotHeader[2].Trace.Value)
	require.EqualValues(t, *header3.Header, *gotHeader[2].Header)

	require.EqualValues(t, *v4, gotHeader[3].Trace.Value)
	require.EqualValues(t, *header4.Header, *gotHeader[3].Header)

	// purged from memory
	_, ok = hc.GetMaxProfit(uint64(slot))
	require.False(t, ok)

	el, max, rev, err = hc.GetSingleSlot(uint64(slot))
	require.NoError(t, err)
	require.Len(t, el, 0)
	empty := [32]byte{}
	require.Equal(t, max, empty)
	require.Equal(t, rev, uint64(0))

	// check corect max after turn off
	h, err := ds.GetMaxProfitHeader(ctx, uint64(slot))
	require.NoError(t, err)
	require.EqualValues(t, hnt2, h)

	// remove wraps
	err = store.Delete(ctx, datastore.HeaderKey(uint64(slot)))
	require.NoError(t, err)

	err = store.Delete(ctx, datastore.HeaderMaxNewKey(uint64(slot)))
	require.NoError(t, err)

	_, err = ds.GetHeadersBySlot(ctx, uint64(slot))
	require.Error(t, datastore.ErrNotFound, err)

	err = ds.FixOrphanHeaders(ctx, uint64(slot), time.Hour)
	require.NoError(t, err)

	har, err := ds.GetHeadersBySlot(ctx, uint64(slot))
	require.NoError(t, err)
	require.Len(t, har, 4)

	// check corect max after turn off
	h, err = ds.GetMaxProfitHeader(ctx, uint64(slot))
	require.NoError(t, err)
	require.EqualValues(t, hnt2, h)

	// put fifth element
	header5 := randomHeaderAndTrace()
	header5.Trace.Slot = uint64(slotInt) + 1
	header5.Trace.BuilderPubkey = BPk
	v5 := &types.U256Str{}
	v5.FromBig(big.NewInt(3))
	header5.Trace.Value = *v5
	jsHeader, _ = json.Marshal(header5)
	err = ds.PutHeader(ctx, structs.HeaderData{
		Slot:           slot + 1,
		HeaderAndTrace: header5,
		Marshaled:      jsHeader,
	}, time.Hour)
	require.NoError(t, err)

	// put sixth element  // should load the state from memory
	header6 := randomHeaderAndTrace()
	header6.Trace.Slot = uint64(slotInt)
	header6.Trace.BuilderPubkey = BPk
	v6 := &types.U256Str{}
	v6.FromBig(big.NewInt(7689))
	header6.Trace.Value = *v6
	jsHeader, _ = json.Marshal(header6)
	err = ds.PutHeader(ctx, structs.HeaderData{
		Slot:           slot,
		HeaderAndTrace: header6,
		Marshaled:      jsHeader,
	}, time.Hour)
	require.NoError(t, err)

	el, max, rev, err = hc.GetSingleSlot(uint64(slot))
	require.NoError(t, err)
	require.Len(t, el, 5)

	require.EqualValues(t, header6.Trace.BlockHash, max)
	require.Equal(t, uint64(2), rev)
}
