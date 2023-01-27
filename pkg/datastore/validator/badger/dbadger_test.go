package dbadger_test

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	dbadger "github.com/blocknative/dreamboat/pkg/datastore/validator/badger"

	tBadger "github.com/blocknative/dreamboat/pkg/datastore/transport/badger"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestPutGetRegistration(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tB, err := tBadger.Open("/tmp/" + t.Name() + uuid.New().String())
	require.NoError(t, err)
	defer tB.Close()

	ds := dbadger.NewDatastore(tB, time.Minute)
	registration := randomRegistration()
	key := structs.PubKey{PublicKey: registration.Message.Pubkey}

	// put
	err = ds.PutNewerRegistration(ctx, key, registration)
	require.NoError(t, err)

	// get
	gotRegistration, err := ds.GetRegistration(ctx, key)
	require.NoError(t, err)
	require.EqualValues(t, registration, gotRegistration)
}

func BenchmarkPutRegistration(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tB, err := tBadger.Open("/tmp/" + b.Name() + uuid.New().String())
	require.NoError(b, err)
	defer tB.Close()

	ds := dbadger.NewDatastore(tB, time.Minute)

	registration := randomRegistration()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := ds.PutNewerRegistration(ctx, structs.PubKey{PublicKey: registration.Message.Pubkey}, registration)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkPutRegistrationParallel(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tB, err := tBadger.Open("/tmp/" + b.Name() + uuid.New().String())
	require.NoError(b, err)
	defer tB.Close()

	ds := dbadger.NewDatastore(tB, time.Minute)

	registration := randomRegistration()

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(b.N)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		go func() {
			err := ds.PutNewerRegistration(ctx, structs.PubKey{PublicKey: registration.Message.Pubkey}, registration)
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

	tB, err := tBadger.Open("/tmp/" + b.Name() + uuid.New().String())
	require.NoError(b, err)
	defer tB.Close()

	ds := dbadger.NewDatastore(tB, time.Minute)

	registration := randomRegistration()
	key := structs.PubKey{PublicKey: registration.Message.Pubkey}

	_ = ds.PutNewerRegistration(ctx, key, registration)

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

	tB, err := tBadger.Open("/tmp/" + b.Name() + uuid.New().String())
	require.NoError(b, err)
	defer tB.Close()

	ds := dbadger.NewDatastore(tB, time.Minute)
	registration := randomRegistration()
	key := structs.PubKey{PublicKey: registration.Message.Pubkey}

	_ = ds.PutNewerRegistration(ctx, key, registration)

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

func random20Bytes() (b [20]byte) {
	rand.Read(b[:])
	return b
}

func random96Bytes() (b [96]byte) {
	rand.Read(b[:])
	return b
}
