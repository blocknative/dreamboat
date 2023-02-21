package stream

import (
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/types"
)

func ToStreamBlock(block *structs.BlockAndTrace, isCache bool, source string) *StreamBlock {
	var transactions [][]byte
	for _, tx := range block.Payload.Data.Transactions {
		transactions = append(transactions, tx)
	}

	return &StreamBlock{
		Source:  source,
		IsCache: isCache,
		Block: &PayloadAndTrace{
			Trace: &SignedBidTrace{
				Signature: block.Trace.Signature[:],
				Message: &BidTrace{
					Slot:                 block.Trace.Message.Slot,
					ParentHash:           block.Trace.Message.ParentHash[:],
					BlockHash:            block.Trace.Message.BlockHash[:],
					BuilderPubkey:        block.Trace.Message.BuilderPubkey[:],
					ProposerPubkey:       block.Trace.Message.ProposerPubkey[:],
					ProposerFeeRecipient: block.Trace.Message.ProposerFeeRecipient[:],
					GasLimit:             block.Trace.Message.GasLimit,
					GasUsed:              block.Trace.Message.GasUsed,
					Value:                block.Trace.Message.Value[:],
				},
			},
			Payload: &PayloadWithVersion{
				Version: string(block.Payload.Version),
				Payload: &ExecutionPayload{
					ParentHash:    block.Payload.Data.ParentHash[:],
					FeeRecipient:  block.Payload.Data.FeeRecipient[:],
					StateRoot:     block.Payload.Data.StateRoot[:],
					ReceiptsRoot:  block.Payload.Data.ReceiptsRoot[:],
					LogsBloom:     block.Payload.Data.LogsBloom[:],
					Random:        block.Payload.Data.Random[:],
					BlockNumber:   block.Payload.Data.BlockNumber,
					GasLimit:      block.Payload.Data.GasLimit,
					GasUsed:       block.Payload.Data.GasUsed,
					Timestamp:     block.Payload.Data.Timestamp,
					ExtraData:     block.Payload.Data.ExtraData,
					BaseFeePerGas: block.Payload.Data.BaseFeePerGas[:],
					BlockHash:     block.Payload.Data.BlockHash[:],
					Transactions:  transactions,
				},
			},
		},
	}
}

func ToBlockAndTrace(streamBlock *StreamBlock) *structs.BlockAndTrace {
	block := structs.BlockAndTrace{
		Trace: &types.SignedBidTrace{
			Message: &types.BidTrace{},
		},
		Payload: &types.GetPayloadResponse{
			Data: &types.ExecutionPayload{},
		},
	}

	// copy trace
	copy(block.Trace.Signature[:], streamBlock.Block.Trace.Signature)
	block.Trace.Message.Slot = streamBlock.Block.Trace.Message.Slot
	copy(block.Trace.Message.ParentHash[:], streamBlock.Block.Trace.Message.ParentHash)
	copy(block.Trace.Message.BlockHash[:], streamBlock.Block.Trace.Message.BlockHash)
	copy(block.Trace.Message.BuilderPubkey[:], streamBlock.Block.Trace.Message.BuilderPubkey)
	copy(block.Trace.Message.ProposerPubkey[:], streamBlock.Block.Trace.Message.ProposerPubkey)
	copy(block.Trace.Message.ProposerFeeRecipient[:], streamBlock.Block.Trace.Message.ProposerFeeRecipient)
	block.Trace.Message.GasLimit = streamBlock.Block.Trace.Message.GasLimit
	block.Trace.Message.GasUsed = streamBlock.Block.Trace.Message.GasUsed
	copy(block.Trace.Message.Value[:], streamBlock.Block.Trace.Message.Value)

	// copy payload
	block.Payload.Version = types.VersionString(streamBlock.Block.Payload.Version)
	copy(block.Payload.Data.ParentHash[:], streamBlock.Block.Payload.Payload.ParentHash)
	copy(block.Payload.Data.FeeRecipient[:], streamBlock.Block.Payload.Payload.FeeRecipient)
	copy(block.Payload.Data.StateRoot[:], streamBlock.Block.Payload.Payload.StateRoot)
	copy(block.Payload.Data.ReceiptsRoot[:], streamBlock.Block.Payload.Payload.ReceiptsRoot)
	copy(block.Payload.Data.LogsBloom[:], streamBlock.Block.Payload.Payload.LogsBloom)
	copy(block.Payload.Data.Random[:], streamBlock.Block.Payload.Payload.Random)
	block.Payload.Data.BlockNumber = streamBlock.Block.Payload.Payload.BlockNumber
	block.Payload.Data.GasLimit = streamBlock.Block.Payload.Payload.GasLimit
	block.Payload.Data.GasUsed = streamBlock.Block.Payload.Payload.GasUsed
	block.Payload.Data.Timestamp = streamBlock.Block.Payload.Payload.Timestamp
	block.Payload.Data.ExtraData = types.ExtraData{}
	copy(block.Payload.Data.ExtraData[:], streamBlock.Block.Payload.Payload.ExtraData)
	copy(block.Payload.Data.BaseFeePerGas[:], streamBlock.Block.Payload.Payload.BaseFeePerGas)
	copy(block.Payload.Data.BlockHash[:], streamBlock.Block.Payload.Payload.BlockHash)
	block.Payload.Data.Transactions = make([]hexutil.Bytes, 0)
	for _, tx := range streamBlock.Block.Payload.Payload.Transactions {
		block.Payload.Data.Transactions = append(block.Payload.Data.Transactions, tx)
	}

	return &block
}