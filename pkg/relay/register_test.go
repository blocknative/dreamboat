package relay_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	pkg "github.com/blocknative/dreamboat/pkg"
	"github.com/blocknative/dreamboat/pkg/datastore"
	relay "github.com/blocknative/dreamboat/pkg/relay"
	mock_relay "github.com/blocknative/dreamboat/pkg/relay/mocks"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	badger "github.com/ipfs/go-ds-badger2"
	"github.com/lthibault/log"
	"github.com/stretchr/testify/require"
)

func TestRegisterValidator(t *testing.T) {
	t.Parallel()

	const N = 100

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)

	var datadir = "/tmp/" + t.Name() + uuid.New().String()
	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	ds := &datastore.Datastore{TTLStorage: &datastore.TTLDatastoreBatcher{TTLDatastore: store}}
	bs := mock_relay.NewMockState(ctrl)

	relaySigningDomain, err := pkg.ComputeDomain(
		types.DomainTypeAppBuilder,
		pkg.GenesisForkVersionRopsten,
		types.Root{}.String())
	require.NoError(t, err)

	config := relay.RelayConfig{
		TTL:                     time.Minute,
		BuilderSigningDomain:    relaySigningDomain,
		RegisterValidatorMaxNum: 50_000,
	}

	regMgr := relay.NewProcessManager(20000, 20000)
	regMgr.RunStore(ds, config.TTL, 300)
	regMgr.RunVerify(300)

	r := relay.NewRelay(log.New(), config, bs, ds, regMgr)

	fbn := &structs.BeaconState{
		ValidatorsState: structs.ValidatorsState{
			KnownValidators: make(map[types.PubkeyHex]struct{}),
		},
	}

	registrations := make([]structs.SignedValidatorRegistration, 0, N)
	for i := 0; i < N; i++ {
		registration, _ := validValidatorRegistration(t, relaySigningDomain)
		b, err := json.Marshal(registration)
		if err != nil {
			panic(err)
		}
		registrations = append(registrations, structs.SignedValidatorRegistration{SignedValidatorRegistration: *registration, Raw: b})

		fbn.ValidatorsState.KnownValidators[registration.Message.Pubkey.PubkeyHex()] = struct{}{}
	}
	bs.EXPECT().Beacon().Return(fbn)

	err = r.RegisterValidator(ctx, registrations)
	require.NoError(t, err)

	time.Sleep(1 * time.Second)
	for _, registration := range registrations {
		key := structs.PubKey{registration.Message.Pubkey}
		gotRegistration, err := ds.GetRegistration(ctx, key)
		require.NoError(t, err)
		require.EqualValues(t, registration.SignedValidatorRegistration, gotRegistration)
	}
}

func TestBrokenSignatureRegisterValidator(t *testing.T) {
	t.Parallel()

	const N = 10000

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)

	var datadir = "/tmp/" + t.Name() + uuid.New().String()
	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	ds := &datastore.Datastore{TTLStorage: &datastore.TTLDatastoreBatcher{TTLDatastore: store}}
	bs := mock_relay.NewMockState(ctrl)

	relaySigningDomain, err := pkg.ComputeDomain(
		types.DomainTypeAppBuilder,
		pkg.GenesisForkVersionRopsten,
		types.Root{}.String())
	require.NoError(t, err)

	config := relay.RelayConfig{
		TTL:                     time.Minute,
		BuilderSigningDomain:    relaySigningDomain,
		RegisterValidatorMaxNum: 150_000,
	}

	regMgr := relay.NewProcessManager(20000, 20000)
	regMgr.RunStore(ds, config.TTL, 300)
	regMgr.RunVerify(300)

	r := relay.NewRelay(log.New(), config, bs, ds, regMgr)
	fbn := &structs.BeaconState{
		ValidatorsState: structs.ValidatorsState{
			KnownValidators: make(map[types.PubkeyHex]struct{}),
		},
	}

	registrations := make([]structs.SignedValidatorRegistration, 0, N)
	for i := 0; i < N; i++ {
		registration, _ := validValidatorRegistration(t, relaySigningDomain)
		b, err := json.Marshal(registration)
		if err != nil {
			panic(err)
		}
		registrations = append(registrations, structs.SignedValidatorRegistration{SignedValidatorRegistration: *registration, Raw: b})

		fbn.ValidatorsState.KnownValidators[registration.Message.Pubkey.PubkeyHex()] = struct{}{}
	}

	registrations[N/2].Signature = types.Signature{}
	bs.EXPECT().Beacon().Return(fbn)

	err = r.RegisterValidator(ctx, registrations)
	require.Error(t, err)
	t.Logf("returned %s", err.Error())
	time.Sleep(3 * time.Second)

	var errored bool
	for i, registration := range registrations {
		key := structs.PubKey{registration.Message.Pubkey}
		gotRegistration, err := ds.GetRegistration(ctx, key)
		if !errored {
			if i != N/2 {
				if err == nil || err.Error() != "datastore: key not found" {
					require.NoError(t, err)
					require.EqualValues(t, registration.SignedValidatorRegistration, gotRegistration)
				}
			} else {
				errored = true
				require.Error(t, err)
			}
		}
	}
}

func TestNotKnownRegisterValidator(t *testing.T) {
	t.Parallel()

	const N = 10000

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)

	var datadir = "/tmp/" + t.Name() + uuid.New().String()
	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	ds := &datastore.Datastore{TTLStorage: &datastore.TTLDatastoreBatcher{TTLDatastore: store}}
	bs := mock_relay.NewMockState(ctrl)

	relaySigningDomain, err := pkg.ComputeDomain(
		types.DomainTypeAppBuilder,
		pkg.GenesisForkVersionRopsten,
		types.Root{}.String())
	require.NoError(t, err)

	config := relay.RelayConfig{
		TTL:                     time.Minute,
		BuilderSigningDomain:    relaySigningDomain,
		RegisterValidatorMaxNum: 50_000,
	}

	regMgr := relay.NewProcessManager(20000, 20000)
	regMgr.RunStore(ds, config.TTL, 300)
	regMgr.RunVerify(300)

	r := relay.NewRelay(log.New(), config, bs, ds, regMgr)
	fbn := &structs.BeaconState{
		ValidatorsState: structs.ValidatorsState{
			KnownValidators: make(map[types.PubkeyHex]struct{}),
		},
	}

	registrations := make([]structs.SignedValidatorRegistration, 0, N)
	for i := 0; i < N; i++ {
		registration, _ := validValidatorRegistration(t, relaySigningDomain)
		b, err := json.Marshal(registration)
		if err != nil {
			panic(err)
		}
		registrations = append(registrations, structs.SignedValidatorRegistration{SignedValidatorRegistration: *registration, Raw: b})
		if i != N/2 {
			fbn.ValidatorsState.KnownValidators[registration.Message.Pubkey.PubkeyHex()] = struct{}{}
		}

	}

	bs.EXPECT().Beacon().Return(fbn)
	err = r.RegisterValidator(ctx, registrations)
	require.Error(t, err)
	//t.Logf("returned %s", err.Error())
}

func BenchmarkRegisterValidator(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const N = 10_000

	ctrl := gomock.NewController(b)

	ds := mock_relay.NewMockDatastore(ctrl)
	bs := mock_relay.NewMockState(ctrl)

	relaySigningDomain, _ := pkg.ComputeDomain(
		types.DomainTypeAppBuilder,
		pkg.GenesisForkVersionRopsten,
		types.Root{}.String())

	config := relay.RelayConfig{
		TTL:                     5 * time.Minute,
		BuilderSigningDomain:    relaySigningDomain,
		RegisterValidatorMaxNum: 50_000,
	}

	regMgr := relay.NewProcessManager(20000, 20000)
	regMgr.RunStore(ds, config.TTL, 300)
	regMgr.RunVerify(300)

	r := relay.NewRelay(log.New(), config, bs, ds, regMgr)

	fbn := &structs.BeaconState{
		ValidatorsState: structs.ValidatorsState{
			KnownValidators: make(map[types.PubkeyHex]struct{}),
		},
	}

	registrations := make([]structs.SignedValidatorRegistration, 0, N)
	for i := 0; i < N; i++ {
		registration, _ := validValidatorRegistration(b, relaySigningDomain)
		b, err := json.Marshal(registration)
		if err != nil {
			panic(err)
		}
		registrations = append(registrations, structs.SignedValidatorRegistration{SignedValidatorRegistration: *registration, Raw: b})
		fbn.ValidatorsState.KnownValidators[registration.Message.Pubkey.PubkeyHex()] = struct{}{}
	}
	bs.EXPECT().Beacon().Return(fbn).AnyTimes()
	ds.EXPECT().PutRegistrationRaw(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := r.RegisterValidator(ctx, registrations)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkRegisterValidatorParallel(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var datadir = "/tmp/" + b.Name() + uuid.New().String()
	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	ds := &datastore.Datastore{TTLStorage: &datastore.TTLDatastoreBatcher{TTLDatastore: store}}

	relaySigningDomain, _ := pkg.ComputeDomain(
		types.DomainTypeAppBuilder,
		pkg.GenesisForkVersionRopsten,
		types.Root{}.String())

	config := relay.RelayConfig{
		TTL:                     5 * time.Minute,
		BuilderSigningDomain:    relaySigningDomain,
		RegisterValidatorMaxNum: 50_000,
	}

	regMgr := relay.NewProcessManager(20000, 20000)
	regMgr.RunStore(ds, config.TTL, 300)
	regMgr.RunVerify(300)

	ctrl := gomock.NewController(b)
	bs := mock_relay.NewMockState(ctrl)

	const N = 10_000

	r := relay.NewRelay(log.New(), config, bs, ds, regMgr)
	fbn := &structs.BeaconState{
		ValidatorsState: structs.ValidatorsState{
			KnownValidators: make(map[types.PubkeyHex]struct{}),
		},
	}

	registrations := make([]structs.SignedValidatorRegistration, 0, N)
	for i := 0; i < N; i++ {
		registration, _ := validValidatorRegistration(b, relaySigningDomain)
		b, err := json.Marshal(registration)
		if err != nil {
			panic(err)
		}
		registrations = append(registrations, structs.SignedValidatorRegistration{SignedValidatorRegistration: *registration, Raw: b})
		fbn.ValidatorsState.KnownValidators[registration.Message.Pubkey.PubkeyHex()] = struct{}{}
	}
	bs.EXPECT().Beacon().Return(fbn).AnyTimes()

	var wg sync.WaitGroup
	wg.Add(b.N)

	var wg2 sync.WaitGroup
	defer wg2.Wait()
	wg2.Add(b.N)

	var wg3 sync.WaitGroup
	defer wg3.Wait()
	wg3.Add(b.N)

	b.ResetTimer()
	b.ReportAllocs()
	b.Logf(" b.N %d", b.N)

	for i := 0; i < b.N; i++ {
		go func() {
			t := time.Now()
			err := r.RegisterValidator(ctx, registrations)
			b.Logf(" RegisterValidator %s", time.Since(t).String())
			if err != nil {
				panic(err)
			}
			wg.Done()
		}()
	}

	wg.Wait()
	for i := 0; i < b.N; i++ {
		go func() {
			t := time.Now()
			err := r.RegisterValidator(ctx, registrations)
			b.Logf(" RegisterValidator %s", time.Since(t).String())
			if err != nil {
				panic(err)
			}
			wg2.Done()
		}()
	}

	wg2.Wait()
	for i := 0; i < b.N; i++ {
		go func() {
			t := time.Now()
			err := r.RegisterValidator(ctx, registrations)
			b.Logf(" RegisterValidator %s", time.Since(t).String())
			if err != nil {
				panic(err)
			}
			wg3.Done()
		}()
	}

}

func BenchmarkSignatureValidation(b *testing.B) {
	relaySigningDomain, _ := pkg.ComputeDomain(
		types.DomainTypeAppBuilder,
		pkg.GenesisForkVersionRopsten,
		types.Root{}.String())
	registration, _ := validValidatorRegistration(b, relaySigningDomain)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := relay.VerifySignature(
			registration.Message,
			relaySigningDomain,
			registration.Message.Pubkey[:],
			registration.Signature[:])
		if err != nil {
			panic(err)
		}
	}
}
