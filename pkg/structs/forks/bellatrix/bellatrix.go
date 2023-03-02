package bellatrix

import (
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
)

// BuilderSubmitBlockRequest spec: https://flashbots.notion.site/Relay-API-Spec-5fb0819366954962bc02e81cb33840f5#fa719683d4ae4a57bc3bf60e138b0dc6
type SubmitBlockRequest struct {
	BellatrixSignature        types.Signature   `json:"signature" ssz-size:"96"`
	BellatrixMessage          *types.BidTrace   `json:"message"`
	BellatrixExecutionPayload *ExecutionPayload `json:"execution_payload"`
}

func (b *SubmitBlockRequest) Slot() uint64 {
	return b.BellatrixMessage.Slot
}

func (b *SubmitBlockRequest) BlockHash() types.Hash {
	return b.BellatrixExecutionPayload.EpBlockHash
}

func (b *SubmitBlockRequest) BuilderPubkey() types.PublicKey {
	return b.BellatrixMessage.BuilderPubkey
}

func (b *SubmitBlockRequest) ProposerPubkey() types.PublicKey {
	return b.BellatrixMessage.ProposerPubkey
}

func (b *SubmitBlockRequest) ProposerFeeRecipient() types.Address {
	return b.BellatrixMessage.ProposerFeeRecipient
}

func (b *SubmitBlockRequest) Value() types.U256Str {
	return b.BellatrixMessage.Value
}

func (b *SubmitBlockRequest) Signature() types.Signature {
	return b.BellatrixSignature
}

func (b *SubmitBlockRequest) Timestamp() uint64 {
	return b.BellatrixExecutionPayload.EpTimestamp
}

func (b *SubmitBlockRequest) ExecutionPayload() structs.ExecutionPayload {
	return b.BellatrixExecutionPayload
}

func (b *SubmitBlockRequest) Message() *types.BidTrace {
	return b.BellatrixMessage
}

func (s *SubmitBlockRequest) ToSignedBuilderBid(sk *bls.SecretKey, pubkey *types.PublicKey, domain types.Domain) (*types.SignedBuilderBid, error) {

	/*	if sbr == nil {
		return nil, ErrMissingRequest
	} */

	if sk == nil {
		return nil, ErrMissingSecretKey
	}

	header, err := types.PayloadToPayloadHeader(s.BellatrixExecutionPayload)
	if err != nil {
		return nil, err
	}

	builderBid := types.BuilderBid{
		Value:  s.Value(),
		Header: header,
		Pubkey: *pubkey,
	}

	sig, err := types.SignMessage(&builderBid, domain, sk)
	if err != nil {
		return nil, err
	}

	return &types.SignedBuilderBid{
		Message:   &builderBid,
		Signature: sig,
	}, nil
}

/*
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

// HashTreeRoot ssz hashes the BidTrace object
func (b *BidTrace) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(b)
}

// GetTree ssz hashes the BidTrace object
func (b *BidTrace) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(b)
}

// HashTreeRootWith ssz hashes the BidTrace object with a hasher
func (b *BidTrace) HashTreeRootWith(hh ssz.HashWalker) (err error) {
	indx := hh.Index()

	// Field (0) 'Slot'
	hh.PutUint64(b.Slot)

	// Field (1) 'ParentHash'
	hh.PutBytes(b.ParentHash[:])

	// Field (2) 'BlockHash'
	hh.PutBytes(b.BlockHash[:])

	// Field (3) 'BuilderPubkey'
	hh.PutBytes(b.BuilderPubkey[:])

	// Field (4) 'ProposerPubkey'
	hh.PutBytes(b.ProposerPubkey[:])

	// Field (5) 'ProposerFeeRecipient'
	hh.PutBytes(b.ProposerFeeRecipient[:])

	// Field (6) 'GasLimit'
	hh.PutUint64(b.GasLimit)

	// Field (7) 'GasUsed'
	hh.PutUint64(b.GasUsed)

	// Field (8) 'Value'
	hh.PutBytes(b.Value[:])

	hh.Merkleize(indx)
	return
}
*/

// ExecutionPayload https://github.com/ethereum/consensus-specs/blob/dev/specs/bellatrix/beacon-chain.md#executionpayload
type ExecutionPayload struct {
	EpParentHash    types.Hash      `json:"parent_hash" ssz-size:"32"`
	EpFeeRecipient  types.Address   `json:"fee_recipient" ssz-size:"20"`
	EpStateRoot     types.Root      `json:"state_root" ssz-size:"32"`
	EpReceiptsRoot  types.Root      `json:"receipts_root" ssz-size:"32"`
	EpLogsBloom     types.Bloom     `json:"logs_bloom" ssz-size:"256"`
	EpRandom        types.Hash      `json:"prev_randao" ssz-size:"32"`
	EpBlockNumber   uint64          `json:"block_number,string"`
	EpGasLimit      uint64          `json:"gas_limit,string"`
	EpGasUsed       uint64          `json:"gas_used,string"`
	EpTimestamp     uint64          `json:"timestamp,string"`
	EpExtraData     types.ExtraData `json:"extra_data" ssz-max:"32"`
	EpBaseFeePerGas types.U256Str   `json:"base_fee_per_gas" ssz-max:"32"`
	EpBlockHash     types.Hash      `json:"block_hash" ssz-size:"32"`
	EpTransactions  []hexutil.Bytes `json:"transactions" ssz-max:"1048576,1073741824" ssz-size:"?,?"`
}

func (ep *ExecutionPayload) ParentHash() types.Hash {
	return ep.EpParentHash
}
func (ep *ExecutionPayload) FeeRecipient() types.Address {
	return ep.EpFeeRecipient
}
func (ep *ExecutionPayload) StateRoot() types.Root {
	return ep.EpStateRoot
}
func (ep *ExecutionPayload) ReceiptsRoot() types.Root {
	return ep.EpReceiptsRoot
}
func (ep *ExecutionPayload) LogsBloom() types.Bloom {
	return ep.EpLogsBloom
}
func (ep *ExecutionPayload) Random() types.Hash {
	return ep.EpRandom
}
func (ep *ExecutionPayload) BlockNumber() uint64 {
	return ep.EpBlockNumber
}
func (ep *ExecutionPayload) GasLimit() uint64 {
	return ep.EpGasLimit
}
func (ep *ExecutionPayload) GasUsed() uint64 {
	return ep.EpGasLimit
}
func (ep *ExecutionPayload) Timestamp() uint64 {
	return ep.EpTimestamp
}
func (ep *ExecutionPayload) ExtraData() types.ExtraData {
	return ep.EpExtraData
}
func (ep *ExecutionPayload) BaseFeePerGas() types.U256Str {
	return ep.EpBaseFeePerGas
}
func (ep *ExecutionPayload) BlockHash() types.Hash {
	return ep.EpBlockHash
}
func (ep *ExecutionPayload) Transactions() []hexutil.Bytes {
	return ep.EpTransactions
}
func (ep *ExecutionPayload) Withdrawals() structs.Withdrawal {
	return nil
}

// SignedBlindedBeaconBlock https://github.com/ethereum/beacon-APIs/blob/master/types/bellatrix/block.yaml#L83
type SignedBlindedBeaconBlock struct {
	SMessage   *types.BlindedBeaconBlock `json:"message"`
	SSignature types.Signature           `json:"signature" ssz-size:"96"`
}

func (s *SignedBlindedBeaconBlock) Signature() types.Signature {
	return s.SSignature
}

func (s *SignedBlindedBeaconBlock) Slot() uint64 {
	return s.SMessage.Slot
}

func (s *SignedBlindedBeaconBlock) BlockHash() types.Hash {
	return s.SMessage.Body.ExecutionPayloadHeader.BlockHash
}

func (s *SignedBlindedBeaconBlock) BlockNumber() uint64 {
	return s.SMessage.Body.ExecutionPayloadHeader.BlockNumber
}

func (s *SignedBlindedBeaconBlock) ProposerIndex() uint64 {
	return s.SMessage.ProposerIndex
}

func (s *SignedBlindedBeaconBlock) ParentRoot() types.Root {
	return s.SMessage.ParentRoot
}

func (s *SignedBlindedBeaconBlock) StateRoot() types.Root {
	return s.SMessage.StateRoot
}

func (s *SignedBlindedBeaconBlock) ToBeaconBlock(executionPayload structs.ExecutionPayload) *types.SignedBeaconBlock {
	block := &types.SignedBeaconBlock{
		Signature: s.SSignature,
		Message: &types.BeaconBlock{
			Slot:          s.SMessage.Slot,
			ProposerIndex: s.SMessage.ProposerIndex,
			ParentRoot:    s.SMessage.ParentRoot,
			StateRoot:     s.SMessage.StateRoot,
			Body: &types.BeaconBlockBody{
				RandaoReveal:      s.SMessage.Body.RandaoReveal,
				Eth1Data:          s.SMessage.Body.Eth1Data,
				Graffiti:          s.SMessage.Body.Graffiti,
				ProposerSlashings: s.SMessage.Body.ProposerSlashings,
				AttesterSlashings: s.SMessage.Body.AttesterSlashings,
				Attestations:      s.SMessage.Body.Attestations,
				Deposits:          s.SMessage.Body.Deposits,
				VoluntaryExits:    s.SMessage.Body.VoluntaryExits,
				SyncAggregate:     s.SMessage.Body.SyncAggregate,
				ExecutionPayload:  executionPayload,
			},
		},
	}

	if block.Message.Body.ProposerSlashings == nil {
		block.Message.Body.ProposerSlashings = []*types.ProposerSlashing{}
	}
	if block.Message.Body.AttesterSlashings == nil {
		block.Message.Body.AttesterSlashings = []*types.AttesterSlashing{}
	}
	if block.Message.Body.Attestations == nil {
		block.Message.Body.Attestations = []*types.Attestation{}
	}
	if block.Message.Body.Deposits == nil {
		block.Message.Body.Deposits = []*types.Deposit{}
	}

	if block.Message.Body.VoluntaryExits == nil {
		block.Message.Body.VoluntaryExits = []*types.SignedVoluntaryExit{}
	}

	if block.Message.Body.Eth1Data == nil {
		block.Message.Body.Eth1Data = &types.Eth1Data{}
	}

	if block.Message.Body.SyncAggregate == nil {
		block.Message.Body.SyncAggregate = &types.SyncAggregate{}
	}

	if block.Message.Body.ExecutionPayload == nil {
		block.Message.Body.ExecutionPayload = &types.ExecutionPayload{}
	}

	if block.Message.Body.ExecutionPayload.ExtraData == nil {
		block.Message.Body.ExecutionPayload.ExtraData = types.ExtraData{}
	}

	if block.Message.Body.ExecutionPayload.Transactions == nil {
		block.Message.Body.ExecutionPayload.Transactions = []hexutil.Bytes{}
	}

	return block
}

/*
// BlindedBeaconBlock https://github.com/ethereum/beacon-APIs/blob/master/types/bellatrix/block.yaml#L74
type BlindedBeaconBlock struct {
	Slot          uint64                  `json:"slot,string"`
	ProposerIndex uint64                  `json:"proposer_index,string"`
	ParentRoot    Root                    `json:"parent_root" ssz-size:"32"`
	StateRoot     Root                    `json:"state_root" ssz-size:"32"`
	Body          *BlindedBeaconBlockBody `json:"body"`
}
*/

/*
// BlindedBeaconBlockBody https://github.com/ethereum/beacon-APIs/blob/master/types/bellatrix/block.yaml#L65
type BlindedBeaconBlockBody struct {
	RandaoReveal           Signature               `json:"randao_reveal" ssz-size:"96"`
	Eth1Data               *Eth1Data               `json:"eth1_data"`
	Graffiti               Hash                    `json:"graffiti" ssz-size:"32"`
	ProposerSlashings      []*ProposerSlashing     `json:"proposer_slashings" ssz-max:"16"`
	AttesterSlashings      []*AttesterSlashing     `json:"attester_slashings" ssz-max:"2"`
	Attestations           []*Attestation          `json:"attestations" ssz-max:"128"`
	Deposits               []*Deposit              `json:"deposits" ssz-max:"16"`
	VoluntaryExits         []*SignedVoluntaryExit  `json:"voluntary_exits" ssz-max:"16"`
	SyncAggregate          *SyncAggregate          `json:"sync_aggregate"`
	ExecutionPayloadHeader *ExecutionPayloadHeader `json:"execution_payload_header"`
}*/
