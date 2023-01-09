package stream_test

import (
	"math/rand"
	"testing"

	pkg "github.com/blocknative/dreamboat/pkg"
	relay "github.com/blocknative/dreamboat/pkg/relay"
	"github.com/blocknative/dreamboat/pkg/stream"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestConvertStreamBlock(t *testing.T) {
	expected := randomBlockAndTrace()
	sBlock := stream.ToStreamBlock(expected, true, "")

	got := stream.ToBlockAndTrace(sBlock)

	require.EqualValues(t, expected, got)
}

func TestMasrshalStreamBlock(t *testing.T) {
	expected := randomBlockAndTrace()
	sBlock := stream.ToStreamBlock(expected, true, "")

	b, err := proto.Marshal(sBlock)
	require.NoError(t, err)

	gotSBlock := stream.StreamBlock{}
	err = proto.Unmarshal(b, &gotSBlock)
	require.NoError(t, err)

	got := stream.ToBlockAndTrace(&gotSBlock)

	require.EqualValues(t, expected, got)
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

func randomBlockAndTrace() *structs.BlockAndTrace {

	sk, _, _ := bls.GenerateNewKeypair()

	pk := types.PublicKey(random48Bytes())

	payload := randomPayload()
	GenesisForkVersionMainnet := "0x00000000"

	relaySigningDomain, _ := pkg.ComputeDomain(
		types.DomainTypeAppBuilder,
		GenesisForkVersionMainnet,
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

	signedBuilderBid, _ := relay.SubmitBlockRequestToSignedBuilderBid(
		submitRequest,
		sk,
		&pk,
		relaySigningDomain,
	)

	blockBidAndTrace := relay.SubmitBlockRequestToBlockBidAndTrace(signedBuilderBid, submitRequest)

	// BLOCK PARAMS HAS TO BE THE SAME!!!!
	blockBidAndTrace.Trace.Message.BlockHash = blockBidAndTrace.Payload.Data.BlockHash

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

func randomRegistration() types.SignedValidatorRegistration {
	msg := &types.RegisterValidatorRequestMessage{
		FeeRecipient: types.Address(random20Bytes()),
		GasLimit:     rand.Uint64(),
		Timestamp:    rand.Uint64(),
		Pubkey:       types.PublicKey(random48Bytes()),
	}
	return types.SignedValidatorRegistration{
		Message:   msg,
		Signature: types.Signature(random96Bytes()),
	}
}

func random32Bytes() (b [32]byte) {
	rand.Read(b[:])
	return b
}

func random48Bytes() (b [48]byte) {
	rand.Read(b[:])
	return b
}

func random96Bytes() (b [96]byte) {
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
