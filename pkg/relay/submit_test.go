package relay_test

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/blocknative/dreamboat/blstools"
	"github.com/blocknative/dreamboat/pkg/auction"
	"github.com/blocknative/dreamboat/pkg/datastore"
	"github.com/blocknative/dreamboat/pkg/verify"
	"github.com/blocknative/dreamboat/test/common"

	"github.com/blocknative/dreamboat/pkg/relay/mocks"
	"github.com/google/uuid"

	relay "github.com/blocknative/dreamboat/pkg/relay"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/golang/mock/gomock"
	"github.com/lthibault/log"
	"github.com/stretchr/testify/require"

	badger "github.com/ipfs/go-ds-badger2"
)

func TestSubmitBlock(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)

	sk, _, _ := bls.GenerateNewKeypair()

	var datadir = "/tmp/" + t.Name() + uuid.New().String()
	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)

	hc := datastore.NewHeaderController(100, time.Hour)
	ds, err := datastore.NewDatastore(&datastore.TTLDatastoreBatcher{TTLDatastore: store}, store.DB, hc, 100)
	require.NoError(t, err)
	bs := mocks.NewMockState(ctrl)

	ver := verify.NewVerificationManager(l, 20000)
	ver.RunVerify(300)

	relaySigningDomain, err := common.ComputeDomain(
		types.DomainTypeAppBuilder,
		types.Root{}.String())
	require.NoError(t, err)

	config := relay.RelayConfig{
		TTL:                  time.Minute,
		SecretKey:            sk,
		RegistrationCacheTTL: time.Minute,
		BuilderSigningDomain: relaySigningDomain,
	}

	genesisTime := uint64(time.Now().Unix())
	submitRequest := validSubmitBlockRequest(t, relaySigningDomain, genesisTime)
	signedBuilderBid, err := relay.SubmitBlockRequestToSignedBuilderBid(
		submitRequest,
		config.SecretKey,
		&config.PubKey,
		relaySigningDomain)
	require.NoError(t, err)
	payload := relay.SubmitBlockRequestToBlockBidAndTrace("bellatrix", signedBuilderBid, submitRequest)

	bs.EXPECT().Genesis().AnyTimes().Return(structs.GenesisInfo{GenesisTime: genesisTime})

	bVCli := mocks.NewMockBlockValidationClient(ctrl)
	bVCli.EXPECT().ValidateBlock(gomock.Any(), gomock.Any()).Times(1)
	vCache := mocks.NewMockValidatorCache(ctrl)
	vStore := mocks.NewMockValidatorStore(ctrl)
	vCache.EXPECT().Get(submitRequest.Message.ProposerPubkey).Return(structs.ValidatorCacheEntry{
		Time: time.Now(),
		Entry: types.SignedValidatorRegistration{
			Message: &types.RegisterValidatorRequestMessage{
				FeeRecipient: submitRequest.Message.ProposerFeeRecipient,
			}},
	}, true)

	r := relay.NewRelay(l, config, nil, vCache, vStore, ver, bs, ds, auction.NewAuctioneer(), bVCli)

	bs.EXPECT().HeadSlot().AnyTimes().Return(structs.Slot(submitRequest.Message.Slot))

	err = r.SubmitBlock(ctx, structs.NewMetricGroup(4), submitRequest)
	require.NoError(t, err)

	key := relay.SubmissionToKey(submitRequest)
	gotPayload, _, err := ds.GetPayload(ctx, key)
	require.NoError(t, err)
	require.EqualValues(t, payload, *gotPayload)

	header, err := types.PayloadToPayloadHeader(submitRequest.ExecutionPayload)
	require.NoError(t, err)
	gotHeaders, err := ds.GetHeadersBySlot(ctx, submitRequest.Message.Slot)
	require.NoError(t, err)
	require.Len(t, gotHeaders, 1)
	require.EqualValues(t, header, gotHeaders[0].Header)
}

func BenchmarkSubmitBlock(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(b)

	pk, _, _ := bls.GenerateNewKeypair()

	bs := mocks.NewMockState(ctrl)

	var datadir = "/tmp/" + b.Name() + uuid.New().String()
	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)

	hc := datastore.NewHeaderController(100, time.Hour)
	ds, err := datastore.NewDatastore(&datastore.TTLDatastoreBatcher{TTLDatastore: store}, store.DB, hc, 100)
	require.NoError(b, err)

	ver := verify.NewVerificationManager(l, 20000)
	ver.RunVerify(300)

	relaySigningDomain, _ := common.ComputeDomain(
		types.DomainTypeAppBuilder,
		types.Root{}.String())

	config := relay.RelayConfig{
		TTL:                  5 * time.Minute,
		RegistrationCacheTTL: time.Minute,
		SecretKey:            pk, // pragma: allowlist secret
		PubKey:               types.PublicKey(random48Bytes()),
		BuilderSigningDomain: relaySigningDomain,
	}

	genesisTime := uint64(time.Now().Unix())
	bs.EXPECT().Genesis().AnyTimes().Return(structs.GenesisInfo{GenesisTime: genesisTime})
	submitRequest := validSubmitBlockRequest(b, relaySigningDomain, genesisTime)

	bVCli := mocks.NewMockBlockValidationClient(ctrl)
	bVCli.EXPECT().ValidateBlock(gomock.Any(), gomock.Any()).Times(1)
	vCache := mocks.NewMockValidatorCache(ctrl)
	vStore := mocks.NewMockValidatorStore(ctrl)
	vCache.EXPECT().Get(submitRequest.Message.ProposerPubkey).Return(structs.ValidatorCacheEntry{
		Time: time.Now(),
		Entry: types.SignedValidatorRegistration{
			Message: &types.RegisterValidatorRequestMessage{
				FeeRecipient: submitRequest.Message.ProposerFeeRecipient,
			},
		},
	}, true).AnyTimes()

	r := relay.NewRelay(l, config, nil, vCache, vStore, ver, bs, ds, auction.NewAuctioneer(), bVCli)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := r.SubmitBlock(ctx, structs.NewMetricGroup(4), submitRequest)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkSubmitBlockParallel(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(b)

	var datadir = "/tmp/" + b.Name() + uuid.New().String()
	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)

	hc := datastore.NewHeaderController(100, time.Hour)
	ds, err := datastore.NewDatastore(&datastore.TTLDatastoreBatcher{TTLDatastore: store}, store.DB, hc, 100)
	require.NoError(b, err)
	bs := mocks.NewMockState(ctrl)

	l := log.New()

	ver := verify.NewVerificationManager(l, 20000)
	ver.RunVerify(300)

	relaySigningDomain, _ := common.ComputeDomain(
		types.DomainTypeAppBuilder,
		types.Root{}.String())

	pk, _, _ := bls.GenerateNewKeypair()
	config := relay.RelayConfig{
		TTL:                  5 * time.Minute,
		RegistrationCacheTTL: time.Minute,
		SecretKey:            pk, // pragma: allowlist secret
		PubKey:               types.PublicKey(random48Bytes()),
		BuilderSigningDomain: relaySigningDomain,
	}

	genesisTime := uint64(time.Now().Unix())
	bs.EXPECT().Genesis().AnyTimes().Return(structs.GenesisInfo{GenesisTime: genesisTime})
	submitRequest := validSubmitBlockRequest(b, relaySigningDomain, genesisTime)

	bVCli := mocks.NewMockBlockValidationClient(ctrl)
	bVCli.EXPECT().ValidateBlock(gomock.Any(), gomock.Any()).Times(1)
	vCache := mocks.NewMockValidatorCache(ctrl)
	vStore := mocks.NewMockValidatorStore(ctrl)
	vCache.EXPECT().Get(submitRequest.Message.ProposerPubkey).Return(structs.ValidatorCacheEntry{
		Time: time.Now(),
		Entry: types.SignedValidatorRegistration{
			Message: &types.RegisterValidatorRequestMessage{
				FeeRecipient: submitRequest.Message.ProposerFeeRecipient,
			},
		},
	}, true).AnyTimes()

	r := relay.NewRelay(l, config, nil, vCache, vStore, ver, bs, ds, auction.NewAuctioneer(), bVCli)

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(b.N)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		go func() {
			err := r.SubmitBlock(ctx, structs.NewMetricGroup(4), submitRequest)
			if err != nil {
				panic(err)
			}
			wg.Done()
		}()
	}
}

func TestSubmitBlockInvalidTimestamp(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)

	ds := &datastore.Datastore{TTLStorage: newMockDatastore()}
	bs := mocks.NewMockState(ctrl)
	sk, _, _ := bls.GenerateNewKeypair()

	l := log.New()

	bVCli := mocks.NewMockBlockValidationClient(ctrl)
	//bVCli.EXPECT().ValidateBlock(gomock.Any(), gomock.Any()).Times(1)
	ver := verify.NewVerificationManager(l, 20000)
	ver.RunVerify(300)

	relaySigningDomain, err := common.ComputeDomain(
		types.DomainTypeAppBuilder,
		types.Root{}.String())
	require.NoError(t, err)

	config := relay.RelayConfig{
		TTL:                  5 * time.Minute,
		RegistrationCacheTTL: time.Minute,
		SecretKey:            sk, // pragma: allowlist secret
		PubKey:               types.PublicKey(random48Bytes()),
		BuilderSigningDomain: relaySigningDomain,
	}

	r := relay.NewRelay(l, config, nil, nil, nil, ver, bs, ds, auction.NewAuctioneer(), bVCli)

	genesisTime := uint64(time.Now().Unix())
	bs.EXPECT().Genesis().AnyTimes().Return(structs.GenesisInfo{GenesisTime: genesisTime})
	submitRequest := validSubmitBlockRequest(t, relaySigningDomain, genesisTime+1) // +1 in order to make timestamp invalid

	err = r.SubmitBlock(ctx, structs.NewMetricGroup(4), submitRequest)
	require.Error(t, err)
}

func TestSubmitBlocksTwoBuilders(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)

	sk, _, _ := bls.GenerateNewKeypair()

	var datadir = "/tmp/" + uuid.New().String()
	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	hc := datastore.NewHeaderController(100, time.Hour)
	ds, err := datastore.NewDatastore(&datastore.TTLDatastoreBatcher{TTLDatastore: store}, store.DB, hc, 100)
	require.NoError(t, err)
	bs := mocks.NewMockState(ctrl)

	genesisTime := uint64(time.Now().Unix())
	bs.EXPECT().Genesis().AnyTimes().Return(structs.GenesisInfo{GenesisTime: genesisTime})

	ver := verify.NewVerificationManager(l, 20000)
	ver.RunVerify(300)

	relaySigningDomain, _ := common.ComputeDomain(
		types.DomainTypeAppBuilder,
		types.Root{}.String())

	config := relay.RelayConfig{
		TTL:                  5 * time.Minute,
		RegistrationCacheTTL: time.Minute,
		SecretKey:            sk, // pragma: allowlist secret
		PubKey:               types.PublicKey(random48Bytes()),
		BuilderSigningDomain: relaySigningDomain,
	}

	proposerPubkey := types.PublicKey(random48Bytes())
	proposerFeeRecipient := types.Address(random20Bytes())

	bVCli := mocks.NewMockBlockValidationClient(ctrl)
	bVCli.EXPECT().ValidateBlock(gomock.Any(), gomock.Any()).Times(2)
	vCache := mocks.NewMockValidatorCache(ctrl)
	vStore := mocks.NewMockValidatorStore(ctrl)
	vCache.EXPECT().Get(proposerPubkey).Return(structs.ValidatorCacheEntry{
		Time: time.Now(),
		Entry: types.SignedValidatorRegistration{
			Message: &types.RegisterValidatorRequestMessage{
				FeeRecipient: proposerFeeRecipient,
			},
		},
	}, true).AnyTimes()

	r := relay.NewRelay(l, config, nil, vCache, vStore, ver, bs, ds, auction.NewAuctioneer(), bVCli)

	// generate and send 1st block
	skB1, pubKeyB1, err := blstools.GenerateNewKeypair()
	require.NoError(t, err)

	slot := structs.Slot(rand.Uint64())
	payloadB1 := randomPayload()
	payloadB1.Timestamp = genesisTime + (uint64(slot) * 12)

	msgB1 := &types.BidTrace{
		Slot:                 uint64(slot),
		ParentHash:           payloadB1.ParentHash,
		BlockHash:            payloadB1.BlockHash,
		BuilderPubkey:        pubKeyB1,
		ProposerPubkey:       proposerPubkey,
		ProposerFeeRecipient: proposerFeeRecipient,
		Value:                types.IntToU256(10),
	}

	bs.EXPECT().HeadSlot().AnyTimes().Return(structs.Slot(msgB1.Slot))

	signatureB1, err := types.SignMessage(msgB1, relaySigningDomain, skB1)
	require.NoError(t, err)

	submitRequestOne := &types.BuilderSubmitBlockRequest{
		Signature:        signatureB1,
		Message:          msgB1,
		ExecutionPayload: payloadB1,
	}

	err = r.SubmitBlock(ctx, structs.NewMetricGroup(4), submitRequestOne)
	require.NoError(t, err)

	skB2, pubKeyB2, err := blstools.GenerateNewKeypair()
	require.NoError(t, err)

	payloadB2 := randomPayload()
	payloadB2.Timestamp = genesisTime + (uint64(slot) * 12)

	msgB2 := &types.BidTrace{
		Slot:                 uint64(slot),
		ParentHash:           payloadB2.ParentHash,
		BlockHash:            payloadB2.BlockHash,
		BuilderPubkey:        pubKeyB2,
		ProposerPubkey:       proposerPubkey,
		ProposerFeeRecipient: proposerFeeRecipient,
		Value:                types.IntToU256(1000),
	}

	signatureB2, err := types.SignMessage(msgB2, relaySigningDomain, skB2)
	require.NoError(t, err)

	submitRequestTwo := &types.BuilderSubmitBlockRequest{
		Signature:        signatureB2,
		Message:          msgB2,
		ExecutionPayload: payloadB2,
	}

	err = r.SubmitBlock(ctx, structs.NewMetricGroup(4), submitRequestTwo)
	require.NoError(t, err)

	// check that payload served from relay is 2nd builders
	signedBuilderBid, err := relay.SubmitBlockRequestToSignedBuilderBid(
		submitRequestOne,
		config.SecretKey,
		&config.PubKey,
		relaySigningDomain)
	require.NoError(t, err)
	payload := relay.SubmitBlockRequestToBlockBidAndTrace("bellatrix", signedBuilderBid, submitRequestOne)

	key := relay.SubmissionToKey(submitRequestOne)
	gotPayload, _, err := ds.GetPayload(ctx, key)
	require.NoError(t, err)
	require.EqualValues(t, payload, *gotPayload)

	header, err := types.PayloadToPayloadHeader(submitRequestTwo.ExecutionPayload)
	require.NoError(t, err)
	gotHeaders, err := ds.GetMaxProfitHeader(ctx, uint64(slot))

	require.NoError(t, err)
	require.EqualValues(t, header, gotHeaders.Header)
}

func TestSubmitBlocksCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)

	sk, _, _ := bls.GenerateNewKeypair()

	var datadir = "/tmp/" + uuid.New().String()
	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	hc := datastore.NewHeaderController(100, time.Hour)
	ds, err := datastore.NewDatastore(&datastore.TTLDatastoreBatcher{TTLDatastore: store}, store.DB, hc, 100)
	require.NoError(t, err)

	bs := mocks.NewMockState(ctrl)

	genesisTime := uint64(time.Now().Unix())
	bs.EXPECT().Genesis().AnyTimes().Return(structs.GenesisInfo{GenesisTime: genesisTime})

	l := log.New()
	ver := verify.NewVerificationManager(l, 20000)
	ver.RunVerify(300)

	relaySigningDomain, _ := common.ComputeDomain(
		types.DomainTypeAppBuilder,
		types.Root{}.String())

	config := relay.RelayConfig{
		TTL:                  5 * time.Minute,
		RegistrationCacheTTL: time.Minute,
		SecretKey:            sk, // pragma: allowlist secret
		PubKey:               types.PublicKey(random48Bytes()),
		BuilderSigningDomain: relaySigningDomain,
	}

	proposerPubkey := types.PublicKey(random48Bytes())
	proposerFeeRecipient := types.Address(random20Bytes())

	bVCli := mocks.NewMockBlockValidationClient(ctrl)
	bVCli.EXPECT().ValidateBlock(gomock.Any(), gomock.Any()).Times(2)
	vCache := mocks.NewMockValidatorCache(ctrl)
	vStore := mocks.NewMockValidatorStore(ctrl)
	vCache.EXPECT().Get(proposerPubkey).Return(structs.ValidatorCacheEntry{
		Time: time.Now(),
		Entry: types.SignedValidatorRegistration{
			Message: &types.RegisterValidatorRequestMessage{
				FeeRecipient: proposerFeeRecipient,
			},
		},
	}, true).AnyTimes()

	r := relay.NewRelay(l, config, nil, vCache, vStore, ver, bs, ds, auction.NewAuctioneer(), bVCli)

	skB1, pubKeyB1, err := blstools.GenerateNewKeypair()
	require.NoError(t, err)

	slot := structs.Slot(rand.Uint64())
	payloadB1 := randomPayload()
	payloadB1.Timestamp = genesisTime + (uint64(slot) * 12)

	msgB1 := &types.BidTrace{
		Slot:                 uint64(slot),
		ParentHash:           payloadB1.ParentHash,
		BlockHash:            payloadB1.BlockHash,
		BuilderPubkey:        pubKeyB1,
		ProposerPubkey:       proposerPubkey,
		ProposerFeeRecipient: proposerFeeRecipient,
		Value:                types.IntToU256(1000),
	}

	bs.EXPECT().HeadSlot().AnyTimes().Return(slot)

	signatureB1, err := types.SignMessage(msgB1, relaySigningDomain, skB1)
	require.NoError(t, err)

	submitRequestOne := &types.BuilderSubmitBlockRequest{
		Signature:        signatureB1,
		Message:          msgB1,
		ExecutionPayload: payloadB1,
	}

	err = r.SubmitBlock(ctx, structs.NewMetricGroup(4), submitRequestOne)
	require.NoError(t, err)

	// generate and send 2nd block from same builder
	payloadB2 := randomPayload()
	payloadB2.Timestamp = genesisTime + (uint64(slot) * 12)

	msgB2 := &types.BidTrace{
		Slot:                 uint64(slot),
		ParentHash:           payloadB2.ParentHash,
		BlockHash:            payloadB2.BlockHash,
		BuilderPubkey:        pubKeyB1,
		ProposerPubkey:       proposerPubkey,
		ProposerFeeRecipient: proposerFeeRecipient,
		Value:                types.IntToU256(1),
	}

	signatureB2, err := types.SignMessage(msgB2, relaySigningDomain, skB1)
	require.NoError(t, err)

	submitRequestTwo := &types.BuilderSubmitBlockRequest{
		Signature:        signatureB2,
		Message:          msgB2,
		ExecutionPayload: payloadB2,
	}

	err = r.SubmitBlock(ctx, structs.NewMetricGroup(4), submitRequestTwo)
	require.NoError(t, err)

	// check that payload served from relay is 2nd block with lower value
	header, err := types.PayloadToPayloadHeader(submitRequestTwo.ExecutionPayload)
	require.NoError(t, err)

	gotHeaders, err := ds.GetMaxProfitHeader(ctx, uint64(slot))
	require.NoError(t, err)

	require.EqualValues(t, header, gotHeaders.Header)
}

type vFeeProposer struct{ t structs.ValidatorCacheEntry }

func VFeeProposer(t structs.ValidatorCacheEntry) gomock.Matcher {
	return &vFeeProposer{t}
}
func (o *vFeeProposer) Matches(x interface{}) bool {
	vce, ok := x.(structs.ValidatorCacheEntry)
	if !ok {
		return false
	}

	return vce.Entry.Message.FeeRecipient == o.t.Entry.Message.FeeRecipient
}

func (o *vFeeProposer) String() string {
	return "is " + o.t.Entry.Message.FeeRecipient.String()
}

func TestRegistartionCache(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)

	sk, _, _ := bls.GenerateNewKeypair()

	var datadir = "/tmp/" + uuid.New().String()
	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	hc := datastore.NewHeaderController(100, time.Hour)
	ds, err := datastore.NewDatastore(&datastore.TTLDatastoreBatcher{TTLDatastore: store}, store.DB, hc, 100)
	require.NoError(t, err)

	bs := mocks.NewMockState(ctrl)

	genesisTime := uint64(time.Now().Unix())
	bs.EXPECT().Genesis().AnyTimes().Return(structs.GenesisInfo{GenesisTime: genesisTime})

	l := log.New()
	ver := verify.NewVerificationManager(l, 20000)
	ver.RunVerify(300)

	relaySigningDomain, _ := common.ComputeDomain(
		types.DomainTypeAppBuilder,
		types.Root{}.String())

	config := relay.RelayConfig{
		TTL:                  5 * time.Minute,
		RegistrationCacheTTL: time.Minute,
		SecretKey:            sk, // pragma: allowlist secret
		PubKey:               types.PublicKey(random48Bytes()),
		BuilderSigningDomain: relaySigningDomain,
	}

	proposerPubkey := types.PublicKey(random48Bytes())
	proposerFeeRecipient := types.Address(random20Bytes())
	vCE := structs.ValidatorCacheEntry{
		Entry: types.SignedValidatorRegistration{
			Message: &types.RegisterValidatorRequestMessage{
				FeeRecipient: proposerFeeRecipient,
				Pubkey:       proposerPubkey,
			},
		},
	}
	bVCli := mocks.NewMockBlockValidationClient(ctrl)
	bVCli.EXPECT().ValidateBlock(gomock.Any(), gomock.Any()).Times(1)

	vCache := mocks.NewMockValidatorCache(ctrl)
	vCache.EXPECT().Get(proposerPubkey).Return(structs.ValidatorCacheEntry{}, false)
	vCache.EXPECT().Add(proposerPubkey, VFeeProposer(vCE)).Return(false)

	vStore := mocks.NewMockValidatorStore(ctrl)
	vStore.EXPECT().GetRegistration(gomock.Any(), proposerPubkey).Return(types.SignedValidatorRegistration{
		Message: &types.RegisterValidatorRequestMessage{
			FeeRecipient: proposerFeeRecipient,
			Pubkey:       proposerPubkey,
		},
	}, nil)

	r := relay.NewRelay(l, config, nil, vCache, vStore, ver, bs, ds, auction.NewAuctioneer(), bVCli)

	skB1, pubKeyB1, err := blstools.GenerateNewKeypair()
	require.NoError(t, err)

	slot := structs.Slot(rand.Uint64())
	payloadB1 := randomPayload()
	payloadB1.Timestamp = genesisTime + (uint64(slot) * 12)

	msgB1 := &types.BidTrace{
		Slot:                 uint64(slot),
		ParentHash:           payloadB1.ParentHash,
		BlockHash:            payloadB1.BlockHash,
		BuilderPubkey:        pubKeyB1,
		ProposerPubkey:       proposerPubkey,
		ProposerFeeRecipient: proposerFeeRecipient,
		Value:                types.IntToU256(1000),
	}

	bs.EXPECT().HeadSlot().AnyTimes().Return(structs.Slot(slot))

	signatureB1, err := types.SignMessage(msgB1, relaySigningDomain, skB1)
	require.NoError(t, err)

	submitRequestOne := &types.BuilderSubmitBlockRequest{
		Signature:        signatureB1,
		Message:          msgB1,
		ExecutionPayload: payloadB1,
	}

	err = r.SubmitBlock(ctx, structs.NewMetricGroup(4), submitRequestOne)
	require.NoError(t, err)

	_, err = ds.GetMaxProfitHeader(ctx, uint64(slot))
	require.NoError(t, err)
}
