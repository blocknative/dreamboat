package relay_test

import (
	"context"
	"encoding/json"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

	mock_relay "github.com/blocknative/dreamboat/internal/mock/pkg"
	relay "github.com/blocknative/dreamboat/pkg"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	badger "github.com/ipfs/go-ds-badger2"
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

	ds := &relay.DefaultDatastore{TTLStorage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	r, _ := relay.NewRelay(config, ds)

	relaySigningDomain, err := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		relay.GenesisForkVersionRopsten,
		types.Root{}.String())
	require.NoError(t, err)

	knownValidators := make(map[types.PubkeyHex]struct{}, N)
	registrations := make([]structs.SignedValidatorRegistration, 0, N)
	for i := 0; i < N; i++ {
		registration, _ := validValidatorRegistration(t, relaySigningDomain)
		b, err := json.Marshal(registration)
		if err != nil {
			panic(err)
		}
		registrations = append(registrations, structs.SignedValidatorRegistration{SignedValidatorRegistration: *registration, Raw: b})

		knownValidators[registration.Message.Pubkey.PubkeyHex()] = struct{}{}
	}

	bc.EXPECT().IsKnownValidator(gomock.Any()).Return(true, nil).Times(N)

	err = r.RegisterValidator(ctx, registrations, state{ds: ds, bc: bc})
	require.NoError(t, err)

	for _, registration := range registrations {
		key := structs.PubKey{registration.Message.Pubkey}
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
		SecretKey: sk, // pragma: allowlist secret
		PubKey:    types.PublicKey(random48Bytes())}

	ds := &relay.DefaultDatastore{TTLStorage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	r, _ := relay.NewRelay(config, ds)

	relaySigningDomain, err := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		relay.GenesisForkVersionRopsten,
		types.Root{}.String())
	require.NoError(t, err)

	genesisTime := uint64(time.Now().Unix())
	submitRequest := validSubmitBlockRequest(t, relaySigningDomain, genesisTime)
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
		relay.HeaderAndTrace{
			Header: header,
			Trace: &relay.BidTraceWithTimestamp{
				BidTraceExtended: relay.BidTraceExtended{
					BidTrace: *submitRequest.Message,
				},
				Timestamp: uint64(time.Now().UnixMicro()),
			},
		},
		time.Minute)
	require.NoError(t, err)
	err = ds.PutRegistration(ctx, structs.PubKey{registration.Message.Pubkey}, *registration, time.Minute)
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
		SecretKey: pk, //pragma: allowlist secret
		PubKey:    types.PublicKey(random48Bytes()),
		TTL:       time.Minute}

	ds := &relay.DefaultDatastore{TTLStorage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)
	r, _ := relay.NewRelay(config, ds)

	proposerSigningDomain, err := relay.ComputeDomain(
		types.DomainTypeBeaconProposer,
		relay.BellatrixForkVersionRopsten,
		relay.GenesisValidatorsRootRopsten)
	require.NoError(t, err)
	genesisTime := uint64(time.Now().Unix())
	submitRequest := validSubmitBlockRequest(t, proposerSigningDomain, genesisTime)
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
		Slot:      structs.Slot(request.Message.Slot),
	}
	err = ds.PutPayload(ctx, key, &payload, time.Minute)
	require.NoError(t, err)
	err = ds.PutHeader(ctx, structs.Slot(submitRequest.Message.Slot),
		relay.HeaderAndTrace{
			Header: header,
			Trace: &relay.BidTraceWithTimestamp{
				BidTraceExtended: relay.BidTraceExtended{
					BidTrace: *submitRequest.Message,
				},
				Timestamp: uint64(time.Now().UnixMicro()),
			},
		},
		time.Minute)
	require.NoError(t, err)
	err = ds.PutRegistration(ctx, structs.PubKey{registration.Message.Pubkey}, *registration, time.Minute)
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
	ds := &relay.DefaultDatastore{TTLStorage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)
	r, err := relay.NewRelay(config, ds)
	require.NoError(t, err)

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

	ds := &relay.DefaultDatastore{TTLStorage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)
	r, _ := relay.NewRelay(config, ds)

	relaySigningDomain, err := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		relay.GenesisForkVersionRopsten,
		types.Root{}.String())
	require.NoError(t, err)
	genesisTime := uint64(time.Now().Unix())
	bc.EXPECT().Genesis().AnyTimes().Return(relay.GenesisInfo{GenesisTime: genesisTime})
	submitRequest := validSubmitBlockRequest(t, relaySigningDomain, genesisTime)

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
	gotHeaders, err := ds.GetHeaders(ctx, relay.Query{Slot: structs.Slot(submitRequest.Message.Slot)})
	require.NoError(t, err)
	require.Len(t, gotHeaders, 1)
	require.EqualValues(t, header, gotHeaders[0].Header)
}

func TestSubmitBlockInvalidTimestamp(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)

	sk, _, _ := bls.GenerateNewKeypair()
	config := relay.Config{Log: log.New(), Network: "ropsten", SecretKey: sk, TTL: time.Minute}
	r, _ := relay.NewRelay(config)
	ds := &relay.DefaultDatastore{TTLStorage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	relaySigningDomain, err := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		relay.GenesisForkVersionRopsten,
		types.Root{}.String())
	require.NoError(t, err)
	genesisTime := uint64(time.Now().Unix())
	bc.EXPECT().Genesis().AnyTimes().Return(relay.GenesisInfo{GenesisTime: genesisTime})
	submitRequest := validSubmitBlockRequest(t, relaySigningDomain, genesisTime+1) // +1 in order to make timestamp invalid

	err = r.SubmitBlock(ctx, submitRequest, state{ds: ds, bc: bc})
	require.Error(t, err)
}

func TestSubmitBlocksTwoBuilders(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)

	sk, _, _ := bls.GenerateNewKeypair()
	config := relay.Config{Log: log.New(), Network: "ropsten", SecretKey: sk, TTL: time.Minute}
	r, _ := relay.NewRelay(config)
	ds := &relay.DefaultDatastore{TTLStorage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	genesisTime := uint64(time.Now().Unix())
	bc.EXPECT().Genesis().AnyTimes().Return(relay.GenesisInfo{GenesisTime: genesisTime})

	relaySigningDomain, err := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		relay.GenesisForkVersionRopsten,
		types.Root{}.String())
	require.NoError(t, err)

	// generate and send 1st block
	skB1, pkB1, err := bls.GenerateNewKeypair()
	require.NoError(t, err)

	var pubKeyB1 types.PublicKey
	pubKeyB1.FromSlice(pkB1.Compress())

	slot := relay.Slot(rand.Uint64())
	payloadB1 := randomPayload()
	payloadB1.Timestamp = genesisTime + (uint64(slot) * 12)

	msgB1 := &types.BidTrace{
		Slot:                 uint64(slot),
		ParentHash:           payloadB1.ParentHash,
		BlockHash:            payloadB1.BlockHash,
		BuilderPubkey:        pubKeyB1,
		ProposerPubkey:       types.PublicKey(random48Bytes()),
		ProposerFeeRecipient: types.Address(random20Bytes()),
		Value:                types.IntToU256(10),
	}

	signatureB1, err := types.SignMessage(msgB1, relaySigningDomain, skB1)
	require.NoError(t, err)

	submitRequestOne := &types.BuilderSubmitBlockRequest{
		Signature:        signatureB1,
		Message:          msgB1,
		ExecutionPayload: payloadB1,
	}

	err = r.SubmitBlock(ctx, submitRequestOne, state{ds: ds, bc: bc})
	require.NoError(t, err)

	// generate and send 2nd block
	skB2, pkB2, err := bls.GenerateNewKeypair()
	require.NoError(t, err)

	var pubKeyB2 types.PublicKey
	pubKeyB2.FromSlice(pkB2.Compress())

	payloadB2 := randomPayload()
	payloadB2.Timestamp = genesisTime + (uint64(slot) * 12)

	msgB2 := &types.BidTrace{
		Slot:                 uint64(slot),
		ParentHash:           payloadB2.ParentHash,
		BlockHash:            payloadB2.BlockHash,
		BuilderPubkey:        pubKeyB2,
		ProposerPubkey:       types.PublicKey(random48Bytes()),
		ProposerFeeRecipient: types.Address(random20Bytes()),
		Value:                types.IntToU256(1000),
	}

	signatureB2, err := types.SignMessage(msgB2, relaySigningDomain, skB2)
	require.NoError(t, err)

	submitRequestTwo := &types.BuilderSubmitBlockRequest{
		Signature:        signatureB2,
		Message:          msgB2,
		ExecutionPayload: payloadB2,
	}

	err = r.SubmitBlock(ctx, submitRequestTwo, state{ds: ds, bc: bc})
	require.NoError(t, err)

	// check that payload served from relay is 2nd builders
	signedBuilderBid, err := relay.SubmitBlockRequestToSignedBuilderBid(
		submitRequestOne,
		config.SecretKey,
		&config.PubKey,
		relaySigningDomain)
	require.NoError(t, err)
	payload := relay.SubmitBlockRequestToBlockBidAndTrace(signedBuilderBid, submitRequestOne)

	key := relay.SubmissionToKey(submitRequestOne)
	gotPayload, err := ds.GetPayload(ctx, key)
	require.NoError(t, err)
	require.EqualValues(t, payload, *gotPayload)

	header, err := types.PayloadToPayloadHeader(submitRequestTwo.ExecutionPayload)
	require.NoError(t, err)
	gotHeaders, err := ds.GetMaxProfitHeadersDesc(ctx, slot)

	require.NoError(t, err)
	require.Len(t, gotHeaders, 2)
	require.EqualValues(t, header, gotHeaders[0].Header)
}

func TestSubmitBlocksCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)

	sk, _, _ := bls.GenerateNewKeypair()
	config := relay.Config{Log: log.New(), Network: "ropsten", SecretKey: sk, TTL: time.Minute}
	r, _ := relay.NewRelay(config)
	ds := &relay.DefaultDatastore{TTLStorage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	genesisTime := uint64(time.Now().Unix())
	bc.EXPECT().Genesis().AnyTimes().Return(relay.GenesisInfo{GenesisTime: genesisTime})

	relaySigningDomain, err := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		relay.GenesisForkVersionRopsten,
		types.Root{}.String())
	require.NoError(t, err)

	// generate and send 1st block
	skB1, pkB1, err := bls.GenerateNewKeypair()
	require.NoError(t, err)

	var pubKeyB1 types.PublicKey
	pubKeyB1.FromSlice(pkB1.Compress())

	slot := relay.Slot(rand.Uint64())
	payloadB1 := randomPayload()
	payloadB1.Timestamp = genesisTime + (uint64(slot) * 12)

	msgB1 := &types.BidTrace{
		Slot:                 uint64(slot),
		ParentHash:           payloadB1.ParentHash,
		BlockHash:            payloadB1.BlockHash,
		BuilderPubkey:        pubKeyB1,
		ProposerPubkey:       types.PublicKey(random48Bytes()),
		ProposerFeeRecipient: types.Address(random20Bytes()),
		Value:                types.IntToU256(1000),
	}

	signatureB1, err := types.SignMessage(msgB1, relaySigningDomain, skB1)
	require.NoError(t, err)

	submitRequestOne := &types.BuilderSubmitBlockRequest{
		Signature:        signatureB1,
		Message:          msgB1,
		ExecutionPayload: payloadB1,
	}

	err = r.SubmitBlock(ctx, submitRequestOne, state{ds: ds, bc: bc})
	require.NoError(t, err)

	// generate and send 2nd block from same builder
	payloadB2 := randomPayload()
	payloadB2.Timestamp = genesisTime + (uint64(slot) * 12)

	msgB2 := &types.BidTrace{
		Slot:                 uint64(slot),
		ParentHash:           payloadB2.ParentHash,
		BlockHash:            payloadB2.BlockHash,
		BuilderPubkey:        pubKeyB1,
		ProposerPubkey:       types.PublicKey(random48Bytes()),
		ProposerFeeRecipient: types.Address(random20Bytes()),
		Value:                types.IntToU256(1),
	}

	signatureB2, err := types.SignMessage(msgB2, relaySigningDomain, skB1)
	require.NoError(t, err)

	submitRequestTwo := &types.BuilderSubmitBlockRequest{
		Signature:        signatureB2,
		Message:          msgB2,
		ExecutionPayload: payloadB2,
	}

	err = r.SubmitBlock(ctx, submitRequestTwo, state{ds: ds, bc: bc})
	require.NoError(t, err)

	// check that payload served from relay is 2nd block with lower value
	header, err := types.PayloadToPayloadHeader(submitRequestTwo.ExecutionPayload)
	require.NoError(t, err)
	gotHeaders, err := ds.GetMaxProfitHeadersDesc(ctx, slot)

	require.NoError(t, err)
	require.Len(t, gotHeaders, 1)
	require.EqualValues(t, header, gotHeaders[0].Header)
}

func BenchmarkRegisterValidator(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const N = 10_000

	ctrl := gomock.NewController(b)

	config := relay.Config{Log: log.New(), Network: "ropsten", TTL: 5 * time.Minute}

	ds := &relay.DefaultDatastore{TTLStorage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	r, _ := relay.NewRelay(config, ds)

	registrations := make([]structs.SignedValidatorRegistration, 0, N)
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
		registrations = append(registrations, structs.SignedValidatorRegistration{SignedValidatorRegistration: *registration, Raw: b})

		knownValidators[registration.Message.Pubkey.PubkeyHex()] = struct{}{}
	}

	bc.EXPECT().IsKnownValidator(gomock.Any()).Return(true, nil).Times(b.N * N)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := r.RegisterValidator(ctx, registrations, state{ds: ds, bc: bc})
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
	ds := &relay.DefaultDatastore{TTLStorage: &relay.TTLDatastoreBatcher{TTLDatastore: store}}

	const N = 10_000

	config := relay.Config{Log: log.New(), Network: "ropsten"}
	r, _ := relay.NewRelay(config, ds)

	bc := &FakeBeaconMock{}

	registrations := make([]structs.SignedValidatorRegistration, 0, N)
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
		registrations = append(registrations, structs.SignedValidatorRegistration{SignedValidatorRegistration: *registration, Raw: b})
		knownValidators[registration.Message.Pubkey.PubkeyHex()] = struct{}{}
	}

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
			err := r.RegisterValidator(ctx, registrations, state{ds: ds, bc: bc})
			if err != nil {
				panic(err)
			}
			wg.Done()
		}()
	}

	wg.Wait()
	for i := 0; i < b.N; i++ {
		go func() {
			err := r.RegisterValidator(ctx, registrations, state{ds: ds, bc: bc})
			if err != nil {
				panic(err)
			}
			wg2.Done()
		}()
	}

	wg2.Wait()
	for i := 0; i < b.N; i++ {
		go func() {
			err := r.RegisterValidator(ctx, registrations, state{ds: ds, bc: bc})
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
	config := relay.Config{Log: log.New(),
		Network:   "ropsten",
		SecretKey: pk, // pragma: allowlist secret
		PubKey:    types.PublicKey(random48Bytes())}

	ds := &relay.DefaultDatastore{TTLStorage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)
	r, _ := relay.NewRelay(config, ds)

	proposerSigningDomain, _ := relay.ComputeDomain(
		types.DomainTypeBeaconProposer,
		relay.BellatrixForkVersionRopsten,
		relay.GenesisValidatorsRootRopsten)
	genesisTime := uint64(time.Now().Unix())
	bc.EXPECT().Genesis().AnyTimes().Return(relay.GenesisInfo{GenesisTime: genesisTime})
	submitRequest := validSubmitBlockRequest(b, proposerSigningDomain, genesisTime)
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
		relay.HeaderAndTrace{
			Header: header,
			Trace: &relay.BidTraceWithTimestamp{
				BidTraceExtended: relay.BidTraceExtended{
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
		SecretKey: pk, // pragma: allowlist secret
		PubKey:    types.PublicKey(random48Bytes())}

	ds := &relay.DefaultDatastore{TTLStorage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	r, _ := relay.NewRelay(config, ds)

	proposerSigningDomain, _ := relay.ComputeDomain(
		types.DomainTypeBeaconProposer,
		relay.BellatrixForkVersionRopsten,
		relay.GenesisValidatorsRootRopsten)
	genesisTime := uint64(time.Now().Unix())
	bc.EXPECT().Genesis().AnyTimes().Return(relay.GenesisInfo{GenesisTime: genesisTime})
	submitRequest := validSubmitBlockRequest(b, proposerSigningDomain, genesisTime)
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
		relay.HeaderAndTrace{
			Header: header,
			Trace: &relay.BidTraceWithTimestamp{
				BidTraceExtended: relay.BidTraceExtended{
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
		SecretKey: pk, // pragma: allowlist secret
		PubKey:    types.PublicKey(random48Bytes())}

	ds := &relay.DefaultDatastore{TTLStorage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)
	r, _ := relay.NewRelay(config, ds)

	proposerSigningDomain, _ := relay.ComputeDomain(
		types.DomainTypeBeaconProposer,
		relay.BellatrixForkVersionRopsten,
		relay.GenesisValidatorsRootRopsten)
	genesisTime := uint64(time.Now().Unix())
	bc.EXPECT().Genesis().AnyTimes().Return(relay.GenesisInfo{GenesisTime: genesisTime})
	submitRequest := validSubmitBlockRequest(b, proposerSigningDomain, genesisTime)
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
	key := relay.PayloadKey{
		BlockHash: request.Message.Body.ExecutionPayloadHeader.BlockHash,
		Proposer:  registration.Message.Pubkey,
		Slot:      structs.Slot(request.Message.Slot),
	}
	_ = ds.PutPayload(ctx, key, &payload, time.Minute)
	_ = ds.PutHeader(ctx, structs.Slot(submitRequest.Message.Slot),
		relay.HeaderAndTrace{
			Header: header,
			Trace: &relay.BidTraceWithTimestamp{
				BidTraceExtended: relay.BidTraceExtended{
					BidTrace: *submitRequest.Message,
				},
				Timestamp: uint64(time.Now().UnixMicro()),
			},
		},
		time.Minute)
	_ = ds.PutRegistration(ctx, structs.PubKey{registration.Message.Pubkey}, *registration, time.Minute)

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
		SecretKey: pk, // pragma: allowlist secret
		PubKey:    types.PublicKey(random48Bytes())}

	ds := &relay.DefaultDatastore{TTLStorage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	r, _ := relay.NewRelay(config, ds)

	proposerSigningDomain, _ := relay.ComputeDomain(
		types.DomainTypeBeaconProposer,
		relay.BellatrixForkVersionRopsten,
		relay.GenesisValidatorsRootRopsten)
	genesisTime := uint64(time.Now().Unix())
	bc.EXPECT().Genesis().AnyTimes().Return(relay.GenesisInfo{GenesisTime: genesisTime})
	submitRequest := validSubmitBlockRequest(b, proposerSigningDomain, genesisTime)
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
	key := relay.PayloadKey{
		BlockHash: request.Message.Body.ExecutionPayloadHeader.BlockHash,
		Proposer:  registration.Message.Pubkey,
		Slot:      structs.Slot(request.Message.Slot),
	}
	_ = ds.PutPayload(ctx, key, &payload, time.Minute)
	_ = ds.PutHeader(ctx, structs.Slot(submitRequest.Message.Slot),
		relay.HeaderAndTrace{
			Header: header,
			Trace: &relay.BidTraceWithTimestamp{
				BidTraceExtended: relay.BidTraceExtended{
					BidTrace: *submitRequest.Message,
				},
				Timestamp: uint64(time.Now().UnixMicro()),
			},
		},
		time.Minute)
	_ = ds.PutRegistration(ctx, structs.PubKey{registration.Message.Pubkey}, *registration, time.Minute)

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
	config := relay.Config{Log: log.New(), Network: "ropsten", SecretKey: pk} // pragma: allowlist secret

	ds := &relay.DefaultDatastore{TTLStorage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	r, _ := relay.NewRelay(config, ds)

	relaySigningDomain, _ := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		relay.GenesisForkVersionRopsten,
		types.Root{}.String())
	genesisTime := uint64(time.Now().Unix())
	bc.EXPECT().Genesis().AnyTimes().Return(relay.GenesisInfo{GenesisTime: genesisTime})
	submitRequest := validSubmitBlockRequest(b, relaySigningDomain, genesisTime)

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
	config := relay.Config{Log: log.New(), Network: "ropsten", SecretKey: pk} // pragma: allowlist secret

	ds := &relay.DefaultDatastore{TTLStorage: newMockDatastore()}
	bc := mock_relay.NewMockBeaconState(ctrl)

	r, _ := relay.NewRelay(config, ds)

	relaySigningDomain, _ := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		relay.GenesisForkVersionRopsten,
		types.Root{}.String())
	genesisTime := uint64(time.Now().Unix())
	bc.EXPECT().Genesis().AnyTimes().Return(relay.GenesisInfo{GenesisTime: genesisTime})
	submitRequest := validSubmitBlockRequest(b, relaySigningDomain, genesisTime)

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
