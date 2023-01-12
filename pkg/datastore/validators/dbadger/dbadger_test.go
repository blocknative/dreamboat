package dbadger_test

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/blocknative/dreamboat/pkg/datastore/validators/dbadger"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2"
	badger "github.com/ipfs/go-ds-badger2"
	"github.com/stretchr/testify/require"
)

func TestPutGetRegistration(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newMockDatastore()
	cache, _ := lru.New[structs.PayloadKey, *structs.BlockBidAndTrace](10)
	ds := dbadger.Datastore{TTLStorage: store, PayloadCache: cache}

	registration := randomRegistration()
	key := structs.PubKey{PublicKey: registration.Message.Pubkey}

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
	cache, _ := lru.New[structs.PayloadKey, *structs.BlockBidAndTrace](10)
	ds := dbadger.Datastore{TTLStorage: &datastore.TTLDatastoreBatcher{TTLDatastore: store}, PayloadCache: cache}

	registration := randomRegistration()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := ds.PutRegistration(ctx, structs.PubKey{PublicKey: registration.Message.Pubkey}, registration, time.Minute)
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
	cache, _ := lru.New[structs.PayloadKey, *structs.BlockBidAndTrace](10)
	ds := datastore.Datastore{TTLStorage: &datastore.TTLDatastoreBatcher{TTLDatastore: store}, PayloadCache: cache}

	registration := randomRegistration()

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(b.N)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		go func() {
			err := ds.PutRegistration(ctx, structs.PubKey{PublicKey: registration.Message.Pubkey}, registration, time.Minute)
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

	store, _ := dbadger.NewDatastore(datadir, &badger.DefaultOptions)
	cache, _ := lru.New[structs.PayloadKey, *structs.BlockBidAndTrace](10)
	ds := datastore.Datastore{TTLStorage: &datastore.TTLDatastoreBatcher{TTLDatastore: store}, PayloadCache: cache}

	registration := randomRegistration()
	key := structs.PubKey{PublicKey: registration.Message.Pubkey}

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
	key := structs.PubKey{PublicKey: registration.Message.Pubkey}

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
	cache, _ := lru.New[structs.PayloadKey, *structs.BlockBidAndTrace](10)
	ds := datastore.Datastore{TTLStorage: &datastore.TTLDatastoreBatcher{TTLDatastore: store}, PayloadCache: cache}

	registration := randomRegistration()
	key := structs.PubKey{PublicKey: registration.Message.Pubkey}

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

func random48Bytes() (b [48]byte) {
	rand.Read(b[:])
	return b
}

func random32Bytes() (b [32]byte) {
	rand.Read(b[:])
	return b
}

func random20Bytes() (b [20]byte) {
	rand.Read(b[:])
	return b
}

func random96Bytes() (b [96]byte) {
	rand.Read(b[:])
	return b
}

func random256Bytes() (b [256]byte) {
	rand.Read(b[:])
	return b
}
