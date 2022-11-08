package relay_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	mock_relay "github.com/blocknative/dreamboat/internal/mock/pkg"
	relay "github.com/blocknative/dreamboat/pkg"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	badger "github.com/ipfs/go-ds-badger2"
	"github.com/lthibault/log"
	"github.com/stretchr/testify/require"
)

func TestRegisterValidator2(t *testing.T) {
	t.Parallel()

	const N = 10

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)

	ds := &relay.DefaultDatastore{TTLStorage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	config := relay.Config{Log: log.New(), Network: "ropsten", TTL: time.Minute}
	r, _ := relay.NewRelay(config, ds)

	relaySigningDomain, err := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		relay.GenesisForkVersionRopsten,
		types.Root{}.String())
	require.NoError(t, err)

	knownValidators := make(map[types.PubkeyHex]struct{}, N)
	registrations := make([]relay.SignedValidatorRegistration, 0, N)
	for i := 0; i < N; i++ {
		registration, _ := validValidatorRegistration(t, relaySigningDomain)
		b, err := json.Marshal(registration)
		if err != nil {
			panic(err)
		}
		registrations = append(registrations, relay.SignedValidatorRegistration{SignedValidatorRegistration: *registration, Raw: b})
		knownValidators[registration.Message.Pubkey.PubkeyHex()] = struct{}{}
	}

	bc.EXPECT().IsKnownValidator(gomock.Any()).Return(true, nil).Times(N)

	err = r.RegisterValidator2(ctx, registrations, state{ds: ds, bc: bc})
	require.NoError(t, err)

	for _, registration := range registrations {
		key := relay.PubKey{registration.Message.Pubkey}
		gotRegistration, err := ds.GetRegistration(ctx, key)
		require.NoError(t, err)
		require.EqualValues(t, registration.SignedValidatorRegistration, gotRegistration)
	}

}

func BenchmarkRegisterValidator2(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const N = 10_000

	ctrl := gomock.NewController(b)

	ds := &relay.DefaultDatastore{TTLStorage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	config := relay.Config{Log: log.New(), Network: "ropsten", TTL: 5 * time.Minute}
	r, _ := relay.NewRelay(config, ds)

	registrations := make([]relay.SignedValidatorRegistration, 0, N)
	knownValidators := make(map[types.PubkeyHex]struct{}, N)

	for i := 0; i < N; i++ {
		relaySigningDomain, _ := relay.ComputeDomain(
			types.DomainTypeAppBuilder,
			relay.GenesisForkVersionRopsten,
			types.Root{}.String())

		registration, _ := validValidatorRegistration(b, relaySigningDomain)
		b, err := json.Marshal(registration)
		if err != nil {
			panic(err)
		}
		registrations = append(registrations, relay.SignedValidatorRegistration{SignedValidatorRegistration: *registration, Raw: b})

		knownValidators[registration.Message.Pubkey.PubkeyHex()] = struct{}{}
	}

	bc.EXPECT().IsKnownValidator(gomock.Any()).Return(true, nil).Times(b.N * N)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := r.RegisterValidator2(ctx, registrations, state{ds: ds, bc: bc})
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkRegisterValidator2Parallel(b *testing.B) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var datadir = "/tmp/" + b.Name() + uuid.New().String()

	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	ds := &relay.DefaultDatastore{TTLStorage: &relay.TTLDatastoreBatcher{TTLDatastore: store}}

	const N = 10_000

	config := relay.Config{Log: log.New(), Network: "ropsten"}
	r, _ := relay.NewRelay(config, ds)

	bc := &FakeBeaconMock{}

	registrations := make([]relay.SignedValidatorRegistration, 0, N)
	knownValidators := make(map[types.PubkeyHex]struct{}, N)

	relaySigningDomain, _ := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		relay.GenesisForkVersionRopsten,
		types.Root{}.String())

	for i := 0; i < N; i++ {
		registration, _ := validValidatorRegistration(b, relaySigningDomain)
		b, err := json.Marshal(registration)
		if err != nil {
			panic(err)
		}
		registrations = append(registrations, relay.SignedValidatorRegistration{SignedValidatorRegistration: *registration, Raw: b})
		knownValidators[registration.Message.Pubkey.PubkeyHex()] = struct{}{}
	}

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
			err := r.RegisterValidator2(ctx, registrations, state{ds: ds, bc: bc})
			if err != nil {
				panic(err)
			}
			wg.Done()
		}()
	}

	wg.Wait()
	for i := 0; i < b.N; i++ {
		go func() {
			err := r.RegisterValidator2(ctx, registrations, state{ds: ds, bc: bc})
			if err != nil {
				panic(err)
			}
			wg2.Done()
		}()
	}

	wg2.Wait()
	for i := 0; i < b.N; i++ {
		go func() {
			err := r.RegisterValidator2(ctx, registrations, state{ds: ds, bc: bc})
			if err != nil {
				panic(err)
			}
			wg3.Done()
		}()
	}

}

type FakeBeaconMock struct {
}

func (fbm *FakeBeaconMock) IsKnownValidator(arg0 types.PubkeyHex) (bool, error) {
	return true, nil
}

func (fbm *FakeBeaconMock) HeadSlot() relay.Slot {
	return relay.Slot(0)
}

func (fbm *FakeBeaconMock) ValidatorsMap() relay.BuilderGetValidatorsResponseEntrySlice {
	return nil
}

func (fbm *FakeBeaconMock) KnownValidatorByIndex(uint64) (types.PubkeyHex, error) {
	return types.PubkeyHex(""), nil
}

//2	 839515018 ns/op
