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
	structs.BlockBidAndTrace
	IsBlockCache bool
	StreamSource string
}

type CapellaStreamBlock struct {
	capella.BlockBidAndTrace
	IsBlockCache bool
	StreamSource string
}

func (b CapellaStreamBlock) Block() structs.BlockBidAndTrace {
	return &b.BlockBidAndTrace
}

func (b CapellaStreamBlock) IsCache() bool {
	return b.IsBlockCache
}

func (b CapellaStreamBlock) Source() string {
	return b.StreamSource
}

func (b CapellaStreamBlock) CompleteBlock() (structs.CompleteBlockstruct, error) {
	var cbs structs.CompleteBlockstruct

	cbs.Payload = &b.BlockBidAndTrace

	header, err := bellatrix.PayloadToPayloadHeader(b.ExecutionPayload())
	if err != nil {
		return structs.CompleteBlockstruct{}, fmt.Errorf("failed to create header: %w", err)
	}
	cbs.Header = structs.HeaderAndTrace{
		Header: header,
		Trace: structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: types.BidTrace{
					Slot:                 b.Trace.Message.Slot,
					ParentHash:           b.Trace.Message.ParentHash,
					BlockHash:            b.Trace.Message.BlockHash,
					BuilderPubkey:        b.Trace.Message.BuilderPubkey,
					ProposerPubkey:       b.Trace.Message.ProposerPubkey,
					ProposerFeeRecipient: b.Trace.Message.ProposerFeeRecipient,
					Value:                b.Trace.Message.Value,
					GasLimit:             b.Trace.Message.GasLimit,
					GasUsed:              b.Trace.Message.GasUsed,
				},
				BlockNumber: b.Payload.CapellaData.BlockNumber(),
				NumTx:       uint64(len(b.Payload.CapellaData.Transactions())),
			},
			Timestamp:   uint64(time.Now().UnixMilli() / 1_000),
			TimestampMs: uint64(time.Now().UnixMilli()),
		},
	}

	return cbs, nil
}

func (b CapellaStreamBlock) Loggable() map[string]any {
	return map[string]any{
		"slot":      b.Trace.Message.Slot,
		"blockHash": b.Trace.Message.BlockHash,
	}
}

type BellatrixStreamBlock struct {
	bellatrix.BlockBidAndTrace
	IsBlockCache bool
	StreamSource string
}

func (b BellatrixStreamBlock) Block() structs.BlockBidAndTrace {
	return &b.BlockBidAndTrace
}

func (b BellatrixStreamBlock) IsCache() bool {
	return b.IsBlockCache
}

func (b BellatrixStreamBlock) Source() string {
	return b.StreamSource
}

func (b BellatrixStreamBlock) CompleteBlock() (structs.CompleteBlockstruct, error) {
	var cbs structs.CompleteBlockstruct

	cbs.Payload = &b.BlockBidAndTrace

	header, err := bellatrix.PayloadToPayloadHeader(b.ExecutionPayload())
	if err != nil {
		return structs.CompleteBlockstruct{}, fmt.Errorf("failed to create header: %w", err)
	}
	cbs.Header = structs.HeaderAndTrace{
		Header: header,
		Trace: structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: types.BidTrace{
					Slot:                 b.Trace.Message.Slot,
					ParentHash:           b.Trace.Message.ParentHash,
					BlockHash:            b.Trace.Message.BlockHash,
					BuilderPubkey:        b.Trace.Message.BuilderPubkey,
					ProposerPubkey:       b.Trace.Message.ProposerPubkey,
					ProposerFeeRecipient: b.Trace.Message.ProposerFeeRecipient,
					Value:                b.Trace.Message.Value,
					GasLimit:             b.Trace.Message.GasLimit,
					GasUsed:              b.Trace.Message.GasUsed,
				},
				BlockNumber: b.Payload.BellatrixData.BlockNumber(),
				NumTx:       uint64(len(b.Payload.BellatrixData.Transactions())),
			},
			Timestamp:   uint64(time.Now().UnixMilli() / 1_000),
			TimestampMs: uint64(time.Now().UnixMilli()),
		},
	}

	return cbs, nil
}

func (b BellatrixStreamBlock) Loggable() map[string]any {
	return map[string]any{
		"slot":      b.Trace.Message.Slot,
		"blockHash": b.Trace.Message.BlockHash,
	}
}

type SlotDelivered struct {
	Slot uint64
}
