package capella_test

import (
	crand "crypto/rand"
	"math/rand"
	"testing"

	"github.com/blocknative/dreamboat/structs"
	"github.com/blocknative/dreamboat/structs/forks/bellatrix"
	"github.com/blocknative/dreamboat/structs/forks/capella"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/stretchr/testify/require"
)

func TestEncodeDecode_BuilderBidExtended_SSZ(t *testing.T) {
	extraDataArray := random20Bytes()
	expected := capella.BuilderBidExtended{
		CapellaBuilderBid: capella.BuilderBid{
			CapellaHeader: &capella.ExecutionPayloadHeader{
				ExecutionPayloadHeader: types.ExecutionPayloadHeader{
					ParentHash:       random32Bytes(),
					FeeRecipient:     random20Bytes(),
					StateRoot:        random32Bytes(),
					ReceiptsRoot:     random32Bytes(),
					LogsBloom:        random256Bytes(),
					Random:           random32Bytes(),
					BlockNumber:      rand.Uint64(),
					GasLimit:         rand.Uint64(),
					GasUsed:          rand.Uint64(),
					Timestamp:        rand.Uint64(),
					ExtraData:        extraDataArray[:],
					BaseFeePerGas:    random32Bytes(),
					BlockHash:        random32Bytes(),
					TransactionsRoot: random32Bytes(),
				},
				WithdrawalsRoot: random32Bytes(),
			},
			CapellaValue:  random32Bytes(),
			CapellaPubkey: random48Bytes(),
		},
		CapellaProposer: random48Bytes(),
		CapellaSlot:     rand.Uint64(),
	}

	b, err := expected.MarshalSSZ()
	require.NoError(t, err)

	var got capella.BuilderBidExtended

	err = got.UnmarshalSSZ(b)
	require.NoError(t, err)

	require.EqualValues(t, expected.CapellaBuilderBid.CapellaHeader, got.CapellaBuilderBid.CapellaHeader)
	require.EqualValues(t, expected.CapellaBuilderBid.CapellaValue, got.CapellaBuilderBid.CapellaValue)
	require.EqualValues(t, expected.CapellaBuilderBid.CapellaPubkey, got.CapellaBuilderBid.CapellaPubkey)
	require.EqualValues(t, expected.CapellaProposer, got.CapellaProposer)
	require.EqualValues(t, expected.CapellaSlot, got.CapellaSlot)
}

func TestEncodeDecode_BlockAndTraceExtended(t *testing.T) {
	extraDataArray := random20Bytes()
	expected := capella.BlockAndTraceExtended{
		CapellaPayload: capella.GetPayloadResponse{
			CapellaVersion: types.VersionString("version"),
			CapellaData: capella.ExecutionPayload{
				ExecutionPayload: bellatrix.ExecutionPayload{
					EpParentHash:    random32Bytes(),
					EpFeeRecipient:  random20Bytes(),
					EpStateRoot:     random32Bytes(),
					EpReceiptsRoot:  random32Bytes(),
					EpLogsBloom:     random256Bytes(),
					EpRandom:        random32Bytes(),
					EpBlockNumber:   rand.Uint64(),
					EpGasLimit:      rand.Uint64(),
					EpGasUsed:       rand.Uint64(),
					EpTimestamp:     rand.Uint64(),
					EpExtraData:     extraDataArray[:],
					EpBaseFeePerGas: random32Bytes(),
					EpBlockHash:     random32Bytes(),
					EpTransactions:  randomTransactions(55),
				},
				EpWithdrawals: randomWithdrawals(55),
			},
		},
		CapellaExecutionHeaderHash: random32Bytes(),
		CapellaTrace: capella.SignedBidTrace{
			Signature: random96Bytes(),
			Message: capella.BidTrace{
				Slot:                 rand.Uint64(),
				ParentHash:           random32Bytes(),
				BlockHash:            random32Bytes(),
				BuilderPubkey:        random48Bytes(),
				ProposerPubkey:       random48Bytes(),
				ProposerFeeRecipient: random20Bytes(),
				GasLimit:             rand.Uint64(),
				GasUsed:              rand.Uint64(),
				Value:                random32Bytes(),
			},
		},
	}

	b, err := expected.MarshalSSZ()
	require.NoError(t, err)

	var got capella.BlockAndTraceExtended
	err = got.UnmarshalSSZ(b)
	require.NoError(t, err)

	// Validate execution payload.
	require.EqualValues(t, expected.CapellaPayload.CapellaData.EpParentHash, got.CapellaPayload.CapellaData.EpParentHash)
	require.EqualValues(t, expected.CapellaPayload.CapellaData.EpFeeRecipient, got.CapellaPayload.CapellaData.EpFeeRecipient)
	require.EqualValues(t, expected.CapellaPayload.CapellaData.EpStateRoot, got.CapellaPayload.CapellaData.EpStateRoot)
	require.EqualValues(t, expected.CapellaPayload.CapellaData.EpStateRoot, got.CapellaPayload.CapellaData.EpStateRoot)
	require.EqualValues(t, expected.CapellaPayload.CapellaData.EpReceiptsRoot, got.CapellaPayload.CapellaData.EpReceiptsRoot)
	require.EqualValues(t, expected.CapellaPayload.CapellaData.EpLogsBloom, got.CapellaPayload.CapellaData.EpLogsBloom)
	require.EqualValues(t, expected.CapellaPayload.CapellaData.EpRandom, got.CapellaPayload.CapellaData.EpRandom)
	require.EqualValues(t, expected.CapellaPayload.CapellaData.EpBlockNumber, got.CapellaPayload.CapellaData.EpBlockNumber)
	require.EqualValues(t, expected.CapellaPayload.CapellaData.EpGasLimit, got.CapellaPayload.CapellaData.EpGasLimit)
	require.EqualValues(t, expected.CapellaPayload.CapellaData.EpGasUsed, got.CapellaPayload.CapellaData.EpGasUsed)
	require.EqualValues(t, expected.CapellaPayload.CapellaData.EpTimestamp, got.CapellaPayload.CapellaData.EpTimestamp)
	require.EqualValues(t, expected.CapellaPayload.CapellaData.EpExtraData, got.CapellaPayload.CapellaData.EpExtraData)
	require.EqualValues(t, expected.CapellaPayload.CapellaData.EpBaseFeePerGas, got.CapellaPayload.CapellaData.EpBaseFeePerGas)
	require.EqualValues(t, expected.CapellaPayload.CapellaData.EpBlockHash, got.CapellaPayload.CapellaData.EpBlockHash)
	for i, tx := range expected.CapellaPayload.CapellaData.EpTransactions {
		require.EqualValues(t, tx, got.CapellaPayload.CapellaData.EpTransactions[i])
	}
	for i, w := range expected.CapellaPayload.CapellaData.EpWithdrawals {
		require.EqualValues(t, w, got.CapellaPayload.CapellaData.EpWithdrawals[i])
	}

	// Validate ExecutionHeaderHash.
	require.EqualValues(t, expected.CapellaExecutionHeaderHash, got.CapellaExecutionHeaderHash)

	// Validate Trace.
	require.EqualValues(t, expected.CapellaTrace.Signature, got.CapellaTrace.Signature)
	require.EqualValues(t, expected.CapellaTrace.Message.Slot, got.CapellaTrace.Message.Slot)
	require.EqualValues(t, expected.CapellaTrace.Message.ParentHash, got.CapellaTrace.Message.ParentHash)
	require.EqualValues(t, expected.CapellaTrace.Message.BlockHash, got.CapellaTrace.Message.BlockHash)
	require.EqualValues(t, expected.CapellaTrace.Message.BuilderPubkey, got.CapellaTrace.Message.BuilderPubkey)
	require.EqualValues(t, expected.CapellaTrace.Message.ProposerPubkey, got.CapellaTrace.Message.ProposerPubkey)
	require.EqualValues(t, expected.CapellaTrace.Message.ProposerFeeRecipient, got.CapellaTrace.Message.ProposerFeeRecipient)
	require.EqualValues(t, expected.CapellaTrace.Message.GasLimit, got.CapellaTrace.Message.GasLimit)
	require.EqualValues(t, expected.CapellaTrace.Message.GasUsed, got.CapellaTrace.Message.GasUsed)
	require.EqualValues(t, expected.CapellaTrace.Message.Value, got.CapellaTrace.Message.Value)
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

func randomTransactions(size int) []hexutil.Bytes {
	txs := make([]hexutil.Bytes, 0, size)
	for i := 0; i < size; i++ {
		tx := make([]byte, 300)
		crand.Read(tx)
		txs = append(txs, tx)
	}
	return txs
}

func randomWithdrawals(size int) structs.Withdrawals {
	ws := make([]*structs.Withdrawal, 0, size)
	for i := 0; i < size; i++ {
		w := structs.Withdrawal{
			Index:          rand.Uint64(),
			ValidatorIndex: rand.Uint64(),
			Address:        random20Bytes(),
			Amount:         rand.Uint64(),
		}
		ws = append(ws, &w)
	}
	return ws
}
