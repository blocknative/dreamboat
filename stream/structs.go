package stream

import (
	"github.com/blocknative/dreamboat/structs"
	"github.com/blocknative/dreamboat/structs/forks/bellatrix"
	"github.com/blocknative/dreamboat/structs/forks/capella"
)

type Metadata struct {
	Source       string
	ForkEncoding ForkVersionFormat
}

type JsonItem struct {
	StreamMeta Metadata
	StreamData []byte
}

func (d JsonItem) Data() []byte {
	return d.StreamData
}

func (d JsonItem) Meta() Metadata {
	return d.StreamMeta
}

type HeaderWithSlot struct {
	ExecutionPayloadHeader structs.ExecutionPayloadHeader
	HeaderSlot             uint64
}

type CapellaHeaderWithSlot struct {
	ExecutionPayloadHeader capella.ExecutionPayloadHeader
	HeaderSlot             uint64
}

func (b CapellaHeaderWithSlot) Header() structs.ExecutionPayloadHeader {
	return &b.ExecutionPayloadHeader
}

func (b CapellaHeaderWithSlot) Slot() uint64 {
	return b.HeaderSlot
}

type BellatrixHeaderWithSlot struct {
	ExecutionPayloadHeader bellatrix.ExecutionPayloadHeader
	HeaderSlot             uint64
}

func (b BellatrixHeaderWithSlot) Header() structs.ExecutionPayloadHeader {
	return &b.ExecutionPayloadHeader
}

func (b BellatrixHeaderWithSlot) Slot() uint64 {
	return b.HeaderSlot
}
