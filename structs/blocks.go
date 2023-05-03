package structs

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
)

type SubmitBlockRequest interface {
	Raw() []byte
	Slot() uint64
	BlockHash() types.Hash
	ParentHash() types.Hash
	TraceBlockHash() types.Hash
	TraceParentHash() types.Hash
	BuilderPubkey() types.PublicKey
	ProposerPubkey() types.PublicKey
	ProposerFeeRecipient() types.Address
	Value() types.U256Str
	Signature() types.Signature
	Timestamp() uint64
	Random() types.Hash
	Withdrawals() Withdrawals

	NumTx() uint64

	ComputeSigningRoot(d types.Domain) ([32]byte, error)

	ToPayloadKey() PayloadKey

	PreparePayloadContents(sk *bls.SecretKey, pubkey *types.PublicKey, domain types.Domain) (cbs CompleteBlockstruct, err error)
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
}

type GetPayloadResponse interface {
	Data() ExecutionPayload
}

type SignedBlindedBeaconBlock interface {
	Raw() []byte
	Slot() uint64
	BlockHash() types.Hash
	BlockNumber() uint64
	ProposerIndex() uint64
	Signature() types.Signature

	ComputeSigningRoot(d types.Domain) ([32]byte, error)

	ToBeaconBlock(executionPayload ExecutionPayload) (SignedBeaconBlock, error)
	ToPayloadKey(pk types.PublicKey) (PayloadKey, error)

	ExecutionHeaderHash() (types.Hash, error)

	Loggable() map[string]any
}

// BuilderBid https://github.com/ethereum/builder-specs/pull/2/files#diff-b37cbf48e8754483e30e7caaadc5defc8c3c6e1aaf3273ee188d787b7c75d993
type BuilderBid interface {
	Value() types.U256Str
	Pubkey() types.PublicKey

	HashTreeRoot() ([32]byte, error)
}

// SignedBuilderBid https://github.com/ethereum/builder-specs/pull/2/files#diff-b37cbf48e8754483e30e7caaadc5defc8c3c6e1aaf3273ee188d787b7c75d993
type SignedBuilderBid interface {
	Signature() types.Signature
	Value() types.U256Str
}

type GetHeaderResponse interface {
	Version() types.VersionString
	Data() SignedBuilderBid
}

type SignedBeaconBlock interface {
	Signature() types.Signature
}
