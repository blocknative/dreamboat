package relay_test

import (
	"context"
	"math/rand"
	"sort"
	"sync"
	"testing"
	"time"

	relay "github.com/blocknative/dreamboat/pkg"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/google/uuid"
	"github.com/ipfs/go-datastore"
	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
	badger "github.com/ipfs/go-ds-badger2"
	"github.com/stretchr/testify/require"
)

func TestPutGetHeader(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ds := relay.DefaultDatastore{TTLStorage: newMockDatastore()}

	header := randomHeaderAndTrace()
	slotInt := rand.Int()
	slot := relay.Slot(slotInt)

	header.Trace.Slot = uint64(slotInt)

	// put
	err := ds.PutHeader(ctx, slot, header, time.Minute)
	require.NoError(t, err)

	// get
	gotHeader, err := ds.GetHeaders(ctx, relay.Query{Slot: slot})
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader[0].Trace.Value)
	require.EqualValues(t, *header.Header, *gotHeader[0].Header)

	// get by block hash
	gotHeader, err = ds.GetHeaders(ctx, relay.Query{BlockHash: header.Header.BlockHash})
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader[0].Trace.Value)
	require.EqualValues(t, *header.Header, *gotHeader[0].Header)

	// get by block number
	gotHeader, err = ds.GetHeaders(ctx, relay.Query{BlockNum: header.Header.BlockNumber})
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader[0].Trace.Value)
	require.EqualValues(t, *header.Header, *gotHeader[0].Header)
}

func TestPutGetHeaderDuplicate(t *testing.T) {
	t.Parallel()

	const N = 10

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ds := relay.DefaultDatastore{TTLStorage: newMockDatastore()}

	header := randomHeaderAndTrace()
	slotInt := rand.Int()
	slot := relay.Slot(slotInt)

	header.Trace.Slot = uint64(slotInt)
	for i := 0; i < N; i++ {
		// put
		err := ds.PutHeader(ctx, slot, header, time.Minute)
		require.NoError(t, err)
	}

	// get
	gotHeaders, err := ds.GetHeaders(ctx, relay.Query{Slot: slot})
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

	ds := relay.DefaultDatastore{TTLStorage: newMockDatastore()}

	headers := make([]relay.HeaderAndTrace, N)
	slots := make([]relay.Slot, N)

	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		go func(i int) {
			header := randomHeaderAndTrace()
			slotInt := rand.Int()
			slot := relay.Slot(slotInt)

			header.Trace.Slot = uint64(slotInt)
			err := ds.PutHeader(ctx, slot, header, time.Minute)
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
		gotHeader, err := ds.GetHeaders(ctx, relay.Query{Slot: slot})
		require.NoError(t, err)
		require.EqualValues(t, header.Trace.Value, gotHeader[0].Trace.Value)
		require.EqualValues(t, *header.Header, *gotHeader[0].Header)

		// get by block hash
		gotHeader, err = ds.GetHeaders(ctx, relay.Query{BlockHash: header.Header.BlockHash})
		require.NoError(t, err)
		require.EqualValues(t, header.Trace.Value, gotHeader[0].Trace.Value)
		require.EqualValues(t, *header.Header, *gotHeader[0].Header)

		// get by block number
		gotHeader, err = ds.GetHeaders(ctx, relay.Query{BlockNum: header.Header.BlockNumber})
		require.NoError(t, err)
		require.EqualValues(t, header.Trace.Value, gotHeader[0].Trace.Value)
		require.EqualValues(t, *header.Header, *gotHeader[0].Header)
	}
}

func TestPutGetHeaderDelivered(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := relay.DefaultDatastore{TTLStorage: newMockDatastore()}

	header := randomHeaderAndTrace()
	slotInt := rand.Int()
	slot := relay.Slot(slotInt)

	header.Trace.Slot = uint64(slotInt)

	// put
	err := d.PutHeader(ctx, slot, header, time.Minute)
	require.NoError(t, err)

	// get
	_, err = d.GetDelivered(ctx, relay.Query{Slot: slot})
	require.ErrorIs(t, err, ds.ErrNotFound)

	// get by block hash
	_, err = d.GetDelivered(ctx, relay.Query{BlockHash: header.Trace.BlockHash})
	require.ErrorIs(t, err, ds.ErrNotFound)

	// get by block number
	_, err = d.GetDelivered(ctx, relay.Query{BlockNum: header.Header.BlockNumber})
	require.ErrorIs(t, err, ds.ErrNotFound)

	_, err = d.GetDelivered(ctx, relay.Query{PubKey: header.Trace.ProposerPubkey})
	require.ErrorIs(t, err, ds.ErrNotFound)

	// set as delivered and retrieve again
	err = d.PutDelivered(ctx, slot, relay.DeliveredTrace{Trace: *header.Trace, BlockNumber: header.Header.BlockNumber}, time.Minute)
	require.NoError(t, err)

	// get
	gotHeader, err := d.GetDelivered(ctx, relay.Query{Slot: slot})
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader.BidTrace.Value)

	// get by block hash
	gotHeader, err = d.GetDelivered(ctx, relay.Query{BlockHash: header.Trace.BlockHash})
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader.BidTrace.Value)

	// get by block number
	gotHeader, err = d.GetDelivered(ctx, relay.Query{BlockNum: header.Header.BlockNumber})
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader.BidTrace.Value)

	gotHeader, err = d.GetDelivered(ctx, relay.Query{PubKey: header.Trace.ProposerPubkey})
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader.BidTrace.Value)
}

func TestPutGetHeaderBatch(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const N = 10

	batch := make([]relay.HeaderAndTrace, 0)
	queries := make([]relay.Query, 0)

	for i := 0; i < N; i++ {
		header := randomHeaderAndTrace()
		slotInt := rand.Int()
		slot := relay.Slot(slotInt)

		header.Trace.Slot = uint64(slotInt)

		batch = append(batch, header)
		queries = append(queries, relay.Query{Slot: slot})
	}

	sort.Slice(batch, func(i, j int) bool {
		return batch[i].Trace.Slot < batch[j].Trace.Slot
	})

	t.Run("Mock", func(t *testing.T) {
		t.Parallel()

		store := newMockDatastore()
		ds := relay.DefaultDatastore{TTLStorage: store}
		for i, payload := range batch {
			ds.PutHeader(ctx, queries[i].Slot, payload, time.Minute)
		}
		// get
		gotBatch, err := ds.GetHeaderBatch(ctx, queries)
		require.NoError(t, err)
		sort.Slice(gotBatch, func(i, j int) bool {
			return gotBatch[i].Trace.Slot < gotBatch[j].Trace.Slot
		})
		require.Len(t, gotBatch, len(batch))
		require.EqualValues(t, batch, gotBatch)
	})

	t.Run("DatastoreBatcher", func(t *testing.T) {
		t.Parallel()

		store, err := badger.NewDatastore("/tmp/BadgerBatcher", &badger.DefaultOptions)
		require.NoError(t, err)
		ds := relay.DefaultDatastore{TTLStorage: &relay.TTLDatastoreBatcher{TTLDatastore: store}}

		for i, payload := range batch {
			ds.PutHeader(ctx, queries[i].Slot, payload, time.Minute)
		}
		// get
		gotBatch, err := ds.GetHeaderBatch(ctx, queries)
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

	headers := make([]relay.HeaderAndTrace, 0)
	batch := make([]relay.BidTraceWithTimestamp, 0)
	queries := make([]relay.Query, 0)

	for i := 0; i < N; i++ {
		header := randomHeaderAndTrace()
		slot := relay.Slot(header.Trace.Slot)

		headers = append(headers, header)
		batch = append(batch, *header.Trace)
		queries = append(queries, relay.Query{Slot: slot})
	}

	sort.Slice(batch, func(i, j int) bool {
		return batch[i].Slot < batch[j].Slot
	})

	store := newMockDatastore()
	ds := relay.DefaultDatastore{TTLStorage: store}
	for i, header := range headers {
		err := ds.PutHeader(ctx, queries[i].Slot, header, time.Minute)
		require.NoError(t, err)
	}
	// get
	gotBatch, _ := ds.GetDeliveredBatch(ctx, queries)
	require.Len(t, gotBatch, 0)

	for i, header := range headers {
		trace := relay.DeliveredTrace{Trace: *header.Trace, BlockNumber: header.Header.BlockNumber}
		err := ds.PutDelivered(ctx, queries[i].Slot, trace, time.Minute)
		require.NoError(t, err)
	}

	gotBatch, err := ds.GetDeliveredBatch(ctx, queries)
	require.NoError(t, err)
	sort.Slice(gotBatch, func(i, j int) bool {
		return gotBatch[i].BidTrace.Slot < gotBatch[j].BidTrace.Slot
	})
	require.Len(t, gotBatch, len(batch))
	require.EqualValues(t, batch, gotBatch)

}

func TestPutGetPayload(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newMockDatastore()
	ds := relay.DefaultDatastore{TTLStorage: store}

	payload := randomBlockBidAndTrace()

	// put
	key := relay.PayloadKey{
		BlockHash: payload.Trace.Message.BlockHash,
		Proposer:  payload.Trace.Message.ProposerPubkey,
		Slot:      relay.Slot(payload.Trace.Message.Slot),
	}
	err := ds.PutPayload(ctx, key, payload, time.Minute)
	require.NoError(t, err)

	// get
	gotPayload, err := ds.GetPayload(ctx, key)
	require.NoError(t, err)
	require.EqualValues(t, *payload, *gotPayload)
}

func TestPutGetRegistration(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newMockDatastore()
	ds := relay.DefaultDatastore{TTLStorage: store}

	registration := randomRegistration()
	key := relay.PubKey{registration.Message.Pubkey}

	// put
	err := ds.PutRegistration(ctx, key, registration, time.Minute)
	require.NoError(t, err)

	// get
	gotRegistration, err := ds.GetRegistration(ctx, key)
	require.NoError(t, err)
	require.EqualValues(t, registration, gotRegistration)
}

func BenchmarkPutRegistration(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var datadir = "/tmp/" + b.Name() + uuid.New().String()

	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	ds := relay.DefaultDatastore{TTLStorage: &relay.TTLDatastoreBatcher{TTLDatastore: store}}

	registration := randomRegistration()
	key := relay.PubKey{registration.Message.Pubkey}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := ds.PutRegistration(ctx, key, registration, time.Minute)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkPutRegistrationParallel(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var datadir = "/tmp/" + b.Name() + uuid.New().String()

	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	ds := relay.DefaultDatastore{TTLStorage: &relay.TTLDatastoreBatcher{TTLDatastore: store}}

	registration := randomRegistration()
	key := relay.PubKey{registration.Message.Pubkey}

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(b.N)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		go func() {
			err := ds.PutRegistration(ctx, key, registration, time.Minute)
			if err != nil {
				panic(err)
			}
			wg.Done()
		}()
	}
}

func BenchmarkGetRegistration(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var datadir = "/tmp/" + b.Name() + uuid.New().String()

	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	ds := relay.DefaultDatastore{TTLStorage: &relay.TTLDatastoreBatcher{TTLDatastore: store}}

	registration := randomRegistration()
	key := relay.PubKey{registration.Message.Pubkey}

	_ = ds.PutRegistration(ctx, key, registration, time.Minute)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := ds.GetRegistration(ctx, key)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkGetRegistrationParallel(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var datadir = "/tmp/" + b.Name() + uuid.New().String()

	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	ds := relay.DefaultDatastore{TTLStorage: &relay.TTLDatastoreBatcher{TTLDatastore: store}}

	registration := randomRegistration()
	key := relay.PubKey{registration.Message.Pubkey}

	_ = ds.PutRegistration(ctx, key, registration, time.Minute)

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(b.N)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		go func() {
			_, err := ds.GetRegistration(ctx, key)
			if err != nil {
				panic(err)
			}
			wg.Done()
		}()
	}
}

func randomHeaderAndTrace() relay.HeaderAndTrace {
	block := randomBlockBidAndTrace()
	header, _ := types.PayloadToPayloadHeader(block.Payload.Data)

	return relay.HeaderAndTrace{
		Header: header,
		Trace: &relay.BidTraceWithTimestamp{
			BidTraceExtended: relay.BidTraceExtended{
				BidTrace: *block.Trace.Message,
			},
			Timestamp: uint64(time.Now().UnixMicro()),
		},
	}
}

func randomPayload() *types.ExecutionPayload {

	return &types.ExecutionPayload{
		ParentHash:    types.Hash(random32Bytes()),
		FeeRecipient:  types.Address(random20Bytes()),
		StateRoot:     types.Hash(random32Bytes()),
		ReceiptsRoot:  types.Hash(random32Bytes()),
		LogsBloom:     types.Bloom(random256Bytes()),
		Random:        random32Bytes(),
		BlockNumber:   rand.Uint64(),
		GasLimit:      rand.Uint64(),
		GasUsed:       rand.Uint64(),
		Timestamp:     rand.Uint64(),
		ExtraData:     types.ExtraData{},
		BaseFeePerGas: types.IntToU256(rand.Uint64()),
		BlockHash:     types.Hash(random32Bytes()),
		Transactions:  randomTransactions(2),
	}
}

func randomBlockBidAndTrace() *relay.BlockBidAndTrace {

	sk, _, _ := bls.GenerateNewKeypair()

	pk := types.PublicKey(random48Bytes())

	payload := randomPayload()
	GenesisForkVersionMainnet := "0x00000000"

	relaySigningDomain, _ := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		GenesisForkVersionMainnet,
		types.Root{}.String())

	msg := &types.BidTrace{
		Slot:                 rand.Uint64(),
		ParentHash:           types.Hash(random32Bytes()),
		BlockHash:            types.Hash(random32Bytes()),
		BuilderPubkey:        types.PublicKey(random48Bytes()),
		ProposerPubkey:       types.PublicKey(random48Bytes()),
		ProposerFeeRecipient: types.Address(random20Bytes()),
		Value:                types.IntToU256(rand.Uint64()),
	}

	signature, _ := types.SignMessage(msg, relaySigningDomain, sk)

	submitRequest := &types.BuilderSubmitBlockRequest{
		Signature:        signature,
		Message:          msg,
		ExecutionPayload: payload,
	}

	signedBuilderBid, _ := relay.SubmitBlockRequestToSignedBuilderBid(
		submitRequest,
		sk,
		&pk,
		relaySigningDomain,
	)

	blockBidAndTrace := relay.SubmitBlockRequestToBlockBidAndTrace(signedBuilderBid, submitRequest)

	return &blockBidAndTrace
}

func randomTransactions(size int) []hexutil.Bytes {
	txs := make([]hexutil.Bytes, 0, size)
	for i := 0; i < size; i++ {
		tx := make([]byte, rand.Intn(32))
		rand.Read(tx)
		txs = append(txs, tx)
	}
	return txs
}

func randomRegistration() types.SignedValidatorRegistration {
	msg := &types.RegisterValidatorRequestMessage{
		FeeRecipient: types.Address(random20Bytes()),
		GasLimit:     rand.Uint64(),
		Timestamp:    rand.Uint64(),
		Pubkey:       types.PublicKey(random48Bytes()),
	}
	return types.SignedValidatorRegistration{
		Message:   msg,
		Signature: types.Signature(random96Bytes()),
	}
}

func validValidatorRegistration(t require.TestingT, domain types.Domain) (*types.SignedValidatorRegistration, *bls.SecretKey) {
	sk, pk, err := bls.GenerateNewKeypair()
	require.NoError(t, err)

	var pubKey types.PublicKey
	pubKey.FromSlice(pk.Compress())

	msg := &types.RegisterValidatorRequestMessage{
		FeeRecipient: types.Address{0x42},
		GasLimit:     15_000_000,
		Timestamp:    1652369368,
		Pubkey:       pubKey,
	}

	signature, err := types.SignMessage(msg, domain, sk)
	require.NoError(t, err)
	return &types.SignedValidatorRegistration{
		Message:   msg,
		Signature: signature,
	}, sk
}

func validSubmitBlockRequest(t require.TestingT, domain types.Domain) *types.BuilderSubmitBlockRequest {
	sk, pk, err := bls.GenerateNewKeypair()
	require.NoError(t, err)

	var pubKey types.PublicKey
	pubKey.FromSlice(pk.Compress())

	payload := randomPayload()

	msg := &types.BidTrace{
		Slot:                 rand.Uint64(),
		ParentHash:           payload.ParentHash,
		BlockHash:            payload.BlockHash,
		BuilderPubkey:        pubKey,
		ProposerPubkey:       types.PublicKey(random48Bytes()),
		ProposerFeeRecipient: types.Address(random20Bytes()),
		Value:                types.IntToU256(rand.Uint64()),
	}

	signature, err := types.SignMessage(msg, domain, sk)
	require.NoError(t, err)

	return &types.BuilderSubmitBlockRequest{
		Signature:        signature,
		Message:          msg,
		ExecutionPayload: payload,
	}
}

func validSignedBlindedBeaconBlock(t require.TestingT, domain types.Domain) *types.BuilderSubmitBlockRequest {
	sk, pk, err := bls.GenerateNewKeypair()
	require.NoError(t, err)

	var pubKey types.PublicKey
	pubKey.FromSlice(pk.Compress())

	payload := randomPayload()

	msg := &types.BidTrace{
		Slot:                 rand.Uint64(),
		ParentHash:           types.Hash(random32Bytes()),
		BlockHash:            types.Hash(random32Bytes()),
		BuilderPubkey:        types.PublicKey(random48Bytes()),
		ProposerPubkey:       types.PublicKey(random48Bytes()),
		ProposerFeeRecipient: types.Address(random20Bytes()),
		Value:                types.IntToU256(rand.Uint64()),
	}

	signature, err := types.SignMessage(msg, domain, sk)
	require.NoError(t, err)

	return &types.BuilderSubmitBlockRequest{
		Signature:        signature,
		Message:          msg,
		ExecutionPayload: payload,
	}
}

func random32Bytes() (b [32]byte) {
	rand.Read(b[:])
	return b
}

func random48Bytes() (b [48]byte) {
	rand.Read(b[:])
	return b
}

func random96Bytes() (b [96]byte) {
	rand.Read(b[:])
	return b
}

func random20Bytes() (b [20]byte) {
	rand.Read(b[:])
	return b
}

func random256Bytes() (b [256]byte) {
	rand.Read(b[:])
	return b
}

var _ relay.TTLStorage = (*mockDatastore)(nil)

type mockDatastore struct{ datastore.Datastore }

func newMockDatastore() mockDatastore {
	return mockDatastore{ds_sync.MutexWrap(datastore.NewMapDatastore())}
}

func (d mockDatastore) PutWithTTL(ctx context.Context, key datastore.Key, value []byte, ttl time.Duration) error {
	go func() {
		time.Sleep(ttl)
		d.Delete(ctx, key)
	}()

	return d.Datastore.Put(ctx, key, value)
}

func (d mockDatastore) GetBatch(ctx context.Context, keys []ds.Key) (batch [][]byte, err error) {
	for _, key := range keys {
		data, err := d.Datastore.Get(ctx, key)
		if err != nil {
			continue
		}
		batch = append(batch, data)
	}

	return
}
