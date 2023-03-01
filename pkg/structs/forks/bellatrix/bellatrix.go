package bellatrix

import (
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/types"
)

// BuilderSubmitBlockRequest spec: https://flashbots.notion.site/Relay-API-Spec-5fb0819366954962bc02e81cb33840f5#fa719683d4ae4a57bc3bf60e138b0dc6
type BellatrixBuilderSubmitBlockRequest struct {
	BellatrixSignature        types.Signature   `json:"signature" ssz-size:"96"`
	Message                   *BidTrace         `json:"message"`
	BellatrixExecutionPayload *ExecutionPayload `json:"execution_payload"`
}

func (b *BellatrixBuilderSubmitBlockRequest) Slot() uint64 {
	return b.Message.Slot
}

func (b *BellatrixBuilderSubmitBlockRequest) BlockHash() types.Hash {
	return b.BellatrixExecutionPayload.BlockHash
}

func (b *BellatrixBuilderSubmitBlockRequest) BuilderPubkey() types.PublicKey {
	return b.Message.BuilderPubkey
}

func (b *BellatrixBuilderSubmitBlockRequest) ProposerPubkey() types.PublicKey {
	return b.Message.ProposerPubkey
}

func (b *BellatrixBuilderSubmitBlockRequest) ProposerFeeRecipient() types.Address {
	return b.Message.ProposerFeeRecipient
}

func (b *BellatrixBuilderSubmitBlockRequest) Value() types.U256Str {
	return b.Message.Value
}

func (b *BellatrixBuilderSubmitBlockRequest) Signature() types.Signature {
	return b.BellatrixSignature
}

func (b *BellatrixBuilderSubmitBlockRequest) ExecutionPayload() structs.ExecutionPayload {
	return b.ExecutionPayload
}

// BidTrace is public information about a bid: https://flashbots.notion.site/Relay-API-Spec-5fb0819366954962bc02e81cb33840f5#286c858c4ba24e58ada6348d8d4b71ec
type BidTrace struct {
	Slot                 uint64          `json:"slot,string"`
	ParentHash           types.Hash      `json:"parent_hash" ssz-size:"32"`
	BlockHash            types.Hash      `json:"block_hash" ssz-size:"32"`
	BuilderPubkey        types.PublicKey `json:"builder_pubkey" ssz-size:"48"`
	ProposerPubkey       types.PublicKey `json:"proposer_pubkey" ssz-size:"48"`
	ProposerFeeRecipient types.Address   `json:"proposer_fee_recipient" ssz-size:"20"`
	GasLimit             uint64          `json:"gas_limit,string"`
	GasUsed              uint64          `json:"gas_used,string"`
	Value                types.U256Str   `json:"value" ssz-size:"32"`
}

// SignedBidTrace is a BidTrace with a signature
type SignedBidTrace struct {
	Signature types.Signature `json:"signature" ssz-size:"96"`
	Message   *BidTrace       `json:"message"`
}

// ExecutionPayload https://github.com/ethereum/consensus-specs/blob/dev/specs/bellatrix/beacon-chain.md#executionpayload
type ExecutionPayload struct {
	ParentHash    types.Hash      `json:"parent_hash" ssz-size:"32"`
	FeeRecipient  types.Address   `json:"fee_recipient" ssz-size:"20"`
	StateRoot     types.Root      `json:"state_root" ssz-size:"32"`
	ReceiptsRoot  types.Root      `json:"receipts_root" ssz-size:"32"`
	LogsBloom     types.Bloom     `json:"logs_bloom" ssz-size:"256"`
	Random        types.Hash      `json:"prev_randao" ssz-size:"32"`
	BlockNumber   uint64          `json:"block_number,string"`
	GasLimit      uint64          `json:"gas_limit,string"`
	GasUsed       uint64          `json:"gas_used,string"`
	Timestamp     uint64          `json:"timestamp,string"`
	ExtraData     types.ExtraData `json:"extra_data" ssz-max:"32"`
	BaseFeePerGas types.U256Str   `json:"base_fee_per_gas" ssz-max:"32"`
	BlockHash     types.Hash      `json:"block_hash" ssz-size:"32"`
	Transactions  []hexutil.Bytes `json:"transactions" ssz-max:"1048576,1073741824" ssz-size:"?,?"`
}
