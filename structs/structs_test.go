package structs_test

import (
	crand "crypto/rand"
	"math/rand"
	"testing"

	"github.com/attestantio/go-eth2-client/spec/bellatrix"
	attCapella "github.com/attestantio/go-eth2-client/spec/capella"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/blocknative/dreamboat/structs/forks/capella"
	"github.com/flashbots/mev-boost-relay/common"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"

	bCapella "github.com/attestantio/go-builder-client/api/capella"
	v1 "github.com/attestantio/go-builder-client/api/v1"

	"github.com/flashbots/go-boost-utils/types"
)

func TestEncodeDecode_SubmitBlockRequestSSZ(t *testing.T) {
	req, err := validSubmitBlockRequest()
	require.NoError(t, err)

	b, err := req.Capella.MarshalSSZ()
	require.NoError(t, err)

	creq := capella.SubmitBlockRequest{}
	err = creq.UnmarshalSSZ(b)
	require.NoError(t, err)

	// Validate trace fields.
	require.EqualValues(t, req.Capella.Message.Slot, creq.CapellaMessage.Slot)
	require.EqualValues(t, req.Capella.Message.ParentHash, creq.CapellaMessage.ParentHash)
	require.EqualValues(t, req.Capella.Message.BlockHash, creq.CapellaMessage.BlockHash)
	require.EqualValues(t, req.Capella.Message.BuilderPubkey, creq.CapellaMessage.BuilderPubkey)
	require.EqualValues(t, req.Capella.Message.ProposerPubkey, creq.CapellaMessage.ProposerPubkey)
	require.EqualValues(t, req.Capella.Message.ProposerFeeRecipient, creq.CapellaMessage.ProposerFeeRecipient)
	require.EqualValues(t, req.Capella.Message.GasLimit, creq.CapellaMessage.GasLimit)
	require.EqualValues(t, req.Capella.Message.GasUsed, creq.CapellaMessage.GasUsed)
	require.True(t, req.Capella.Message.Value.ToBig().Cmp(creq.CapellaMessage.Value.BigInt()) == 0)

	// Validate signature.
	require.EqualValues(t, req.Capella.Signature, creq.CapellaSignature)

	// Validate exeuction payload fields.
	require.EqualValues(t, req.Capella.ExecutionPayload.ParentHash, creq.CapellaExecutionPayload.EpParentHash)
	require.EqualValues(t, req.Capella.ExecutionPayload.FeeRecipient, creq.CapellaExecutionPayload.EpFeeRecipient)
	require.EqualValues(t, req.Capella.ExecutionPayload.StateRoot, creq.CapellaExecutionPayload.EpStateRoot)
	require.EqualValues(t, req.Capella.ExecutionPayload.ReceiptsRoot, creq.CapellaExecutionPayload.EpReceiptsRoot)
	require.EqualValues(t, req.Capella.ExecutionPayload.LogsBloom, creq.CapellaExecutionPayload.EpLogsBloom)
	require.EqualValues(t, req.Capella.ExecutionPayload.PrevRandao, creq.CapellaExecutionPayload.EpRandom)
	require.EqualValues(t, req.Capella.ExecutionPayload.BlockNumber, creq.CapellaExecutionPayload.EpBlockNumber)
	require.EqualValues(t, req.Capella.ExecutionPayload.GasLimit, creq.CapellaExecutionPayload.EpGasLimit)
	require.EqualValues(t, req.Capella.ExecutionPayload.GasUsed, creq.CapellaExecutionPayload.EpGasUsed)
	require.EqualValues(t, req.Capella.ExecutionPayload.Timestamp, creq.CapellaExecutionPayload.EpTimestamp)
	require.EqualValues(t, req.Capella.ExecutionPayload.ExtraData, creq.CapellaExecutionPayload.EpExtraData)
	require.EqualValues(t, req.Capella.ExecutionPayload.BaseFeePerGas, creq.CapellaExecutionPayload.EpBaseFeePerGas)
	require.EqualValues(t, req.Capella.ExecutionPayload.BlockHash, creq.CapellaExecutionPayload.EpBlockHash)
	for i, tx := range req.Capella.ExecutionPayload.Transactions {
		require.EqualValues(t, tx, creq.CapellaExecutionPayload.EpTransactions[i])
	}
	for i, wd := range req.Capella.ExecutionPayload.Withdrawals {
		require.EqualValues(t, wd, creq.CapellaExecutionPayload.EpWithdrawals[i])
	}
}

func validSubmitBlockRequest() (*common.BuilderSubmitBlockRequest, error) {
	payload := randomPayload()

	msg := &v1.BidTrace{
		Slot:                 rand.Uint64(),
		ParentHash:           phase0.Hash32(random32Bytes()),
		BlockHash:            random32Bytes(),
		BuilderPubkey:        phase0.BLSPubKey(random48Bytes()),
		ProposerPubkey:       phase0.BLSPubKey(random48Bytes()),
		ProposerFeeRecipient: random20Bytes(),
		Value:                uint256.NewInt(1),
	}

	req := common.BuilderSubmitBlockRequest{
		Capella: &bCapella.SubmitBlockRequest{
			Signature:        phase0.BLSSignature(random96Bytes()),
			Message:          msg,
			ExecutionPayload: payload,
		},
	}
	return &req, nil
}

func randomPayload() *attCapella.ExecutionPayload {
	return &attCapella.ExecutionPayload{
		ParentHash:    phase0.Hash32(random32Bytes()),
		FeeRecipient:  bellatrix.ExecutionAddress(random20Bytes()),
		StateRoot:     types.Hash(random32Bytes()),
		ReceiptsRoot:  types.Hash(random32Bytes()),
		LogsBloom:     types.Bloom(random256Bytes()),
		PrevRandao:    random32Bytes(),
		BlockNumber:   rand.Uint64(),
		GasLimit:      rand.Uint64(),
		GasUsed:       rand.Uint64(),
		Timestamp:     rand.Uint64(),
		ExtraData:     types.ExtraData{},
		BaseFeePerGas: types.IntToU256(rand.Uint64()),
		BlockHash:     phase0.Hash32(random32Bytes()),
		Transactions:  randomTransactions(55), // 55 is based on our own builder stats of avg number of txs.
	}
}

func randomTransactions(size int) []bellatrix.Transaction {
	txs := make([]bellatrix.Transaction, 0, size)
	for i := 0; i < size; i++ {
		tx := make([]byte, 300)
		crand.Read(tx)
		txs = append(txs, tx)
	}
	return txs
}

func random32Bytes() (b [32]byte) {
	crand.Read(b[:])
	return b
}

func random48Bytes() (b [48]byte) {
	crand.Read(b[:])
	return b
}

func random96Bytes() (b [96]byte) {
	crand.Read(b[:])
	return b
}

func random20Bytes() (b [20]byte) {
	crand.Read(b[:])
	return b
}

func random256Bytes() (b [256]byte) {
	crand.Read(b[:])
	return b
}
