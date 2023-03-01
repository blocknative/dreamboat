package structs

import (
	"github.com/flashbots/go-boost-utils/types"
)

type BuilderSubmitBlockRequest interface {
	Slot() uint64
	BlockHash() types.Hash
	BuilderPubkey() types.PublicKey
	ProposerPubkey() types.PublicKey
	ProposerFeeRecipient() types.Address
	Value() types.U256Str
	Signature() types.Signature

	ExecutionPayload() ExecutionPayload
}

type ExecutionPayload interface {
	Slot() uint64
	BlockHash() types.Hash
	BuilderPubkey() types.PublicKey
	ProposerPubkey() types.PublicKey
	ProposerFeeRecipient() types.Address
	Value() types.U256Str
	Signature() types.Signature
}
