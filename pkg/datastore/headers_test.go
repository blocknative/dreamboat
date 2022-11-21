package datastore_test

import (
	"context"
	"encoding/json"
	"math/rand"
	"sort"
	"sync"
	"testing"
	"time"

	datastore "github.com/blocknative/dreamboat/pkg/datastore"

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

	hc := datastore.NewHeaderController()
	ds := datastore.NewDatastore(&datastore.TTLDatastoreBatcher{TTLDatastore: store}, store.DB, hc)

	header := randomHeaderAndTrace()
	slotInt := rand.Int()
	slot := structs.Slot(slotInt)

	header.Trace.Slot = uint64(slotInt)

	// put
	jsHeader, _ := json.Marshal(header)
	err = ds.PutHeader(ctx, structs.HR{
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

	hc := datastore.NewHeaderController()
	ds := datastore.NewDatastore(&datastore.TTLDatastoreBatcher{TTLDatastore: store}, store.DB, hc)

	header := randomHeaderAndTrace()
	slotInt := rand.Int()
	slot := structs.Slot(slotInt)

	header.Trace.Slot = uint64(slotInt)
	for i := 0; i < N; i++ {
		// put
		jsHeader, _ := json.Marshal(header)
		err = ds.PutHeader(ctx, structs.HR{
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

	hc := datastore.NewHeaderController()
	ds := datastore.NewDatastore(&datastore.TTLDatastoreBatcher{TTLDatastore: store}, store.DB, hc)

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
			err = ds.PutHeader(ctx, structs.HR{
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

		hc := datastore.NewHeaderController()
		ds := datastore.NewDatastore(&datastore.TTLDatastoreBatcher{TTLDatastore: store}, store.DB, hc)
		for i, payload := range batch {
			jsHeader, _ := json.Marshal(payload)
			err := ds.PutHeader(ctx, structs.HR{
				Slot:           slots[i],
				HeaderAndTrace: payload,
				Marshaled:      jsHeader,
			}, time.Minute)
			require.NoError(t, err)
		}
		// get
		gotBatch, err := ds.GetLatestHeaders(ctx, 400)
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
		hc := datastore.NewHeaderController()
		ds := datastore.NewDatastore(&datastore.TTLDatastoreBatcher{TTLDatastore: store}, store.DB, hc)

		for i, payload := range batch {
			jsHeader, _ := json.Marshal(payload)
			err := ds.PutHeader(ctx, structs.HR{
				Slot:           slots[i],
				HeaderAndTrace: payload,
				Marshaled:      jsHeader,
			}, time.Minute)
			require.NoError(t, err)
		}

		//get
		gotBatch, err := ds.GetLatestHeaders(ctx, 500)
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
	hc := datastore.NewHeaderController()
	ds := datastore.NewDatastore(&datastore.TTLDatastoreBatcher{TTLDatastore: store}, store.DB, hc)

	for i, header := range headers {
		jsHeader, _ := json.Marshal(header)
		err = ds.PutHeader(ctx, structs.HR{
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
