package relay_test

import (
	"context"
	"encoding/json"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/blocknative/dreamboat/pkg/datastore"
	mock_relay "github.com/blocknative/dreamboat/pkg/relay/mocks"
	"github.com/ethereum/go-ethereum/common/hexutil"

	pkg "github.com/blocknative/dreamboat/pkg"
	relay "github.com/blocknative/dreamboat/pkg/relay"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	badger "github.com/ipfs/go-ds-badger2"
	"github.com/lthibault/log"
	"github.com/stretchr/testify/require"

	ds "github.com/ipfs/go-datastore"
	goDatastore "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
)

func TestRegisterValidator(t *testing.T) {
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
	r, _ := relay.NewRelay(log.New(), config, bs, ds)

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

	err = r.RegisterValidator(ctx, registrations)
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

func TestGetHeader(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)
	sk, _, _ := bls.GenerateNewKeypair()

	ds := &datastore.Datastore{TTLStorage: newMockDatastore()}
	bs := mock_relay.NewMockState(ctrl)

	relaySigningDomain, err := pkg.ComputeDomain(
		types.DomainTypeAppBuilder,
		pkg.GenesisForkVersionRopsten,
		types.Root{}.String())

	config := relay.RelayConfig{
		TTL:                  time.Minute,
		BuilderSigningDomain: relaySigningDomain,
		SecretKey:            sk, // pragma: allowlist secret
		PubKey:               types.PublicKey(random48Bytes()),
	}
	r, _ := relay.NewRelay(log.New(), config, bs, ds)

	require.NoError(t, err)
	submitRequest := validSubmitBlockRequest(t, relaySigningDomain)
	registration, _ := validValidatorRegistration(t, relaySigningDomain)

	request := structs.HeaderRequest{}
	request["slot"] = strconv.Itoa(int(submitRequest.Message.Slot))
	request["parent_hash"] = submitRequest.ExecutionPayload.ParentHash.String()
	request["pubkey"] = registration.Message.Pubkey.String()

	signedBuilderBid, _ := relay.SubmitBlockRequestToSignedBuilderBid(
		submitRequest,
		sk,
		&config.PubKey,
		relaySigningDomain,
	)

	payload := relay.SubmitBlockRequestToBlockBidAndTrace(signedBuilderBid, submitRequest)

	// fill the datastore
	key := relay.SubmissionToKey(submitRequest)
	err = ds.PutPayload(ctx, key, &payload, time.Minute)
	require.NoError(t, err)
	header, err := types.PayloadToPayloadHeader(submitRequest.ExecutionPayload)
	require.NoError(t, err)
	err = ds.PutHeader(ctx, structs.Slot(submitRequest.Message.Slot),
		structs.HeaderAndTrace{
			Header: header,
			Trace: &structs.BidTraceWithTimestamp{
				BidTraceExtended: structs.BidTraceExtended{
					BidTrace: *submitRequest.Message,
				},
				Timestamp: uint64(time.Now().UnixMicro()),
			},
		},
		time.Minute)
	require.NoError(t, err)
	err = ds.PutRegistration(ctx, structs.PubKey{registration.Message.Pubkey}, *registration, time.Minute)
	require.NoError(t, err)

	response, err := r.GetHeader(ctx, request)
	require.NoError(t, err)

	require.EqualValues(t, header, response.Data.Message.Header)
	require.EqualValues(t, submitRequest.Message.Value, response.Data.Message.Value)
	require.EqualValues(t, config.PubKey, response.Data.Message.Pubkey)
}

func TestGetPayload(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)

	pk, _, _ := bls.GenerateNewKeypair()

	ds := &datastore.Datastore{TTLStorage: newMockDatastore()}
	bs := mock_relay.NewMockState(ctrl)

	proposerSigningDomain, err := pkg.ComputeDomain(
		types.DomainTypeBeaconProposer,
		pkg.BellatrixForkVersionRopsten,
		pkg.GenesisValidatorsRootRopsten)

	config := relay.RelayConfig{
		SecretKey:            pk, //pragma: allowlist secret
		PubKey:               types.PublicKey(random48Bytes()),
		TTL:                  time.Minute,
		BuilderSigningDomain: proposerSigningDomain,
	}
	r, _ := relay.NewRelay(log.New(), config, bs, ds)

	require.NoError(t, err)
	submitRequest := validSubmitBlockRequest(t, proposerSigningDomain)
	header, err := types.PayloadToPayloadHeader(submitRequest.ExecutionPayload)
	require.NoError(t, err)
	registration, sk := validValidatorRegistration(t, proposerSigningDomain)

	msg := &types.BlindedBeaconBlock{
		Slot:          submitRequest.Message.Slot,
		ProposerIndex: 2,
		ParentRoot:    types.Root{0x03},
		StateRoot:     types.Root{0x04},
		Body: &types.BlindedBeaconBlockBody{
			Eth1Data: &types.Eth1Data{
				DepositRoot:  types.Root{0x05},
				DepositCount: 5,
				BlockHash:    types.Hash{0x06},
			},
			ProposerSlashings:      []*types.ProposerSlashing{},
			AttesterSlashings:      []*types.AttesterSlashing{},
			Attestations:           []*types.Attestation{},
			Deposits:               []*types.Deposit{},
			VoluntaryExits:         []*types.SignedVoluntaryExit{},
			SyncAggregate:          &types.SyncAggregate{types.CommitteeBits{0x07}, types.Signature{0x08}},
			ExecutionPayloadHeader: header,
		},
	}
	signature, err := types.SignMessage(msg, proposerSigningDomain, sk)
	require.NoError(t, err)
	request := &types.SignedBlindedBeaconBlock{
		Message:   msg,
		Signature: signature,
	}
	signedBuilderBid, _ := relay.SubmitBlockRequestToSignedBuilderBid(
		submitRequest,
		pk,
		&config.PubKey,
		proposerSigningDomain,
	)

	payload := relay.SubmitBlockRequestToBlockBidAndTrace(signedBuilderBid, submitRequest)

	// fill the datastore
	key := structs.PayloadKey{
		BlockHash: request.Message.Body.ExecutionPayloadHeader.BlockHash,
		Proposer:  registration.Message.Pubkey,
		Slot:      structs.Slot(request.Message.Slot),
	}
	err = ds.PutPayload(ctx, key, &payload, time.Minute)
	require.NoError(t, err)
	err = ds.PutHeader(ctx, structs.Slot(submitRequest.Message.Slot),
		structs.HeaderAndTrace{
			Header: header,
			Trace: &structs.BidTraceWithTimestamp{
				BidTraceExtended: structs.BidTraceExtended{
					BidTrace: *submitRequest.Message,
				},
				Timestamp: uint64(time.Now().UnixMicro()),
			},
		},
		time.Minute)
	require.NoError(t, err)
	err = ds.PutRegistration(ctx, structs.PubKey{registration.Message.Pubkey}, *registration, time.Minute)
	require.NoError(t, err)

	fbn := &structs.BeaconState{
		ValidatorsState: structs.ValidatorsState{
			KnownValidatorsByIndex: map[uint64]types.PubkeyHex{
				request.Message.ProposerIndex: registration.Message.Pubkey.PubkeyHex(),
			},
		},
	}
	bs.EXPECT().Beacon().Return(fbn).Times(1)

	response, err := r.GetPayload(ctx, request)
	require.NoError(t, err)

	require.EqualValues(t, submitRequest.ExecutionPayload, response.Data)
}

func TestGetValidators(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	ds := &datastore.Datastore{TTLStorage: newMockDatastore()}
	bs := mock_relay.NewMockState(ctrl)

	config := relay.RelayConfig{
		TTL: time.Minute,
	}
	r, err := relay.NewRelay(log.New(), config, bs, ds)

	require.NoError(t, err)

	//bc.EXPECT().ValidatorsMap().Return(nil).Times(1)

	validators := r.GetValidators()
	require.Nil(t, validators)
}

func TestSubmitBlock(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)

	sk, _, _ := bls.GenerateNewKeypair()

	ds := &datastore.Datastore{TTLStorage: newMockDatastore()}
	bs := mock_relay.NewMockState(ctrl)

	relaySigningDomain, err := pkg.ComputeDomain(
		types.DomainTypeAppBuilder,
		pkg.GenesisForkVersionRopsten,
		types.Root{}.String())

	config := relay.RelayConfig{
		TTL:                  time.Minute,
		SecretKey:            sk,
		BuilderSigningDomain: relaySigningDomain,
	}
	r, err := relay.NewRelay(log.New(), config, bs, ds)

	require.NoError(t, err)
	submitRequest := validSubmitBlockRequest(t, relaySigningDomain)

	err = r.SubmitBlock(ctx, submitRequest)
	require.NoError(t, err)

	signedBuilderBid, err := relay.SubmitBlockRequestToSignedBuilderBid(
		submitRequest,
		config.SecretKey,
		&config.PubKey,
		relaySigningDomain)
	require.NoError(t, err)
	payload := relay.SubmitBlockRequestToBlockBidAndTrace(signedBuilderBid, submitRequest)

	key := relay.SubmissionToKey(submitRequest)
	gotPayload, err := ds.GetPayload(ctx, key)
	require.NoError(t, err)
	require.EqualValues(t, payload, *gotPayload)

	header, err := types.PayloadToPayloadHeader(submitRequest.ExecutionPayload)
	require.NoError(t, err)
	gotHeaders, err := ds.GetHeaders(ctx, structs.Query{Slot: structs.Slot(submitRequest.Message.Slot)})
	require.NoError(t, err)
	require.Len(t, gotHeaders, 1)
	require.EqualValues(t, header, gotHeaders[0].Header)
}

func BenchmarkRegisterValidator(b *testing.B) {
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
	r, _ := relay.NewRelay(log.New(), config, bs, ds)

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
	r, _ := relay.NewRelay(log.New(), config, bs, ds)

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
			err := r.RegisterValidator(ctx, registrations)
			if err != nil {
				panic(err)
			}
			wg.Done()
		}()
	}

	wg.Wait()
	for i := 0; i < b.N; i++ {
		go func() {
			err := r.RegisterValidator(ctx, registrations)
			if err != nil {
				panic(err)
			}
			wg2.Done()
		}()
	}

	wg2.Wait()
	for i := 0; i < b.N; i++ {
		go func() {
			err := r.RegisterValidator(ctx, registrations)
			if err != nil {
				panic(err)
			}
			wg3.Done()
		}()
	}
}

func BenchmarkGetHeader(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(b)

	pk, _, _ := bls.GenerateNewKeypair()
	ds := &datastore.Datastore{TTLStorage: newMockDatastore()}
	bs := mock_relay.NewMockState(ctrl)

	proposerSigningDomain, _ := pkg.ComputeDomain(
		types.DomainTypeBeaconProposer,
		pkg.BellatrixForkVersionRopsten,
		pkg.GenesisValidatorsRootRopsten)

	config := relay.RelayConfig{
		TTL:                   5 * time.Minute,
		SecretKey:             pk, // pragma: allowlist secret
		PubKey:                types.PublicKey(random48Bytes()),
		ProposerSigningDomain: proposerSigningDomain,
	}
	r, _ := relay.NewRelay(log.New(), config, bs, ds)

	submitRequest := validSubmitBlockRequest(b, proposerSigningDomain)
	registration, _ := validValidatorRegistration(b, proposerSigningDomain)

	request := structs.HeaderRequest{}
	request["slot"] = strconv.Itoa(int(submitRequest.Message.Slot))
	request["parent_hash"] = submitRequest.ExecutionPayload.ParentHash.String()
	request["pubkey"] = registration.Message.Pubkey.String()

	signedBuilderBid, _ := relay.SubmitBlockRequestToSignedBuilderBid(
		submitRequest,
		pk,
		&config.PubKey,
		proposerSigningDomain,
	)
	payload := relay.SubmitBlockRequestToBlockBidAndTrace(signedBuilderBid, submitRequest)

	// fill the datastore
	key := relay.SubmissionToKey(submitRequest)
	_ = ds.PutPayload(ctx, key, &payload, time.Minute)
	header, _ := types.PayloadToPayloadHeader(submitRequest.ExecutionPayload)
	_ = ds.PutHeader(ctx, structs.Slot(submitRequest.Message.Slot),
		structs.HeaderAndTrace{
			Header: header,
			Trace: &structs.BidTraceWithTimestamp{
				BidTraceExtended: structs.BidTraceExtended{
					BidTrace: *submitRequest.Message,
				},
				Timestamp: uint64(time.Now().UnixMicro()),
			},
		},
		time.Minute)
	_ = ds.PutRegistration(ctx, structs.PubKey{registration.Message.Pubkey}, *registration, time.Minute)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := r.GetHeader(ctx, request)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkGetHeaderParallel(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(b)

	pk, _, _ := bls.GenerateNewKeypair()
	ds := &datastore.Datastore{TTLStorage: newMockDatastore()}
	bs := mock_relay.NewMockState(ctrl)

	proposerSigningDomain, _ := pkg.ComputeDomain(
		types.DomainTypeBeaconProposer,
		pkg.BellatrixForkVersionRopsten,
		pkg.GenesisValidatorsRootRopsten)

	config := relay.RelayConfig{
		TTL:                   5 * time.Minute,
		SecretKey:             pk, // pragma: allowlist secret
		PubKey:                types.PublicKey(random48Bytes()),
		ProposerSigningDomain: proposerSigningDomain,
	}
	r, _ := relay.NewRelay(log.New(), config, bs, ds)

	submitRequest := validSubmitBlockRequest(b, proposerSigningDomain)
	registration, _ := validValidatorRegistration(b, proposerSigningDomain)

	request := structs.HeaderRequest{}
	request["slot"] = strconv.Itoa(int(submitRequest.Message.Slot))
	request["parent_hash"] = submitRequest.ExecutionPayload.ParentHash.String()
	request["pubkey"] = registration.Message.Pubkey.String()

	signedBuilderBid, _ := relay.SubmitBlockRequestToSignedBuilderBid(
		submitRequest,
		pk,
		&config.PubKey,
		proposerSigningDomain,
	)
	payload := relay.SubmitBlockRequestToBlockBidAndTrace(signedBuilderBid, submitRequest)

	// fill the datastore
	key := relay.SubmissionToKey(submitRequest)
	_ = ds.PutPayload(ctx, key, &payload, time.Minute)
	header, _ := types.PayloadToPayloadHeader(submitRequest.ExecutionPayload)
	_ = ds.PutHeader(ctx, structs.Slot(submitRequest.Message.Slot),
		structs.HeaderAndTrace{
			Header: header,
			Trace: &structs.BidTraceWithTimestamp{
				BidTraceExtended: structs.BidTraceExtended{
					BidTrace: *submitRequest.Message,
				},
				Timestamp: uint64(time.Now().UnixMicro()),
			},
		},
		time.Minute)
	_ = ds.PutRegistration(ctx, structs.PubKey{registration.Message.Pubkey}, *registration, time.Minute)

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(b.N)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		go func() {
			_, err := r.GetHeader(ctx, request)
			if err != nil {
				panic(err)
			}
			wg.Done()
		}()
	}
}

func BenchmarkGetPayload(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(b)

	pk, _, _ := bls.GenerateNewKeypair()

	ds := &datastore.Datastore{TTLStorage: newMockDatastore()}
	bs := mock_relay.NewMockState(ctrl)

	proposerSigningDomain, _ := pkg.ComputeDomain(
		types.DomainTypeBeaconProposer,
		pkg.BellatrixForkVersionRopsten,
		pkg.GenesisValidatorsRootRopsten)

	config := relay.RelayConfig{
		TTL:                   5 * time.Minute,
		SecretKey:             pk, // pragma: allowlist secret
		PubKey:                types.PublicKey(random48Bytes()),
		ProposerSigningDomain: proposerSigningDomain,
	}
	r, _ := relay.NewRelay(log.New(), config, bs, ds)

	submitRequest := validSubmitBlockRequest(b, proposerSigningDomain)
	header, _ := types.PayloadToPayloadHeader(submitRequest.ExecutionPayload)
	registration, sk := validValidatorRegistration(b, proposerSigningDomain)

	msg := &types.BlindedBeaconBlock{
		Slot:          submitRequest.Message.Slot,
		ProposerIndex: 2,
		ParentRoot:    types.Root{0x03},
		StateRoot:     types.Root{0x04},
		Body: &types.BlindedBeaconBlockBody{
			Eth1Data: &types.Eth1Data{
				DepositRoot:  types.Root{0x05},
				DepositCount: 5,
				BlockHash:    types.Hash{0x06},
			},
			ProposerSlashings:      []*types.ProposerSlashing{},
			AttesterSlashings:      []*types.AttesterSlashing{},
			Attestations:           []*types.Attestation{},
			Deposits:               []*types.Deposit{},
			VoluntaryExits:         []*types.SignedVoluntaryExit{},
			SyncAggregate:          &types.SyncAggregate{types.CommitteeBits{0x07}, types.Signature{0x08}},
			ExecutionPayloadHeader: header,
		},
	}
	signature, _ := types.SignMessage(msg, proposerSigningDomain, sk)
	request := &types.SignedBlindedBeaconBlock{
		Message:   msg,
		Signature: signature,
	}

	signedBuilderBid, _ := relay.SubmitBlockRequestToSignedBuilderBid(
		submitRequest,
		pk,
		&config.PubKey,
		proposerSigningDomain,
	)
	payload := relay.SubmitBlockRequestToBlockBidAndTrace(signedBuilderBid, submitRequest)

	// fill the datastore
	key := structs.PayloadKey{
		BlockHash: request.Message.Body.ExecutionPayloadHeader.BlockHash,
		Proposer:  registration.Message.Pubkey,
		Slot:      structs.Slot(request.Message.Slot),
	}
	_ = ds.PutPayload(ctx, key, &payload, time.Minute)
	_ = ds.PutHeader(ctx, structs.Slot(submitRequest.Message.Slot),
		structs.HeaderAndTrace{
			Header: header,
			Trace: &structs.BidTraceWithTimestamp{
				BidTraceExtended: structs.BidTraceExtended{
					BidTrace: *submitRequest.Message,
				},
				Timestamp: uint64(time.Now().UnixMicro()),
			},
		},
		time.Minute)
	_ = ds.PutRegistration(ctx, structs.PubKey{registration.Message.Pubkey}, *registration, time.Minute)

	//bc.EXPECT().KnownValidatorByIndex(request.Message.ProposerIndex).Return(registration.Message.Pubkey.PubkeyHex(), nil).Times(1)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := r.GetPayload(ctx, request)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkGetPayloadParallel(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(b)

	pk, _, _ := bls.GenerateNewKeypair()
	ds := &datastore.Datastore{TTLStorage: newMockDatastore()}
	bs := mock_relay.NewMockState(ctrl)

	proposerSigningDomain, _ := pkg.ComputeDomain(
		types.DomainTypeBeaconProposer,
		pkg.BellatrixForkVersionRopsten,
		pkg.GenesisValidatorsRootRopsten)

	config := relay.RelayConfig{
		TTL:                   5 * time.Minute,
		SecretKey:             pk, // pragma: allowlist secret
		PubKey:                types.PublicKey(random48Bytes()),
		ProposerSigningDomain: proposerSigningDomain,
	}
	r, _ := relay.NewRelay(log.New(), config, bs, ds)

	submitRequest := validSubmitBlockRequest(b, proposerSigningDomain)
	header, _ := types.PayloadToPayloadHeader(submitRequest.ExecutionPayload)
	registration, sk := validValidatorRegistration(b, proposerSigningDomain)

	msg := &types.BlindedBeaconBlock{
		Slot:          submitRequest.Message.Slot,
		ProposerIndex: 2,
		ParentRoot:    types.Root{0x03},
		StateRoot:     types.Root{0x04},
		Body: &types.BlindedBeaconBlockBody{
			Eth1Data: &types.Eth1Data{
				DepositRoot:  types.Root{0x05},
				DepositCount: 5,
				BlockHash:    types.Hash{0x06},
			},
			ProposerSlashings:      []*types.ProposerSlashing{},
			AttesterSlashings:      []*types.AttesterSlashing{},
			Attestations:           []*types.Attestation{},
			Deposits:               []*types.Deposit{},
			VoluntaryExits:         []*types.SignedVoluntaryExit{},
			SyncAggregate:          &types.SyncAggregate{types.CommitteeBits{0x07}, types.Signature{0x08}},
			ExecutionPayloadHeader: header,
		},
	}
	signature, _ := types.SignMessage(msg, proposerSigningDomain, sk)
	request := &types.SignedBlindedBeaconBlock{
		Message:   msg,
		Signature: signature,
	}

	signedBuilderBid, _ := relay.SubmitBlockRequestToSignedBuilderBid(
		submitRequest,
		pk,
		&config.PubKey,
		proposerSigningDomain,
	)
	payload := relay.SubmitBlockRequestToBlockBidAndTrace(signedBuilderBid, submitRequest)

	// fill the datastore
	key := structs.PayloadKey{
		BlockHash: request.Message.Body.ExecutionPayloadHeader.BlockHash,
		Proposer:  registration.Message.Pubkey,
		Slot:      structs.Slot(request.Message.Slot),
	}
	_ = ds.PutPayload(ctx, key, &payload, time.Minute)
	_ = ds.PutHeader(ctx, structs.Slot(submitRequest.Message.Slot),
		structs.HeaderAndTrace{
			Header: header,
			Trace: &structs.BidTraceWithTimestamp{
				BidTraceExtended: structs.BidTraceExtended{
					BidTrace: *submitRequest.Message,
				},
				Timestamp: uint64(time.Now().UnixMicro()),
			},
		},
		time.Minute)
	_ = ds.PutRegistration(ctx, structs.PubKey{registration.Message.Pubkey}, *registration, time.Minute)

	//	bc.EXPECT().KnownValidatorByIndex(request.Message.ProposerIndex).Return(registration.Message.Pubkey.PubkeyHex(), nil).Times(1)

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(b.N)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		go func() {
			_, err := r.GetPayload(ctx, request)
			if err != nil {
				panic(err)
			}
			wg.Done()
		}()
	}
}

func BenchmarkSubmitBlock(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(b)

	pk, _, _ := bls.GenerateNewKeypair()

	ds := &datastore.Datastore{TTLStorage: newMockDatastore()}
	bs := mock_relay.NewMockState(ctrl)

	relaySigningDomain, _ := pkg.ComputeDomain(
		types.DomainTypeAppBuilder,
		pkg.GenesisForkVersionRopsten,
		types.Root{}.String())

	config := relay.RelayConfig{
		TTL:                  5 * time.Minute,
		SecretKey:            pk, // pragma: allowlist secret
		PubKey:               types.PublicKey(random48Bytes()),
		BuilderSigningDomain: relaySigningDomain,
	}
	r, _ := relay.NewRelay(log.New(), config, bs, ds)

	submitRequest := validSubmitBlockRequest(b, relaySigningDomain)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := r.SubmitBlock(ctx, submitRequest)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkSubmitBlockParallel(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(b)

	pk, _, _ := bls.GenerateNewKeypair()

	ds := &datastore.Datastore{TTLStorage: newMockDatastore()}
	bs := mock_relay.NewMockState(ctrl)

	relaySigningDomain, _ := pkg.ComputeDomain(
		types.DomainTypeAppBuilder,
		pkg.GenesisForkVersionRopsten,
		types.Root{}.String())

	config := relay.RelayConfig{
		TTL:                  5 * time.Minute,
		SecretKey:            pk, // pragma: allowlist secret
		PubKey:               types.PublicKey(random48Bytes()),
		BuilderSigningDomain: relaySigningDomain,
	}
	r, _ := relay.NewRelay(log.New(), config, bs, ds)

	submitRequest := validSubmitBlockRequest(b, relaySigningDomain)

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(b.N)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		go func() {
			err := r.SubmitBlock(ctx, submitRequest)
			if err != nil {
				panic(err)
			}
			wg.Done()
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
		_, err := types.VerifySignature(
			registration.Message,
			relaySigningDomain,
			registration.Message.Pubkey[:],
			registration.Signature[:])
		if err != nil {
			panic(err)
		}
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

func random256Bytes() (b [256]byte) {
	rand.Read(b[:])
	return b
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

func randomTransactions(size int) []hexutil.Bytes {
	txs := make([]hexutil.Bytes, 0, size)
	for i := 0; i < size; i++ {
		tx := make([]byte, rand.Intn(32))
		rand.Read(tx)
		txs = append(txs, tx)
	}
	return txs
}

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
