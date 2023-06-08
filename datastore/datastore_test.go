package datastore_test

import (
	"context"
	"math/rand"
	crand "crypto/rand"
	"testing"
	"time"

	datastore "github.com/blocknative/dreamboat/datastore"
	"github.com/blocknative/dreamboat/structs/forks/bellatrix"
	"github.com/blocknative/dreamboat/structs/forks/capella"
	"github.com/blocknative/dreamboat/test/common"
	"github.com/google/uuid"

	tBadger "github.com/blocknative/dreamboat/datastore/transport/badger"

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

	tB, err := tBadger.Open("/tmp/" + t.Name() + uuid.New().String())
	require.NoError(t, err)
	defer tB.Close()

	ds := datastore.Datastore{TTLStorage: tB}

	payload := randomBlockAndTrace()

	// put
	key := structs.PayloadKey{
		BlockHash: payload.ExecutionPayload().BlockHash(),
		Proposer:  payload.Proposer(),
		Slot:      structs.Slot(payload.Slot()),
	}
	err = ds.PutPayload(ctx, key, payload, time.Minute)
	require.NoError(t, err)

	// get
	gotPayload, err := ds.GetPayload(ctx, structs.ForkCapella, key)
	require.NoError(t, err)
	require.EqualValues(t, payload, gotPayload)
}

func randomPayload() capella.ExecutionPayload {
	return capella.ExecutionPayload{
		ExecutionPayload: bellatrix.ExecutionPayload{
			EpParentHash:    types.Hash(random32Bytes()),
			EpFeeRecipient:  types.Address(random20Bytes()),
			EpStateRoot:     types.Hash(random32Bytes()),
			EpReceiptsRoot:  types.Hash(random32Bytes()),
			EpLogsBloom:     types.Bloom(random256Bytes()),
			EpRandom:        random32Bytes(),
			EpBlockNumber:   rand.Uint64(),
			EpGasLimit:      rand.Uint64(),
			EpGasUsed:       rand.Uint64(),
			EpTimestamp:     rand.Uint64(),
			EpExtraData:     types.ExtraData{},
			EpBaseFeePerGas: types.IntToU256(rand.Uint64()),
			EpBlockHash:     types.Hash(random32Bytes()),
			EpTransactions:  randomTransactions(2),
		},
	}
}

func randomBlockAndTrace() structs.BlockAndTraceExtended {
	sk, _, _ := bls.GenerateNewKeypair()

	// pk := types.PublicKey(random48Bytes())

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

	return &capella.BlockAndTraceExtended{
		CapellaPayload: capella.GetPayloadResponse{
			CapellaVersion: types.VersionString("capella"),
			CapellaData:    payload,
		},
		CapellaTrace: capella.SignedBidTrace{
			Signature: signature,
			Message:   capella.BidTrace(*msg),
		},
		CapellaExecutionHeaderHash: types.Hash{},
	}
}

func randomTransactions(size int) []hexutil.Bytes {
	txs := make([]hexutil.Bytes, 0, size)
	for i := 0; i < size; i++ {
		tx := make([]byte, rand.Intn(32))
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

func random20Bytes() (b [20]byte) {
	crand.Read(b[:])
	return b
}

func random256Bytes() (b [256]byte) {
	crand.Read(b[:])
	return b
}
