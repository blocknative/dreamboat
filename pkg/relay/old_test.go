package relay_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/blocknative/dreamboat/pkg/datastore"
	mock_relay "github.com/blocknative/dreamboat/pkg/relay/mocks"

	pkg "github.com/blocknative/dreamboat/pkg"
	relay "github.com/blocknative/dreamboat/pkg/relay"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	badger "github.com/ipfs/go-ds-badger2"
	"github.com/lthibault/log"
	"github.com/stretchr/testify/require"
)

func TestOLDRegisterValidator(t *testing.T) {
	t.Parallel()

	const N = 10

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)
	ds := &datastore.Datastore{TTLStorage: newMockDatastore()}
	bs := mock_relay.NewMockState(ctrl)

	relaySigningDomain, err := pkg.ComputeDomain(
		types.DomainTypeAppBuilder,
		pkg.GenesisForkVersionRopsten,
		types.Root{}.String())
	require.NoError(t, err)

	config := relay.RelayConfig{
		TTL:                  time.Minute,
		BuilderSigningDomain: relaySigningDomain,
	}
	r, _ := relay.NewRelay(log.New(), config, bs, ds, nil)

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
	bs.EXPECT().Beacon().Return(fbn).AnyTimes()

	err = r.OLDRegisterValidator(ctx, registrations)
	require.NoError(t, err)

	for _, registration := range registrations {
		key := structs.PubKey{registration.Message.Pubkey}
		gotRegistration, err := ds.GetRegistration(ctx, key)
		require.NoError(t, err)
		registration.Raw = nil
		require.EqualValues(t, registration.Message, gotRegistration.Message)
		require.EqualValues(t, registration.Signature, gotRegistration.Signature)
	}

}

func BenchmarkOLDRegisterValidator(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const N = 10_000

	ctrl := gomock.NewController(b)

	ds := &datastore.Datastore{TTLStorage: newMockDatastore()}
	bs := mock_relay.NewMockState(ctrl)

	relaySigningDomain, _ := pkg.ComputeDomain(
		types.DomainTypeAppBuilder,
		pkg.GenesisForkVersionRopsten,
		types.Root{}.String())

	config := relay.RelayConfig{
		TTL:                  5 * time.Minute,
		BuilderSigningDomain: relaySigningDomain,
	}
	r, _ := relay.NewRelay(log.New(), config, bs, ds, nil)

	registrations := make([]structs.SignedValidatorRegistration, 0, N)
	fbn := &structs.BeaconState{
		ValidatorsState: structs.ValidatorsState{
			KnownValidators: make(map[types.PubkeyHex]struct{}),
		},
	}

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

	// 	bc.EXPECT().IsKnownValidator(gomock.Any()).Return(true, nil).Times(b.N * N)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := r.OLDRegisterValidator(ctx, registrations)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkOLDRegisterValidatorParallel(b *testing.B) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var datadir = "/tmp/" + b.Name() + uuid.New().String()

	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	ds := &datastore.Datastore{TTLStorage: &datastore.TTLDatastoreBatcher{TTLDatastore: store}}

	ctrl := gomock.NewController(b)
	bs := mock_relay.NewMockState(ctrl)

	const N = 10_000

	relaySigningDomain, _ := pkg.ComputeDomain(
		types.DomainTypeAppBuilder,
		pkg.GenesisForkVersionRopsten,
		types.Root{}.String())

	config := relay.RelayConfig{
		TTL:                  5 * time.Minute,
		BuilderSigningDomain: relaySigningDomain,
	}
	r, _ := relay.NewRelay(log.New(), config, bs, ds, nil)

	registrations := make([]structs.SignedValidatorRegistration, 0, N)

	fbn := &structs.BeaconState{
		ValidatorsState: structs.ValidatorsState{
			KnownValidators: make(map[types.PubkeyHex]struct{}),
		},
	}

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
	//bc.EXPECT().IsKnownValidator(gomock.Any()).Return(true, nil).AnyTimes() // Times(b.N * N)

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

	for i := 0; i < b.N; i++ {
		go func() {
			err := r.OLDRegisterValidator(ctx, registrations)
			if err != nil {
				panic(err)
			}
			wg.Done()
		}()
	}

	wg.Wait()
	for i := 0; i < b.N; i++ {
		go func() {
			err := r.OLDRegisterValidator(ctx, registrations)
			if err != nil {
				panic(err)
			}
			wg2.Done()
		}()
	}

	wg2.Wait()
	for i := 0; i < b.N; i++ {
		go func() {
			err := r.OLDRegisterValidator(ctx, registrations)
			if err != nil {
				panic(err)
			}
			wg3.Done()
		}()
	}
}
