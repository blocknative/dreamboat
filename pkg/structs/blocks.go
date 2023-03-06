package structs

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
)

type SubmitBlockRequest interface {
	Slot() uint64
	BlockHash() types.Hash
	BuilderPubkey() types.PublicKey
	ProposerPubkey() types.PublicKey
	ProposerFeeRecipient() types.Address
	Value() types.U256Str
	Signature() types.Signature
	Timestamp() uint64

	ExecutionPayload() ExecutionPayload
	Message() *types.BidTrace

	//ToSignedBuilderBid(sk *bls.SecretKey, pubkey *types.PublicKey, domain types.Domain) (*types.SignedBuilderBid, error)
	ToBlockBidAndTrace(sk *bls.SecretKey, pubkey *types.PublicKey, domain types.Domain) (bbt BlockBidAndTrace, err error)
}

type ExecutionPayload interface {
	ParentHash() types.Hash
	FeeRecipient() types.Address
	StateRoot() types.Root
	ReceiptsRoot() types.Root
	LogsBloom() types.Bloom
	Random() types.Hash
	BlockNumber() uint64
	GasLimit() uint64
	GasUsed() uint64
	Timestamp() uint64
	ExtraData() types.ExtraData
	BaseFeePerGas() types.U256Str
	BlockHash() types.Hash
	Transactions() []hexutil.Bytes
	Withdrawals() Withdrawal
}

type Withdrawal interface {
	HashTreeRoot() ([32]byte, error)
}

type GetPayloadResponse struct {
	Version types.VersionString `json:"version"`
	Data    ExecutionPayload    `json:"data"`
}

type SignedBlindedBeaconBlock interface {
	Slot() uint64
	BlockHash() types.Hash
	BlockNumber() uint64
	ProposerIndex() uint64
	Signature() types.Signature

	ToBeaconBlock(executionPayload ExecutionPayload) *types.SignedBeaconBlock
	Message() types.HashTreeRoot
}

// BuilderBid https://github.com/ethereum/builder-specs/pull/2/files#diff-b37cbf48e8754483e30e7caaadc5defc8c3c6e1aaf3273ee188d787b7c75d993
type BuilderBid interface {
	Header() *ExecutionPayloadHeader
	Value() types.U256Str
	Pubkey() types.PublicKey
}

type ExecutionPayloadHeader struct {
	types.ExecutionPayloadHeader
	WithdrawalsRoot types.Root `json:"withdrawals_root,omitempty" ssz-size:"32"`
}
