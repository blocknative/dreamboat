package relay_test

import (
	"context"
	"encoding/json"
	"io"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/blocknative/dreamboat/auction"
	"github.com/blocknative/dreamboat/blstools"
	"github.com/blocknative/dreamboat/datastore"
	"github.com/blocknative/dreamboat/structs/forks/bellatrix"
	"github.com/blocknative/dreamboat/test/common"
	"github.com/blocknative/dreamboat/verify"

	"github.com/blocknative/dreamboat/relay/mocks"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/google/uuid"

	relay "github.com/blocknative/dreamboat/relay"
	"github.com/blocknative/dreamboat/structs"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/golang/mock/gomock"
	"github.com/lthibault/log"
	"github.com/stretchr/testify/require"

	badger "github.com/ipfs/go-ds-badger2"
)

var (
	l = log.New(log.WithWriter(io.Discard))
)

const (
	GenesisValidatorsRootRopsten = "0x44f1e56283ca88b35c789f7f449e52339bc1fefe3a45913a43a6d16edcd33cf1"
)

func TestGetHeader(t *testing.T) {
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

	relaySigningDomain, err := common.ComputeDomain(
		types.DomainTypeAppBuilder,
		types.Root{}.String())

	config := relay.RelayConfig{
		TTL:                  time.Minute,
		BuilderSigningDomain: relaySigningDomain,
		SecretKey:            sk, // pragma: allowlist secret
		PubKey:               types.PublicKey(random48Bytes()),
	}

	a := auction.NewAuctioneer()
	ver := verify.NewVerificationManager(l, 20000)
	ver.RunVerify(300)
	r := relay.NewRelay(log.New(), config, nil, nil, nil, ver, bs, ds, a, nil)
	require.NoError(t, err)

	genesisTime := uint64(time.Now().Unix())
	bs.EXPECT().Genesis().AnyTimes().Return(structs.GenesisInfo{GenesisTime: genesisTime})
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

	payload := relay.SubmitBlockRequestToBlockBidAndTrace("bellatrix", signedBuilderBid, submitRequest)

	// fill the datastore
	key := relay.SubmissionToKey(submitRequest)
	err = ds.PutPayload(ctx, key, &payload, time.Minute)
	require.NoError(t, err)
	header, err := types.PayloadToPayloadHeader(submitRequest.ExecutionPayload)
	require.NoError(t, err)
	pHeader := structs.HeaderAndTrace{
		Header: header,
		Trace: &structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: *submitRequest.Message,
			},
			Timestamp: uint64(time.Now().UnixMicro()),
		},
	}

	jsHeader, _ := json.Marshal(pHeader)
	err = ds.PutHeader(ctx, structs.HeaderData{
		Slot:           structs.Slot(submitRequest.Message.Slot),
		HeaderAndTrace: pHeader,
		Marshaled:      jsHeader,
	}, time.Minute)
	require.NoError(t, err)

	a.AddBlock(&structs.CompleteBlockstruct{Header: pHeader, Payload: payload})
	response, err := r.GetHeader(ctx, structs.NewMetricGroup(4), request)
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

	var datadir = "/tmp/" + t.Name() + uuid.New().String()
	store, _ := badger.NewDatastore(datadir, &badger.DefaultOptions)
	hc := datastore.NewHeaderController(100, time.Hour)

	ds, err := datastore.NewDatastore(&datastore.TTLDatastoreBatcher{TTLDatastore: store}, store.DB, hc, 100)
	require.NoError(t, err)

	bs := mocks.NewMockState(ctrl)

	proposerSigningDomain, err := common.ComputeDomain(
		types.DomainTypeBeaconProposer,
		GenesisValidatorsRootRopsten)
	require.NoError(t, err)

	config := relay.RelayConfig{
		SecretKey:             pk, //pragma: allowlist secret
		PubKey:                types.PublicKey(random48Bytes()),
		TTL:                   time.Minute,
		ProposerSigningDomain: map[string]types.Domain{"bellatrix": proposerSigningDomain},
		BuilderSigningDomain:  types.DomainBuilder,
	}

	l := log.New()

	ver := verify.NewVerificationManager(l, 20)
	ver.RunVerify(300)

	r := relay.NewRelay(l, config, nil, nil, nil, ver, bs, ds, auction.NewAuctioneer(), nil)

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
			SyncAggregate:          &types.SyncAggregate{CommitteeBits: types.CommitteeBits{0x07}, CommitteeSignature: types.Signature{0x08}},
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

	payload := relay.SubmitBlockRequestToBlockBidAndTrace("bellatrix", signedBuilderBid, submitRequest)

	// fill the datastore
	key := structs.PayloadKey{
		BlockHash: request.Message.Body.ExecutionPayloadHeader.BlockHash,
		Proposer:  registration.Message.Pubkey,
		Slot:      structs.Slot(request.Message.Slot),
	}
	err = ds.PutPayload(ctx, key, &payload, time.Minute)
	require.NoError(t, err)
	pHeader := structs.HeaderAndTrace{
		Header: header,
		Trace: &structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: *submitRequest.Message,
			},
			Timestamp: uint64(time.Now().UnixMicro()),
		},
	}

	jsHeader, _ := json.Marshal(pHeader)
	err = ds.PutHeader(ctx, structs.HeaderData{
		Slot:           structs.Slot(submitRequest.Message.Slot),
		HeaderAndTrace: pHeader,
		Marshaled:      jsHeader,
	}, time.Minute)

	require.NoError(t, err)

	validatorState := structs.ValidatorsState{
		KnownValidatorsByIndex: map[uint64]types.PubkeyHex{
			request.Message.ProposerIndex: registration.Message.Pubkey.PubkeyHex(),
		},
	}

	bs.EXPECT().KnownValidators().Return(validatorState).Times(1)

	response, err := r.GetPayload(ctx, structs.NewMetricGroup(4), request)
	require.NoError(t, err)

	require.EqualValues(t, submitRequest.ExecutionPayload, response.Data)
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

func validSubmitBlockRequest(t require.TestingT, domain types.Domain, genesisTime uint64) *structs.SubmitBlockRequest {
	sk, pubKey, err := blstools.GenerateNewKeypair()
	require.NoError(t, err)

	slot := rand.Uint64()

	payload := randomPayload()
	payload.Timestamp = genesisTime + (slot * 12)

	msg := &types.BidTrace{
		Slot:                 slot,
		ParentHash:           payload.ParentHash,
		BlockHash:            payload.BlockHash,
		BuilderPubkey:        pubKey,
		ProposerPubkey:       types.PublicKey(random48Bytes()),
		ProposerFeeRecipient: types.Address(random20Bytes()),
		Value:                types.IntToU256(rand.Uint64()),
	}

	signature, err := types.SignMessage(msg, domain, sk)
	require.NoError(t, err)

	return &bellatrix.SubmitBlockRequest{
		BellatrixSignature: signature,
		BellatrixMessage:   msg,
		ExecutionPayload:   payload,
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
