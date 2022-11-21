package datastore_test

import (
	"context"
	"encoding/json"
	"math/rand"
	"sync"
	"testing"
	"time"

	pkg "github.com/blocknative/dreamboat/pkg"
	datastore "github.com/blocknative/dreamboat/pkg/datastore"
	realRelay "github.com/blocknative/dreamboat/pkg/relay"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/google/uuid"
	ds "github.com/ipfs/go-datastore"
	goDatastore "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
	badger "github.com/ipfs/go-ds-badger2"
	"github.com/stretchr/testify/require"
)

func TestPutGetHeaderDelivered(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := badger.NewDatastore("/tmp/BadgerBatcher4", &badger.DefaultOptions)
	require.NoError(t, err)

	hc := datastore.NewHeaderController()
	d := datastore.NewDatastore(&datastore.TTLDatastoreBatcher{TTLDatastore: store}, store.DB, hc)

	header := randomHeaderAndTrace()
	slotInt := rand.Int()
	slot := structs.Slot(slotInt)

	header.Trace.Slot = uint64(slotInt)

	// put
	jsHeader, _ := json.Marshal(header)
	err = d.PutHeader(ctx, structs.HR{
		Slot:           slot,
		HeaderAndTrace: header,
		Marshaled:      jsHeader,
	}, time.Minute)
	require.NoError(t, err)

	// get
	_, err = d.GetDelivered(ctx, structs.PayloadQuery{Slot: slot})
	require.ErrorIs(t, err, ds.ErrNotFound)

	// get by block hash
	_, err = d.GetDelivered(ctx, structs.PayloadQuery{BlockHash: header.Trace.BlockHash})
	require.ErrorIs(t, err, ds.ErrNotFound)

	// get by block number
	_, err = d.GetDelivered(ctx, structs.PayloadQuery{BlockNum: header.Header.BlockNumber})
	require.ErrorIs(t, err, ds.ErrNotFound)

	_, err = d.GetDelivered(ctx, structs.PayloadQuery{PubKey: header.Trace.ProposerPubkey})
	require.ErrorIs(t, err, ds.ErrNotFound)

	// set as delivered and retrieve again
	err = d.PutDelivered(ctx, slot, structs.DeliveredTrace{Trace: *header.Trace, BlockNumber: header.Header.BlockNumber}, time.Minute)
	require.NoError(t, err)

	// get
	gotHeader, err := d.GetDelivered(ctx, structs.PayloadQuery{Slot: slot})
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader.BidTrace.Value)

	// get by block hash
	gotHeader, err = d.GetDelivered(ctx, structs.PayloadQuery{BlockHash: header.Trace.BlockHash})
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader.BidTrace.Value)

	// get by block number
	gotHeader, err = d.GetDelivered(ctx, structs.PayloadQuery{BlockNum: header.Header.BlockNumber})
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader.BidTrace.Value)

	gotHeader, err = d.GetDelivered(ctx, structs.PayloadQuery{PubKey: header.Trace.ProposerPubkey})
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader.BidTrace.Value)
}

func TestPutGetPayload(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newMockDatastore()
	ds := datastore.Datastore{TTLStorage: store}

	payload := randomBlockBidAndTrace()

	// put
	key := structs.PayloadKey{
		BlockHash: payload.Trace.Message.BlockHash,
		Proposer:  payload.Trace.Message.ProposerPubkey,
		Slot:      structs.Slot(payload.Trace.Message.Slot),
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
	ds := datastore.Datastore{TTLStorage: store}

	registration := randomRegistration()
	key := structs.PubKey{registration.Message.Pubkey}

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
	ds := datastore.Datastore{TTLStorage: &datastore.TTLDatastoreBatcher{TTLDatastore: store}}

	registration := randomRegistration()
	key := structs.PubKey{registration.Message.Pubkey}

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
	ds := datastore.Datastore{TTLStorage: &datastore.TTLDatastoreBatcher{TTLDatastore: store}}

	registration := randomRegistration()
	key := structs.PubKey{registration.Message.Pubkey}

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
	ds := datastore.Datastore{TTLStorage: &datastore.TTLDatastoreBatcher{TTLDatastore: store}}

	registration := randomRegistration()
	key := structs.PubKey{registration.Message.Pubkey}

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

func TestGetRegistrationReal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var datadir = "/tmp/" + t.Name() + uuid.New().String()

	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	ds := datastore.Datastore{
		TTLStorage: &datastore.TTLDatastoreBatcher{
			TTLDatastore: store},
		Badger: store.DB}

	registration := randomRegistration()
	key := structs.PubKey{registration.Message.Pubkey}

	err := ds.PutRegistration(ctx, key, registration, time.Minute*2)
	require.NoError(t, err)

	regs, err := ds.GetAllRegistration()
	require.NoError(t, err)

	if _, ok := regs[registration.Message.Pubkey.String()]; !ok {
		t.Error("reqistration doesn't exists")
	}
}

func BenchmarkGetRegistrationParallel(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var datadir = "/tmp/" + b.Name() + uuid.New().String()

	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	ds := datastore.Datastore{TTLStorage: &datastore.TTLDatastoreBatcher{TTLDatastore: store}}

	registration := randomRegistration()
	key := structs.PubKey{registration.Message.Pubkey}

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

func randomHeaderAndTrace() structs.HeaderAndTrace {
	block := randomBlockBidAndTrace()
	header, _ := types.PayloadToPayloadHeader(block.Payload.Data)

	return structs.HeaderAndTrace{
		Header: header,
		Trace: &structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace:    *block.Trace.Message,
				BlockNumber: header.BlockNumber,
				NumTx:       999,
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

func randomBlockBidAndTrace() *structs.BlockBidAndTrace {

	sk, _, _ := bls.GenerateNewKeypair()

	pk := types.PublicKey(random48Bytes())

	payload := randomPayload()
	GenesisForkVersionMainnet := "0x00000000"

	relaySigningDomain, _ := pkg.ComputeDomain(
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

	signedBuilderBid, _ := realRelay.SubmitBlockRequestToSignedBuilderBid(
		submitRequest,
		sk,
		&pk,
		relaySigningDomain,
	)

	blockBidAndTrace := realRelay.SubmitBlockRequestToBlockBidAndTrace(signedBuilderBid, submitRequest)

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

var _ datastore.TTLStorage = (*mockDatastore)(nil)

type mockDatastore struct{ goDatastore.Datastore }

func newMockDatastore() mockDatastore {
	return mockDatastore{ds_sync.MutexWrap(goDatastore.NewMapDatastore())}
}

func (d mockDatastore) PutWithTTL(ctx context.Context, key goDatastore.Key, value []byte, ttl time.Duration) error {
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
