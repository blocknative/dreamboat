package datastore_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

	datastore "github.com/blocknative/dreamboat/datastore"
	"github.com/blocknative/dreamboat/test/common"
	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/blocknative/dreamboat/structs"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"

	"github.com/stretchr/testify/require"
)

func TestPutGetPayload(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newMockDatastore()
	cache, _ := lru.New[structs.PayloadKey, structs.BlockBidAndTrace](10)
	ds := datastore.Datastore{TTLStorage: store, PayloadCache: cache}

	payload := randomBlockBidAndTrace()

	// put
	key := structs.PayloadKey{
		BlockHash: payload.Trace.Message.BlockHash,
		Proposer:  payload.Trace.Message.ProposerPubkey,
		Slot:      structs.Slot(payload.Trace.Message.Slot),
	}
	err := ds.PutPayload(ctx, key, payload, time.Minute)
	require.NoError(t, err)

	// get
	gotPayload, _, err := ds.GetPayload(ctx, key)
	require.NoError(t, err)
	require.EqualValues(t, *payload, *gotPayload)
}

func randomHeaderAndTrace() structs.HeaderAndTrace {
	block := randomBlockBidAndTrace()
	header, _ := types.PayloadToPayloadHeader(block.Payload.Data)

	return structs.HeaderAndTrace{
		Header: header,
		Trace: &structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace:    *block.Trace.Message,
				BlockNumber: header.BlockNumber,
				NumTx:       999,
			},
			Timestamp: uint64(time.Now().UnixMicro()),
		},
	}
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

func randomBlockBidAndTrace() *structs.BlockBidAndTrace {

	sk, _, _ := bls.GenerateNewKeypair()

	pk := types.PublicKey(random48Bytes())

	payload := randomPayload()

	relaySigningDomain, _ := common.ComputeDomain(
		types.DomainTypeAppBuilder,
		types.Root{}.String())

	msg := &types.BidTrace{
		Slot:                 rand.Uint64(),
		ParentHash:           types.Hash(random32Bytes()),
		BlockHash:            types.Hash(random32Bytes()),
		BuilderPubkey:        types.PublicKey(random48Bytes()),
		ProposerPubkey:       types.PublicKey(random48Bytes()),
		ProposerFeeRecipient: types.Address(random20Bytes()),
		Value:                types.IntToU256(rand.Uint64()),
	}

	signature, _ := types.SignMessage(msg, relaySigningDomain, sk)

	submitRequest := &types.BuilderSubmitBlockRequest{
		Signature:        signature,
		Message:          msg,
		ExecutionPayload: payload,
	}

	signedBuilderBid, _ := submitRequest.ToSignedBuilderBid(
		submitRequest,
		sk,
		&pk,
		relaySigningDomain,
	)

	blockBidAndTrace := submitRequest.ToBlockBidAndTrace("bellatrix", signedBuilderBid, submitRequest)

	// BLOCK PARAMS HAS TO BE THE SAME!!!!
	blockBidAndTrace.Trace.Message.BlockHash = blockBidAndTrace.Payload.Data.BlockHash()

	return &blockBidAndTrace
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

func random32Bytes() (b [32]byte) {
	rand.Read(b[:])
	return b
}

func random48Bytes() (b [48]byte) {
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
