package stream

import (
	"fmt"
	"time"

	"github.com/blocknative/dreamboat/structs"
	"github.com/blocknative/dreamboat/structs/forks/bellatrix"
	"github.com/blocknative/dreamboat/structs/forks/capella"
	"github.com/flashbots/go-boost-utils/types"
)

type GenericStreamBlock struct {
	Block        structs.BlockBidAndTrace
	IsBlockCache bool
	StreamSource string
}

type CapellaStreamBlock struct {
	Block        capella.BlockBidAndTrace
	IsBlockCache bool
	StreamSource string
}

func (b CapellaStreamBlock) BlockBidAndTrace() structs.BlockBidAndTrace {
	return &b.Block
}

func (b CapellaStreamBlock) IsCache() bool {
	return b.IsBlockCache
}

func (b CapellaStreamBlock) Source() string {
	return b.StreamSource
}

func (b CapellaStreamBlock) CompleteBlock() (structs.CompleteBlockstruct, error) {
	var cbs structs.CompleteBlockstruct

	cbs.Payload = &b.Block

	header, err := bellatrix.PayloadToPayloadHeader(b.Block.ExecutionPayload())
	if err != nil {
		return structs.CompleteBlockstruct{}, fmt.Errorf("failed to create header: %w", err)
	}
	cbs.Header = structs.HeaderAndTrace{
		Header: header,
		Trace: structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: types.BidTrace{
					Slot:                 b.Block.Trace.Message.Slot,
					ParentHash:           b.Block.Trace.Message.ParentHash,
					BlockHash:            b.Block.Trace.Message.BlockHash,
					BuilderPubkey:        b.Block.Trace.Message.BuilderPubkey,
					ProposerPubkey:       b.Block.Trace.Message.ProposerPubkey,
					ProposerFeeRecipient: b.Block.Trace.Message.ProposerFeeRecipient,
					Value:                b.Block.Trace.Message.Value,
					GasLimit:             b.Block.Trace.Message.GasLimit,
					GasUsed:              b.Block.Trace.Message.GasUsed,
				},
				BlockNumber: b.Block.Payload.CapellaData.BlockNumber(),
				NumTx:       uint64(len(b.Block.Payload.CapellaData.Transactions())),
			},
			Timestamp:   uint64(time.Now().UnixMilli() / 1_000),
			TimestampMs: uint64(time.Now().UnixMilli()),
		},
	}

	return cbs, nil
}

func (b CapellaStreamBlock) Loggable() map[string]any {
	return map[string]any{
		"slot":      b.Block.Trace.Message.Slot,
		"blockHash": b.Block.Trace.Message.BlockHash,
	}
}

type BellatrixStreamBlock struct {
	Block        bellatrix.BlockBidAndTrace
	IsBlockCache bool
	StreamSource string
}

func (b BellatrixStreamBlock) BlockBidAndTrace() structs.BlockBidAndTrace {
	return &b.Block
}

func (b BellatrixStreamBlock) IsCache() bool {
	return b.IsBlockCache
}

func (b BellatrixStreamBlock) Source() string {
	return b.StreamSource
}

func (b BellatrixStreamBlock) CompleteBlock() (structs.CompleteBlockstruct, error) {
	var cbs structs.CompleteBlockstruct

	cbs.Payload = &b.Block

	header, err := bellatrix.PayloadToPayloadHeader(b.Block.ExecutionPayload())
	if err != nil {
		return structs.CompleteBlockstruct{}, fmt.Errorf("failed to create header: %w", err)
	}
	cbs.Header = structs.HeaderAndTrace{
		Header: header,
		Trace: structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: types.BidTrace{
					Slot:                 b.Block.Trace.Message.Slot,
					ParentHash:           b.Block.Trace.Message.ParentHash,
					BlockHash:            b.Block.Trace.Message.BlockHash,
					BuilderPubkey:        b.Block.Trace.Message.BuilderPubkey,
					ProposerPubkey:       b.Block.Trace.Message.ProposerPubkey,
					ProposerFeeRecipient: b.Block.Trace.Message.ProposerFeeRecipient,
					Value:                b.Block.Trace.Message.Value,
					GasLimit:             b.Block.Trace.Message.GasLimit,
					GasUsed:              b.Block.Trace.Message.GasUsed,
				},
				BlockNumber: b.Block.Payload.BellatrixData.BlockNumber(),
				NumTx:       uint64(len(b.Block.Payload.BellatrixData.Transactions())),
			},
			Timestamp:   uint64(time.Now().UnixMilli() / 1_000),
			TimestampMs: uint64(time.Now().UnixMilli()),
		},
	}

	return cbs, nil
}

func (b BellatrixStreamBlock) Loggable() map[string]any {
	return map[string]any{
		"slot":      b.Block.Trace.Message.Slot,
		"blockHash": b.Block.Trace.Message.BlockHash,
	}
}

type SlotDelivered struct {
	Slot uint64
}
