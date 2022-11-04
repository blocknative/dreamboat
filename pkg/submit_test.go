package relay_test

import (
	"context"
	"sync"
	"testing"
	"time"

	mock_relay "github.com/blocknative/dreamboat/internal/mock/pkg"
	relay "github.com/blocknative/dreamboat/pkg"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/golang/mock/gomock"
	"github.com/lthibault/log"
	"github.com/stretchr/testify/require"
)

func TestRegisterValidator2(t *testing.T) {
	t.Parallel()

	const N = 10

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)

	config := relay.Config{Log: log.New(), Network: "ropsten", TTL: time.Minute}
	r, _ := relay.NewRelay(config)
	ds := &relay.DefaultDatastore{TTLStorage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	relaySigningDomain, err := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		relay.GenesisForkVersionRopsten,
		types.Root{}.String())
	require.NoError(t, err)

	for i := 0; i < 100; i++ {
		go relay.VerifyParallel(relaySigningDomain)
	}

	knownValidators := make(map[types.PubkeyHex]struct{}, N)
	registrations := make([]types.SignedValidatorRegistration, 0, N)
	for i := 0; i < N; i++ {
		registration, _ := validValidatorRegistration(t, relaySigningDomain)
		registrations = append(registrations, *registration)
		knownValidators[registration.Message.Pubkey.PubkeyHex()] = struct{}{}
	}

	bc.EXPECT().IsKnownValidator(gomock.Any()).Return(true, nil).Times(N)

	err = r.RegisterValidator2(ctx, registrations, state{ds: ds, bc: bc})
	require.NoError(t, err)

	for _, registration := range registrations {
		key := relay.PubKey{registration.Message.Pubkey}
		gotRegistration, err := ds.GetRegistration(ctx, key)
		require.NoError(t, err)
		require.EqualValues(t, registration, gotRegistration)
	}

}

func BenchmarkRegisterValidator2(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const N = 10_000

	ctrl := gomock.NewController(b)

	config := relay.Config{Log: log.New(), Network: "ropsten", TTL: 5 * time.Minute}
	r, _ := relay.NewRelay(config)

	ds := &relay.DefaultDatastore{TTLStorage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	validators := make([]types.SignedValidatorRegistration, 0, N)
	knownValidators := make(map[types.PubkeyHex]struct{}, N)

	for i := 0; i < N; i++ {
		relaySigningDomain, _ := relay.ComputeDomain(
			types.DomainTypeAppBuilder,
			relay.GenesisForkVersionRopsten,
			types.Root{}.String())
		registration, _ := validValidatorRegistration(b, relaySigningDomain)
		validators = append(validators, *registration)
		knownValidators[registration.Message.Pubkey.PubkeyHex()] = struct{}{}
	}

	bc.EXPECT().IsKnownValidator(gomock.Any()).Return(true, nil).Times(b.N * N)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := r.RegisterValidator2(ctx, validators, state{ds: ds, bc: bc})
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkRegisterValidator2Parallel(b *testing.B) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const N = 10_000

	ctrl := gomock.NewController(b)

	config := relay.Config{Log: log.New(), Network: "ropsten"}
	r, _ := relay.NewRelay(config)

	ds := &relay.DefaultDatastore{TTLStorage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	validators := make([]types.SignedValidatorRegistration, 0, N)
	knownValidators := make(map[types.PubkeyHex]struct{}, N)

	relaySigningDomain, _ := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		relay.GenesisForkVersionRopsten,
		types.Root{}.String())

	for i := 0; i < 100; i++ {
		go relay.VerifyParallel(relaySigningDomain)
	}

	for i := 0; i < N; i++ {
		registration, _ := validValidatorRegistration(b, relaySigningDomain)
		validators = append(validators, *registration)
		knownValidators[registration.Message.Pubkey.PubkeyHex()] = struct{}{}
	}

	bc.EXPECT().IsKnownValidator(gomock.Any()).Return(true, nil).AnyTimes()
	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(b.N * 3)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		go func() {
			err := r.RegisterValidator2(ctx, validators, state{ds: ds, bc: bc})
			if err != nil {
				panic(err)
			}
			wg.Done()
		}()
	}

	for i := 0; i < b.N; i++ {
		go func() {
			err := r.RegisterValidator2(ctx, validators, state{ds: ds, bc: bc})
			if err != nil {
				panic(err)
			}
			wg.Done()
		}()
	}

	for i := 0; i < b.N; i++ {
		go func() {
			err := r.RegisterValidator2(ctx, validators, state{ds: ds, bc: bc})
			if err != nil {
				panic(err)
			}
			wg.Done()
		}()
	}

}

//2	 839515018 ns/op
