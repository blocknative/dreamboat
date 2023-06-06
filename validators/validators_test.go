package validators_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/blocknative/dreamboat/beacon"
	"github.com/blocknative/dreamboat/blstools"
	dbadger "github.com/blocknative/dreamboat/datastore/validator/badger"
	"github.com/blocknative/dreamboat/structs"
	"github.com/blocknative/dreamboat/test/common"
	"github.com/blocknative/dreamboat/validators"
	"github.com/blocknative/dreamboat/verify"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2"
	badger "github.com/ipfs/go-ds-badger2"
	"github.com/lthibault/log"
	"github.com/stretchr/testify/require"
)

func TestGetValidators(t *testing.T) {
	t.Parallel()

	l := log.New()
	ver := verify.NewVerificationManager(l, 20)
	ver.RunVerify(300)

	relaySigningDomain, err := common.ComputeDomain(
		types.DomainTypeAppBuilder,
		types.Root{}.String())
	require.NoError(t, err)

	s := &beacon.MultiSlotState{}
	r := validators.NewRegister(l, relaySigningDomain, s, ver, nil)

	dutiesState := structs.DutiesState{
		ProposerDutiesResponse: structs.BuilderGetValidatorsResponseEntrySlice{{
			Slot:  0,
			Entry: &types.SignedValidatorRegistration{},
		}},
	}
	s.SetDuties(dutiesState)

	validators := r.GetValidators(structs.NewMetricGroup(4))
	require.NotNil(t, validators)
}

func TestRegisterValidator(t *testing.T) {
	t.Parallel()

	const N = 100

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var datadir = "/tmp/" + t.Name() + uuid.New().String()
	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	ds := dbadger.NewDatastore(store, time.Minute)

	relaySigningDomain, err := common.ComputeDomain(
		types.DomainTypeAppBuilder,
		types.Root{}.String())
	require.NoError(t, err)

	l := log.New()

	vc, err := lru.New[types.PublicKey, structs.ValidatorCacheEntry](10)
	require.NoError(t, err)
	storeMgr := validators.NewStoreManager(l, vc, ds, 20, 20000)
	storeMgr.RunStore(300)

	ver := verify.NewVerificationManager(l, 20000)
	ver.RunVerify(300)

	validatorsState := structs.ValidatorsState{
		KnownValidators: make(map[types.PubkeyHex]struct{}),
	}

	registrations := make([]types.SignedValidatorRegistration, 0, N)
	for i := 0; i < N; i++ {
		registration, _ := validValidatorRegistration(t, relaySigningDomain)
		registrations = append(registrations, *registration)

		validatorsState.KnownValidators[registration.Message.Pubkey.PubkeyHex()] = struct{}{}
	}
	s := &beacon.MultiSlotState{}
	s.SetKnownValidators(validatorsState)

	vr := validators.NewRegister(l, relaySigningDomain, s, ver, storeMgr)
	err = vr.RegisterValidator(ctx, structs.NewMetricGroup(4), registrations)
	require.NoError(t, err)

	time.Sleep(1 * time.Second)
	for _, registration := range registrations {
		gotRegistration, err := ds.GetRegistration(ctx, registration.Message.Pubkey)
		require.NoError(t, err)
		require.EqualValues(t, registration, gotRegistration)
	}
}

func TestBrokenSignatureRegisterValidator(t *testing.T) {
	t.Parallel()

	const N = 10000

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var datadir = "/tmp/" + t.Name() + uuid.New().String()
	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	ds := dbadger.NewDatastore(store, time.Minute)

	relaySigningDomain, err := common.ComputeDomain(
		types.DomainTypeAppBuilder,
		types.Root{}.String())
	require.NoError(t, err)

	l := log.New()

	vc, err := lru.New[types.PublicKey, structs.ValidatorCacheEntry](10)
	require.NoError(t, err)
	storeMgr := validators.NewStoreManager(l, vc, ds, 20, 20000)
	storeMgr.RunStore(300)

	ver := verify.NewVerificationManager(l, 20000)
	ver.RunVerify(300)

	s := &beacon.MultiSlotState{}
	r := validators.NewRegister(l, relaySigningDomain, s, ver, storeMgr)
	validatorsState := structs.ValidatorsState{
		KnownValidators: make(map[types.PubkeyHex]struct{}),
	}

	registrations := make([]types.SignedValidatorRegistration, 0, N)
	for i := 0; i < N; i++ {
		registration, _ := validValidatorRegistration(t, relaySigningDomain)
		registrations = append(registrations, *registration)

		validatorsState.KnownValidators[registration.Message.Pubkey.PubkeyHex()] = struct{}{}
	}

	registrations[N/2].Signature = types.Signature{}
	s.SetKnownValidators(validatorsState)

	err = r.RegisterValidator(ctx, structs.NewMetricGroup(4), registrations)
	require.Error(t, err)
	//t.Logf("returned %s", err.Error())
	time.Sleep(3 * time.Second)

	var errored bool
	for i, registration := range registrations {
		gotRegistration, err := ds.GetRegistration(ctx, registration.Message.Pubkey)
		if !errored {
			if i != N/2 {
				if err == nil || err.Error() != "not found" {
					require.NoError(t, err)
					require.EqualValues(t, registration, gotRegistration)
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

	var datadir = "/tmp/" + t.Name() + uuid.New().String()
	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	ds := dbadger.NewDatastore(store, time.Minute)

	relaySigningDomain, err := common.ComputeDomain(
		types.DomainTypeAppBuilder,
		types.Root{}.String())
	require.NoError(t, err)

	l := log.New()

	vc, err := lru.New[types.PublicKey, structs.ValidatorCacheEntry](10)
	require.NoError(t, err)
	storeMgr := validators.NewStoreManager(l, vc, ds, 20, 20000)
	storeMgr.RunStore(300)

	ver := verify.NewVerificationManager(l, 20000)
	ver.RunVerify(300)

	s := &beacon.MultiSlotState{}

	r := validators.NewRegister(l, relaySigningDomain, s, ver, storeMgr)
	validatorsState := structs.ValidatorsState{
		KnownValidators: make(map[types.PubkeyHex]struct{}),
	}

	registrations := make([]types.SignedValidatorRegistration, 0, N)
	for i := 0; i < N; i++ {
		registration, _ := validValidatorRegistration(t, relaySigningDomain)
		registrations = append(registrations, *registration)
		if i != N/2 {
			validatorsState.KnownValidators[registration.Message.Pubkey.PubkeyHex()] = struct{}{}
		}

	}

	s.SetKnownValidators(validatorsState)
	err = r.RegisterValidator(ctx, structs.NewMetricGroup(4), registrations)
	require.Error(t, err)
	//t.Logf("returned %s", err.Error())
}

func BenchmarkRegisterValidator(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const N = 10_000

	var datadir = "/tmp/bench" + uuid.New().String()
	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	ds := dbadger.NewDatastore(store, 5*time.Minute)

	relaySigningDomain, _ := common.ComputeDomain(
		types.DomainTypeAppBuilder,
		types.Root{}.String())

	l := log.New()

	vc, err := lru.New[types.PublicKey, structs.ValidatorCacheEntry](10)
	require.NoError(b, err)
	storeMgr := validators.NewStoreManager(l, vc, ds, 20, 20000)
	storeMgr.RunStore(300)

	ver := verify.NewVerificationManager(l, 20000)
	ver.RunVerify(300)

	s := &beacon.MultiSlotState{}

	r := validators.NewRegister(l, relaySigningDomain, s, ver, storeMgr)

	validatorsState := structs.ValidatorsState{
		KnownValidators: make(map[types.PubkeyHex]struct{}),
	}

	registrations := make([]types.SignedValidatorRegistration, 0, N)
	for i := 0; i < N; i++ {
		registration, _ := validValidatorRegistration(b, relaySigningDomain)
		registrations = append(registrations, *registration)
		validatorsState.KnownValidators[registration.Message.Pubkey.PubkeyHex()] = struct{}{}
	}
	s.SetKnownValidators(validatorsState)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := r.RegisterValidator(ctx, structs.NewMetricGroup(4), registrations)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkRegisterValidatorParallel(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var datadir = "/tmp/bench" + uuid.New().String()
	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	ds := dbadger.NewDatastore(store, 5*time.Minute)

	relaySigningDomain, _ := common.ComputeDomain(
		types.DomainTypeAppBuilder,
		types.Root{}.String())

	l := log.New()

	vc, err := lru.New[types.PublicKey, structs.ValidatorCacheEntry](10)
	require.NoError(b, err)
	storeMgr := validators.NewStoreManager(l, vc, ds, 20, 20000)
	storeMgr.RunStore(300)

	ver := verify.NewVerificationManager(l, 20000)
	ver.RunVerify(300)

	const N = 10_000

	s := &beacon.MultiSlotState{}
	r := validators.NewRegister(l, relaySigningDomain, s, ver, storeMgr)
	validatorsState := structs.ValidatorsState{
		KnownValidators: make(map[types.PubkeyHex]struct{}),
	}

	registrations := make([]types.SignedValidatorRegistration, 0, N)
	for i := 0; i < N; i++ {
		registration, _ := validValidatorRegistration(b, relaySigningDomain)
		registrations = append(registrations, *registration)
		validatorsState.KnownValidators[registration.Message.Pubkey.PubkeyHex()] = struct{}{}
	}
	s.SetKnownValidators(validatorsState)

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
			err := r.RegisterValidator(ctx, structs.NewMetricGroup(4), registrations)
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
			err := r.RegisterValidator(ctx, structs.NewMetricGroup(4), registrations)
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
			err := r.RegisterValidator(ctx, structs.NewMetricGroup(4), registrations)
			b.Logf(" RegisterValidator %s", time.Since(t).String())
			if err != nil {
				panic(err)
			}
			wg3.Done()
		}()
	}

}

func validValidatorRegistration(t require.TestingT, domain types.Domain) (*types.SignedValidatorRegistration, *bls.SecretKey) {
	sk, pubKey, err := blstools.GenerateNewKeypair()
	require.NoError(t, err)

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
