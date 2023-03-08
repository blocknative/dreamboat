package bellatrix

import (
	"errors"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ssz "github.com/ferranbt/fastssz"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
)

// BuilderSubmitBlockRequest spec: https://flashbots.notion.site/Relay-API-Spec-5fb0819366954962bc02e81cb33840f5#fa719683d4ae4a57bc3bf60e138b0dc6
type SubmitBlockRequest struct {
	BellatrixSignature        types.Signature  `json:"signature" ssz-size:"96"`
	BellatrixMessage          types.BidTrace   `json:"message"`
	BellatrixExecutionPayload ExecutionPayload `json:"execution_payload"`
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

func (b *SubmitBlockRequest) Random() types.Hash {
	return b.BellatrixExecutionPayload.EpRandom
}

func (b *SubmitBlockRequest) ComputeSigningRoot(d types.Domain) ([32]byte, error) {
	return types.ComputeSigningRoot(&b.BellatrixMessage, d)
}

func (s *SubmitBlockRequest) toSignedBuilderBid(sk *bls.SecretKey, pubkey *types.PublicKey, domain types.Domain) (*SignedBuilderBid, error) {
	header, err := PayloadToPayloadHeader(&s.BellatrixExecutionPayload)
	if err != nil {
		return nil, err
	}

	builderBid := BuilderBid{
		BellatrixValue:  s.Value(),
		BellatrixHeader: header,
		BellatrixPubkey: *pubkey,
	}

	sig, err := types.SignMessage(&builderBid, domain, sk)
	if err != nil {
		return nil, err
	}

	return &SignedBuilderBid{
		BellatrixMessage:   &builderBid,
		BellatrixSignature: sig,
	}, nil
}

func (s *SubmitBlockRequest) toBlockBidAndTrace(signedBuilderBid *SignedBuilderBid) (bbt structs.BlockBidAndTrace) { // TODO(l): remove FB type
	return structs.BlockBidAndTrace{
		Trace: &types.SignedBidTrace{
			Message:   &s.BellatrixMessage,
			Signature: s.BellatrixSignature,
		},
		Bid: &GetHeaderResponse{
			BellatrixVersion: types.VersionString("bellatrix"),
			BellatrixData:    signedBuilderBid,
		},
		Payload: &structs.GetPayloadResponse{
			Version: types.VersionString("bellatrix"),
			Data:    &s.BellatrixExecutionPayload,
		},
	}
}

func (s *SubmitBlockRequest) ToPayloadKey() structs.PayloadKey {
	return structs.PayloadKey{
		BlockHash: s.BellatrixMessage.BlockHash,
		Proposer:  s.BellatrixMessage.ProposerPubkey,
		Slot:      structs.Slot(s.BellatrixMessage.Slot),
	}
}

func (s *SubmitBlockRequest) PreparePayloadContents(sk *bls.SecretKey, pubkey *types.PublicKey, domain types.Domain) (cbs structs.CompleteBlockstruct, err error) {
	signedBuilderBid, err := s.toSignedBuilderBid(sk, pubkey, domain)
	if err != nil {
		return cbs, err
	}

	cbs.Payload = s.toBlockBidAndTrace(signedBuilderBid)
	cbs.Header = structs.HeaderAndTrace{
		Header: signedBuilderBid.BellatrixMessage.BellatrixHeader,
		Trace: &structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: types.BidTrace{
					Slot:                 s.Slot(),
					ParentHash:           cbs.Payload.Payload.Data.ParentHash(),
					BlockHash:            cbs.Payload.Payload.Data.BlockHash(),
					BuilderPubkey:        cbs.Payload.Trace.Message.BuilderPubkey,
					ProposerPubkey:       cbs.Payload.Trace.Message.ProposerPubkey,
					ProposerFeeRecipient: cbs.Payload.Trace.Message.ProposerFeeRecipient,
					Value:                s.Value(),
					GasLimit:             cbs.Payload.Trace.Message.GasLimit,
					GasUsed:              cbs.Payload.Trace.Message.GasUsed,
				},
				BlockNumber: cbs.Payload.Payload.Data.BlockNumber(),
				NumTx:       uint64(len(cbs.Payload.Payload.Data.Transactions())),
			},
			Timestamp:   uint64(time.Now().UnixMilli() / 1_000),
			TimestampMs: uint64(time.Now().UnixMilli()),
		},
	}
	return cbs, nil
}

func PayloadToPayloadHeader(p structs.ExecutionPayload) (*structs.ExecutionPayloadHeader, error) {
	if p == nil {
		return nil, types.ErrNilPayload
	}

	txs := [][]byte{}
	for _, tx := range p.Transactions() {
		txs = append(txs, tx)
	}

	transactions := types.Transactions{Transactions: txs}
	txroot, err := transactions.HashTreeRoot()
	if err != nil {
		return nil, err
	}

	return &structs.ExecutionPayloadHeader{
		ExecutionPayloadHeader: types.ExecutionPayloadHeader{
			ParentHash:       p.ParentHash(),
			FeeRecipient:     p.FeeRecipient(),
			StateRoot:        p.StateRoot(),
			ReceiptsRoot:     p.ReceiptsRoot(),
			LogsBloom:        p.LogsBloom(),
			Random:           p.Random(),
			BlockNumber:      p.BlockNumber(),
			GasLimit:         p.GasLimit(),
			GasUsed:          p.GasUsed(),
			Timestamp:        p.Timestamp(),
			ExtraData:        p.ExtraData(),
			BaseFeePerGas:    p.BaseFeePerGas(),
			BlockHash:        p.BlockHash(),
			TransactionsRoot: txroot,
		},
	}, nil
}

// BuilderBid https://github.com/ethereum/builder-specs/pull/2/files#diff-b37cbf48e8754483e30e7caaadc5defc8c3c6e1aaf3273ee188d787b7c75d993
type BuilderBid struct {
	BellatrixHeader *structs.ExecutionPayloadHeader `json:"header"`
	BellatrixValue  types.U256Str                   `json:"value" ssz-size:"32"`
	BellatrixPubkey types.PublicKey                 `json:"pubkey" ssz-size:"48"`
}

func (b *BuilderBid) Value() types.U256Str {
	return b.BellatrixValue
}

func (b *BuilderBid) Pubkey() types.PublicKey {
	return b.BellatrixPubkey
}

// HashTreeRoot ssz hashes the BuilderBid object
func (b *BuilderBid) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(b)
}

// HashTreeRootWith ssz hashes the BuilderBid object with a hasher
func (b *BuilderBid) HashTreeRootWith(hh ssz.HashWalker) (err error) {
	indx := hh.Index()

	// Field (0) 'Header'
	if err = b.BellatrixHeader.HashTreeRootWith(hh); err != nil {
		return
	}

	// Field (1) 'Value'
	hh.PutBytes(b.BellatrixValue[:])

	// Field (2) 'Pubkey'
	hh.PutBytes(b.BellatrixPubkey[:])

	hh.Merkleize(indx)
	return
}

// GetTree ssz hashes the BuilderBid object
func (b *BuilderBid) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(b)
}

// GetHeaderResponse is the response payload from the getHeader request: https://github.com/ethereum/builder-specs/pull/2/files#diff-c80f52e38c99b1049252a99215450a29fd248d709ffd834a9480c98a233bf32c
type GetHeaderResponse struct {
	BellatrixVersion types.VersionString `json:"version"`
	BellatrixData    *SignedBuilderBid   `json:"data"`
}

func (g *GetHeaderResponse) Version() types.VersionString {
	return g.BellatrixVersion

}
func (g *GetHeaderResponse) Data() structs.SignedBuilderBid {
	return g.BellatrixData
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

func (b *SignedBlindedBeaconBlock) ComputeSigningRoot(d types.Domain) ([32]byte, error) {
	return types.ComputeSigningRoot(b.SMessage, d)
}

func (s *SignedBlindedBeaconBlock) ToPayloadKey(pk types.PublicKey) structs.PayloadKey {
	return structs.PayloadKey{
		BlockHash: s.SMessage.Body.ExecutionPayloadHeader.BlockHash,
		Proposer:  pk,
		Slot:      structs.Slot(s.SMessage.Slot),
	}
}

func (s *SignedBlindedBeaconBlock) ToBeaconBlock(executionPayload structs.ExecutionPayload) (structs.SignedBeaconBlock, error) {
	ep, ok := executionPayload.(*ExecutionPayload)
	if !ok {
		return nil, errors.New("ExecutionPayload is not bellatrix")
	}

	block := &SignedBeaconBlock{
		BellatrixSignature: s.SSignature,
		BellatrixMessage: &BeaconBlock{
			Slot:          s.SMessage.Slot,
			ProposerIndex: s.SMessage.ProposerIndex,
			ParentRoot:    s.SMessage.ParentRoot,
			StateRoot:     s.SMessage.StateRoot,
			Body: &BeaconBlockBody{
				RandaoReveal:      s.SMessage.Body.RandaoReveal,
				Eth1Data:          s.SMessage.Body.Eth1Data,
				Graffiti:          s.SMessage.Body.Graffiti,
				ProposerSlashings: s.SMessage.Body.ProposerSlashings,
				AttesterSlashings: s.SMessage.Body.AttesterSlashings,
				Attestations:      s.SMessage.Body.Attestations,
				Deposits:          s.SMessage.Body.Deposits,
				VoluntaryExits:    s.SMessage.Body.VoluntaryExits,
				SyncAggregate:     s.SMessage.Body.SyncAggregate,
				ExecutionPayload:  ep,
			},
		},
	}

	if block.BellatrixMessage.Body.ProposerSlashings == nil {
		block.BellatrixMessage.Body.ProposerSlashings = []*types.ProposerSlashing{}
	}
	if block.BellatrixMessage.Body.AttesterSlashings == nil {
		block.BellatrixMessage.Body.AttesterSlashings = []*types.AttesterSlashing{}
	}
	if block.BellatrixMessage.Body.Attestations == nil {
		block.BellatrixMessage.Body.Attestations = []*types.Attestation{}
	}
	if block.BellatrixMessage.Body.Deposits == nil {
		block.BellatrixMessage.Body.Deposits = []*types.Deposit{}
	}

	if block.BellatrixMessage.Body.VoluntaryExits == nil {
		block.BellatrixMessage.Body.VoluntaryExits = []*types.SignedVoluntaryExit{}
	}

	if block.BellatrixMessage.Body.Eth1Data == nil {
		block.BellatrixMessage.Body.Eth1Data = &types.Eth1Data{}
	}

	if block.BellatrixMessage.Body.SyncAggregate == nil {
		block.BellatrixMessage.Body.SyncAggregate = &types.SyncAggregate{}
	}

	if block.BellatrixMessage.Body.ExecutionPayload == nil {
		block.BellatrixMessage.Body.ExecutionPayload = &ExecutionPayload{}
	}

	if block.BellatrixMessage.Body.ExecutionPayload.EpExtraData == nil {
		block.BellatrixMessage.Body.ExecutionPayload.EpExtraData = types.ExtraData{}
	}

	if block.BellatrixMessage.Body.ExecutionPayload.EpTransactions == nil {
		block.BellatrixMessage.Body.ExecutionPayload.EpTransactions = []hexutil.Bytes{}
	}

	return block, nil
}

type SignedBuilderBid struct {
	BellatrixMessage   *BuilderBid     `json:"message"`
	BellatrixSignature types.Signature `json:"signature" ssz-size:"96"`
}

/*
func (s *SignedBuilderBid) Message() structs.BuilderBid {
	return s.BellatrixMessage
}
*/

func (s *SignedBuilderBid) Value() types.U256Str {
	return s.BellatrixMessage.BellatrixValue
}

func (s *SignedBuilderBid) Signature() types.Signature {
	return s.BellatrixSignature
}

// SignedBeaconBlock https://github.com/ethereum/beacon-APIs/blob/master/types/bellatrix/block.yaml#L55
type SignedBeaconBlock struct {
	BellatrixMessage   *BeaconBlock    `json:"message"`
	BellatrixSignature types.Signature `json:"signature" ssz-size:"96"`
}

/*
func (s *SignedBeaconBlock) Message() structs.BeaconBlock {
	return s.BellatrixMessage
}*/

func (s *SignedBeaconBlock) Signature() types.Signature {
	return s.BellatrixSignature
}

// BeaconBlock https://github.com/ethereum/beacon-APIs/blob/master/types/bellatrix/block.yaml#L46
type BeaconBlock struct {
	Slot          uint64           `json:"slot,string"`
	ProposerIndex uint64           `json:"proposer_index,string"`
	ParentRoot    types.Root       `json:"parent_root" ssz-size:"32"`
	StateRoot     types.Root       `json:"state_root" ssz-size:"32"`
	Body          *BeaconBlockBody `json:"body"`
}

// BeaconBlockBody https://github.com/ethereum/beacon-APIs/blob/master/types/bellatrix/block.yaml#L38
type BeaconBlockBody struct {
	RandaoReveal      types.Signature              `json:"randao_reveal" ssz-size:"96"`
	Eth1Data          *types.Eth1Data              `json:"eth1_data"`
	Graffiti          types.Hash                   `json:"graffiti" ssz-size:"32"`
	ProposerSlashings []*types.ProposerSlashing    `json:"proposer_slashings" ssz-max:"16"`
	AttesterSlashings []*types.AttesterSlashing    `json:"attester_slashings" ssz-max:"2"`
	Attestations      []*types.Attestation         `json:"attestations" ssz-max:"128"`
	Deposits          []*types.Deposit             `json:"deposits" ssz-max:"16"`
	VoluntaryExits    []*types.SignedVoluntaryExit `json:"voluntary_exits" ssz-max:"16"`
	SyncAggregate     *types.SyncAggregate         `json:"sync_aggregate"`
	ExecutionPayload  *ExecutionPayload            `json:"execution_payload"`
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
