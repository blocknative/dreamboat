package capella_test

import (
	"math/rand"
	"testing"

	"github.com/blocknative/dreamboat/structs/forks/capella"
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
