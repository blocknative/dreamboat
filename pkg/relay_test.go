package relay_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	mock_relay "github.com/blocknative/dreamboat/internal/mock/pkg"
	relay "github.com/blocknative/dreamboat/pkg"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/golang/mock/gomock"
	"github.com/lthibault/log"
	"github.com/stretchr/testify/require"
)

func TestRegisterValidator(t *testing.T) {
	t.Parallel()

	const N = 10

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)

	config := relay.Config{Log: log.New(), Network: "ropsten", TTL: time.Minute}
	r, _ := relay.NewRelay(config)
	ds := &relay.DefaultDatastore{Storage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	relaySigningDomain, err := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		relay.GenesisForkVersionRopsten,
		types.Root{}.String())
	require.NoError(t, err)

	knownValidators := make(map[types.PubkeyHex]struct{}, N)
	registrations := make([]types.SignedValidatorRegistration, 0, N)
	for i := 0; i < N; i++ {
		registration, _ := validValidatorRegistration(t, relaySigningDomain)
		registrations = append(registrations, *registration)
		knownValidators[registration.Message.Pubkey.PubkeyHex()] = struct{}{}
	}

	bc.EXPECT().IsKnownValidator(gomock.Any()).Return(true, nil).Times(N)

	err = r.RegisterValidator(ctx, registrations, state{ds: ds, bc: bc})
	require.NoError(t, err)

	for _, registration := range registrations {
		key := relay.PubKey{registration.Message.Pubkey}
		gotRegistration, err := ds.GetRegistration(ctx, key)
		require.NoError(t, err)
		require.EqualValues(t, registration, gotRegistration)
	}

}

func TestGetHeader(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)

	sk, _, _ := bls.GenerateNewKeypair()
	config := relay.Config{Log: log.New(),
		Network:   "ropsten",
		SecretKey: sk,
		PubKey:    types.PublicKey(random48Bytes())}
	r, _ := relay.NewRelay(config)
	ds := &relay.DefaultDatastore{Storage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	relaySigningDomain, err := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		relay.GenesisForkVersionRopsten,
		types.Root{}.String())
	require.NoError(t, err)
	submitRequest := validSubmitBlockRequest(t, relaySigningDomain)
	registration, _ := validValidatorRegistration(t, relaySigningDomain)

	request := relay.HeaderRequest{}
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
	err = ds.PutHeader(ctx, relay.Slot(submitRequest.Message.Slot),
		relay.HeaderAndTrace{
			Header: header,
			Trace: &relay.BidTraceWithTimestamp{
				BidTrace:  *submitRequest.Message,
				Timestamp: uint64(time.Now().UnixMicro()),
			},
		},
		time.Minute)
	require.NoError(t, err)
	err = ds.PutRegistration(ctx, relay.PubKey{registration.Message.Pubkey}, *registration, time.Minute)
	require.NoError(t, err)

	response, err := r.GetHeader(ctx, request, state{ds: ds, bc: bc})
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
	config := relay.Config{Log: log.New(),
		Network:   "ropsten",
		SecretKey: pk,
		PubKey:    types.PublicKey(random48Bytes()),
		TTL:       time.Minute}
	r, _ := relay.NewRelay(config)
	ds := &relay.DefaultDatastore{Storage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	proposerSigningDomain, err := relay.ComputeDomain(
		types.DomainTypeBeaconProposer,
		relay.BellatrixForkVersionRopsten,
		relay.GenesisValidatorsRootRopsten)
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
	key := relay.PayloadKey{
		BlockHash: request.Message.Body.ExecutionPayloadHeader.BlockHash,
		Proposer:  registration.Message.Pubkey,
		Slot:      relay.Slot(request.Message.Slot),
	}
	err = ds.PutPayload(ctx, key, &payload, time.Minute)
	require.NoError(t, err)
	err = ds.PutHeader(ctx, relay.Slot(submitRequest.Message.Slot),
		relay.HeaderAndTrace{
			Header: header,
			Trace: &relay.BidTraceWithTimestamp{
				BidTrace:  *submitRequest.Message,
				Timestamp: uint64(time.Now().UnixMicro()),
			},
		},
		time.Minute)
	require.NoError(t, err)
	err = ds.PutRegistration(ctx, relay.PubKey{registration.Message.Pubkey}, *registration, time.Minute)
	require.NoError(t, err)

	bc.EXPECT().KnownValidatorByIndex(request.Message.ProposerIndex).Return(registration.Message.Pubkey.PubkeyHex(), nil).Times(1)

	response, err := r.GetPayload(ctx, request, state{ds: ds, bc: bc})
	require.NoError(t, err)

	require.EqualValues(t, submitRequest.ExecutionPayload, response.Data)
}

func TestGetValidators(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	config := relay.Config{Log: log.New(), Network: "ropsten"}
	r, err := relay.NewRelay(config)
	require.NoError(t, err)
	ds := &relay.DefaultDatastore{Storage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	bc.EXPECT().ValidatorsMap().Return(nil).Times(1)

	validators := r.GetValidators(state{ds: ds, bc: bc})
	require.Nil(t, validators)
}

func TestSubmitBlock(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)

	sk, _, _ := bls.GenerateNewKeypair()
	config := relay.Config{Log: log.New(), Network: "ropsten", SecretKey: sk, TTL: time.Minute}
	r, _ := relay.NewRelay(config)
	ds := &relay.DefaultDatastore{Storage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	relaySigningDomain, err := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		relay.GenesisForkVersionRopsten,
		types.Root{}.String())
	require.NoError(t, err)
	submitRequest := validSubmitBlockRequest(t, relaySigningDomain)

	err = r.SubmitBlock(ctx, submitRequest, state{ds: ds, bc: bc})
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
	gotHeader, err := ds.GetHeader(ctx, relay.Query{Slot: relay.Slot(submitRequest.Message.Slot)})
	require.NoError(t, err)
	require.EqualValues(t, header, gotHeader.Header)
}

func BenchmarkRegisterValidator(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const N = 10_000

	ctrl := gomock.NewController(b)

	config := relay.Config{Log: log.New(), Network: "ropsten", TTL: 5 * time.Minute}
	r, _ := relay.NewRelay(config)

	ds := &relay.DefaultDatastore{Storage: newMockDatastore()}
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
		err := r.RegisterValidator(ctx, validators, state{ds: ds, bc: bc})
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkRegisterValidatorParallel(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const N = 10_000

	ctrl := gomock.NewController(b)

	config := relay.Config{Log: log.New(), Network: "ropsten"}
	r, _ := relay.NewRelay(config)

	ds := &relay.DefaultDatastore{Storage: newMockDatastore()}
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

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(b.N)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		go func() {
			err := r.RegisterValidator(ctx, validators, state{ds: ds, bc: bc})
			if err != nil {
				panic(err)
			}
			wg.Done()
		}()
	}
}

func BenchmarkGetHeader(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(b)

	pk, _, _ := bls.GenerateNewKeypair()
	config := relay.Config{Log: log.New(),
		Network:   "ropsten",
		SecretKey: pk,
		PubKey:    types.PublicKey(random48Bytes())}
	r, _ := relay.NewRelay(config)
	ds := &relay.DefaultDatastore{Storage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	proposerSigningDomain, _ := relay.ComputeDomain(
		types.DomainTypeBeaconProposer,
		relay.BellatrixForkVersionRopsten,
		relay.GenesisValidatorsRootRopsten)
	submitRequest := validSubmitBlockRequest(b, proposerSigningDomain)
	registration, _ := validValidatorRegistration(b, proposerSigningDomain)

	request := relay.HeaderRequest{}
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
	_ = ds.PutHeader(ctx, relay.Slot(submitRequest.Message.Slot),
		relay.HeaderAndTrace{
			Header: header,
			Trace: &relay.BidTraceWithTimestamp{
				BidTrace:  *submitRequest.Message,
				Timestamp: uint64(time.Now().UnixMicro()),
			},
		},
		time.Minute)
	_ = ds.PutRegistration(ctx, relay.PubKey{registration.Message.Pubkey}, *registration, time.Minute)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := r.GetHeader(ctx, request, state{ds: ds, bc: bc})
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
	config := relay.Config{Log: log.New(),
		Network:   "ropsten",
		SecretKey: pk,
		PubKey:    types.PublicKey(random48Bytes())}
	r, _ := relay.NewRelay(config)
	ds := &relay.DefaultDatastore{Storage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	proposerSigningDomain, _ := relay.ComputeDomain(
		types.DomainTypeBeaconProposer,
		relay.BellatrixForkVersionRopsten,
		relay.GenesisValidatorsRootRopsten)
	submitRequest := validSubmitBlockRequest(b, proposerSigningDomain)
	registration, _ := validValidatorRegistration(b, proposerSigningDomain)

	request := relay.HeaderRequest{}
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
	_ = ds.PutHeader(ctx, relay.Slot(submitRequest.Message.Slot),
		relay.HeaderAndTrace{
			Header: header,
			Trace: &relay.BidTraceWithTimestamp{
				BidTrace:  *submitRequest.Message,
				Timestamp: uint64(time.Now().UnixMicro()),
			},
		},
		time.Minute)
	_ = ds.PutRegistration(ctx, relay.PubKey{registration.Message.Pubkey}, *registration, time.Minute)

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(b.N)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		go func() {
			_, err := r.GetHeader(ctx, request, state{ds: ds, bc: bc})
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
	config := relay.Config{Log: log.New(),
		Network:   "ropsten",
		SecretKey: pk,
		PubKey:    types.PublicKey(random48Bytes())}
	r, _ := relay.NewRelay(config)
	ds := &relay.DefaultDatastore{Storage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	proposerSigningDomain, _ := relay.ComputeDomain(
		types.DomainTypeBeaconProposer,
		relay.BellatrixForkVersionRopsten,
		relay.GenesisValidatorsRootRopsten)
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
			VoluntaryExits:         []*types.VoluntaryExit{},
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
	key := relay.PayloadKey{
		BlockHash: request.Message.Body.ExecutionPayloadHeader.BlockHash,
		Proposer:  registration.Message.Pubkey,
		Slot:      relay.Slot(request.Message.Slot),
	}
	_ = ds.PutPayload(ctx, key, &payload, time.Minute)
	_ = ds.PutHeader(ctx, relay.Slot(submitRequest.Message.Slot),
		relay.HeaderAndTrace{
			Header: header,
			Trace: &relay.BidTraceWithTimestamp{
				BidTrace:  *submitRequest.Message,
				Timestamp: uint64(time.Now().UnixMicro()),
			},
		},
		time.Minute)
	_ = ds.PutRegistration(ctx, relay.PubKey{registration.Message.Pubkey}, *registration, time.Minute)

	bc.EXPECT().KnownValidatorByIndex(request.Message.ProposerIndex).Return(registration.Message.Pubkey.PubkeyHex(), nil).Times(1)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := r.GetPayload(ctx, request, state{ds: ds, bc: bc})
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
	config := relay.Config{Log: log.New(),
		Network:   "ropsten",
		SecretKey: pk,
		PubKey:    types.PublicKey(random48Bytes())}
	r, _ := relay.NewRelay(config)
	ds := &relay.DefaultDatastore{Storage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	proposerSigningDomain, _ := relay.ComputeDomain(
		types.DomainTypeBeaconProposer,
		relay.BellatrixForkVersionRopsten,
		relay.GenesisValidatorsRootRopsten)
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
			VoluntaryExits:         []*types.VoluntaryExit{},
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
	key := relay.PayloadKey{
		BlockHash: request.Message.Body.ExecutionPayloadHeader.BlockHash,
		Proposer:  registration.Message.Pubkey,
		Slot:      relay.Slot(request.Message.Slot),
	}
	_ = ds.PutPayload(ctx, key, &payload, time.Minute)
	_ = ds.PutHeader(ctx, relay.Slot(submitRequest.Message.Slot),
		relay.HeaderAndTrace{
			Header: header,
			Trace: &relay.BidTraceWithTimestamp{
				BidTrace:  *submitRequest.Message,
				Timestamp: uint64(time.Now().UnixMicro()),
			},
		},
		time.Minute)
	_ = ds.PutRegistration(ctx, relay.PubKey{registration.Message.Pubkey}, *registration, time.Minute)

	bc.EXPECT().KnownValidatorByIndex(request.Message.ProposerIndex).Return(registration.Message.Pubkey.PubkeyHex(), nil).Times(1)

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(b.N)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		go func() {
			_, err := r.GetPayload(ctx, request, state{ds: ds, bc: bc})
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
	config := relay.Config{Log: log.New(), Network: "ropsten", SecretKey: pk}
	r, _ := relay.NewRelay(config)
	ds := &relay.DefaultDatastore{Storage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	relaySigningDomain, _ := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		relay.GenesisForkVersionRopsten,
		types.Root{}.String())
	submitRequest := validSubmitBlockRequest(b, relaySigningDomain)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := r.SubmitBlock(ctx, submitRequest, state{ds: ds, bc: bc})
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
	config := relay.Config{Log: log.New(), Network: "ropsten", SecretKey: pk}
	r, _ := relay.NewRelay(config)
	ds := &relay.DefaultDatastore{Storage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	relaySigningDomain, _ := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		relay.GenesisForkVersionRopsten,
		types.Root{}.String())
	submitRequest := validSubmitBlockRequest(b, relaySigningDomain)

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(b.N)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		go func() {
			err := r.SubmitBlock(ctx, submitRequest, state{ds: ds, bc: bc})
			if err != nil {
				panic(err)
			}
			wg.Done()
		}()
	}
}

func BenchmarkSignatureValidation(b *testing.B) {
	relaySigningDomain, _ := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		relay.GenesisForkVersionRopsten,
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

type state struct {
	ds relay.Datastore
	bc relay.BeaconState
}

func (s state) Datastore() relay.Datastore {
	return s.ds
}

func (s state) Beacon() relay.BeaconState {
	return s.bc
}
