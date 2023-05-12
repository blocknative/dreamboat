package bellatrix

import (
	"errors"
	"time"

	"github.com/blocknative/dreamboat/structs"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ssz "github.com/ferranbt/fastssz"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
)

// BuilderSubmitBlockRequest spec: https://flashbots.notion.site/Relay-API-Spec-5fb0819366954962bc02e81cb33840f5#fa719683d4ae4a57bc3bf60e138b0dc6
type SubmitBlockRequest struct {
	BellatrixRaw              []byte           `json:"-"`
	BellatrixSignature        types.Signature  `json:"signature" ssz-size:"96"`
	BellatrixMessage          types.BidTrace   `json:"message"`
	BellatrixExecutionPayload ExecutionPayload `json:"execution_payload"`
}

func (b *SubmitBlockRequest) Validate() bool {
	return b.BellatrixMessage.Value.String() != "" &&
		b.BellatrixMessage.Slot != 0 &&
		b.BellatrixExecutionPayload.EpBlockNumber > 0 &&
		b.BellatrixExecutionPayload.EpTimestamp > 0
}

func (b *SubmitBlockRequest) Raw() []byte {
	return b.BellatrixRaw
}

func (b *SubmitBlockRequest) TraceBlockHash() types.Hash {
	return b.BellatrixMessage.BlockHash
}

func (b *SubmitBlockRequest) TraceParentHash() types.Hash {
	return b.BellatrixMessage.ParentHash
}

func (b *SubmitBlockRequest) Slot() uint64 {
	return b.BellatrixMessage.Slot
}

func (b *SubmitBlockRequest) BlockHash() types.Hash {
	return b.BellatrixExecutionPayload.EpBlockHash
}

func (b *SubmitBlockRequest) ParentHash() types.Hash {
	return b.BellatrixExecutionPayload.EpParentHash
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

func (b *SubmitBlockRequest) NumTx() uint64 {
	if b.BellatrixExecutionPayload.EpTransactions == nil {
		return 0
	}
	return uint64(len(b.BellatrixExecutionPayload.EpTransactions))
}

func (b *SubmitBlockRequest) Withdrawals() structs.Withdrawals {
	return nil
}

func (b *SubmitBlockRequest) ComputeSigningRoot(d types.Domain) ([32]byte, error) {
	return types.ComputeSigningRoot(&b.BellatrixMessage, d)
}

func (s *SubmitBlockRequest) toSignedBuilderBid(sk *bls.SecretKey, pubkey *types.PublicKey, domain types.Domain) (sbb SignedBuilderBid, err error) {
	header, err := PayloadToPayloadHeader(&s.BellatrixExecutionPayload)
	if err != nil {
		return sbb, err
	}

	builderBid := BuilderBid{
		BellatrixValue:  s.BellatrixMessage.Value,
		BellatrixHeader: header,
		BellatrixPubkey: *pubkey,
	}

	sig, err := types.SignMessage(&builderBid, domain, sk)
	if err != nil {
		return sbb, err
	}

	return SignedBuilderBid{
		BellatrixMessage:   &builderBid,
		BellatrixSignature: sig,
	}, nil
}

func (s *SubmitBlockRequest) toBlockBidAndTrace(signedBuilderBid SignedBuilderBid) (bbt structs.BlockBidAndTrace) {
	return &BlockBidAndTrace{
		Trace: &types.SignedBidTrace{
			Message:   &s.BellatrixMessage,
			Signature: s.BellatrixSignature,
		},
		Bid: GetHeaderResponse{
			BellatrixVersion: types.VersionString("bellatrix"),
			BellatrixData:    signedBuilderBid,
		},
		Payload: GetPayloadResponse{
			BellatrixVersion: types.VersionString("bellatrix"),
			BellatrixData:    s.BellatrixExecutionPayload,
		},
	}
}

type GetPayloadResponse struct {
	BellatrixVersion types.VersionString `json:"version"`
	BellatrixData    ExecutionPayload    `json:"data"`
}

func (s *GetPayloadResponse) Data() structs.ExecutionPayload {
	return &s.BellatrixData
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
		Trace: structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: types.BidTrace{
					Slot:                 s.Slot(),
					ParentHash:           s.BellatrixExecutionPayload.EpParentHash,
					BlockHash:            s.BellatrixExecutionPayload.EpBlockHash,
					BuilderPubkey:        s.BellatrixMessage.BuilderPubkey,
					ProposerPubkey:       s.BellatrixMessage.ProposerPubkey,
					ProposerFeeRecipient: s.BellatrixMessage.ProposerFeeRecipient,
					Value:                s.BellatrixMessage.Value,
					GasLimit:             s.BellatrixMessage.GasLimit,
					GasUsed:              s.BellatrixMessage.GasUsed,
				},
				BlockNumber: s.BellatrixExecutionPayload.EpBlockNumber,
				NumTx:       uint64(len(s.BellatrixExecutionPayload.EpTransactions)),
			},
			Timestamp:   uint64(time.Now().UnixMilli() / 1_000),
			TimestampMs: uint64(time.Now().UnixMilli()),
		},
	}
	return cbs, nil
}

func PayloadToPayloadHeader(p structs.ExecutionPayload) (*ExecutionPayloadHeader, error) {
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

	return &ExecutionPayloadHeader{
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
	BellatrixHeader *ExecutionPayloadHeader `json:"header"`
	BellatrixValue  types.U256Str           `json:"value" ssz-size:"32"`
	BellatrixPubkey types.PublicKey         `json:"pubkey" ssz-size:"48"`
}

func (b *BuilderBid) Value() types.U256Str {
	return b.BellatrixValue
}

func (b *BuilderBid) Pubkey() types.PublicKey {
	return b.BellatrixPubkey
}

func (b *BuilderBid) Header() structs.ExecutionPayloadHeader {
	return b.BellatrixHeader
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

type BuilderBidExtended struct {
	BuilderBid BuilderBid
	Proposer   types.PublicKey
	Slot       uint64
}

// GetHeaderResponse is the response payload from the getHeader request: https://github.com/ethereum/builder-specs/pull/2/files#diff-c80f52e38c99b1049252a99215450a29fd248d709ffd834a9480c98a233bf32c
type GetHeaderResponse struct {
	BellatrixVersion types.VersionString `json:"version"`
	BellatrixData    SignedBuilderBid    `json:"data"`
}

func (g *GetHeaderResponse) Version() types.VersionString {
	return g.BellatrixVersion

}
func (g *GetHeaderResponse) Data() structs.SignedBuilderBid {
	return &g.BellatrixData
}

type ExecutionPayloadHeader struct {
	types.ExecutionPayloadHeader
}

func (eph *ExecutionPayloadHeader) GetParentHash() types.Hash {
	return eph.ParentHash
}

func (eph *ExecutionPayloadHeader) GetBlockHash() types.Hash {
	return eph.BlockHash
}

func (eph *ExecutionPayloadHeader) GetBlockNumber() uint64 {
	return eph.BlockNumber
}

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
	return ep.EpGasUsed
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
	SRaw       []byte                   `json:"-"`
	SMessage   types.BlindedBeaconBlock `json:"message"`
	SSignature types.Signature          `json:"signature" ssz-size:"96"`
}

func (b *SignedBlindedBeaconBlock) Raw() []byte {
	return b.SRaw
}

func (b *SignedBlindedBeaconBlock) Loggable() map[string]any {
	logFields := map[string]any{
		"signature":     b.SSignature.String(),
		"slot":          b.SMessage.Slot,
		"proposerIndex": b.SMessage.ProposerIndex,
		"parentRoot":    b.SMessage.ParentRoot.String(),
		"stateRoot":     b.SMessage.StateRoot.String(),
	}
	if b.SMessage.Body != nil {
		if b.SMessage.Body.Eth1Data != nil {
			logFields["blockHash"] = b.SMessage.Body.Eth1Data.BlockHash.String()
			logFields["depositCount"] = b.SMessage.Body.Eth1Data.DepositCount
			logFields["depositRoot"] = b.SMessage.Body.Eth1Data.DepositRoot.String()
		}
		logFields["randaoReveal"] = b.SMessage.Body.RandaoReveal.String()
		logFields["graffiti"] = b.SMessage.Body.Graffiti.String()
		logFields["proposerSlashings"] = b.SMessage.Body.ProposerSlashings
		logFields["attesterSlashings"] = b.SMessage.Body.AttesterSlashings
		logFields["deposits"] = b.SMessage.Body.Deposits
		logFields["voluntaryExits"] = b.SMessage.Body.VoluntaryExits
		logFields["syncAggregate"] = b.SMessage.Body.SyncAggregate
		logFields["executionPayloadHeader"] = b.SMessage.Body.ExecutionPayloadHeader
	}

	return logFields
}

func (s *SignedBlindedBeaconBlock) Signature() types.Signature {
	return s.SSignature
}

func (s *SignedBlindedBeaconBlock) Slot() uint64 {
	return s.SMessage.Slot
}

func (s *SignedBlindedBeaconBlock) ExecutionHeaderHash() (types.Hash, error) {
	if s.SMessage.Body == nil || s.SMessage.Body.ExecutionPayloadHeader == nil {
		return [32]byte{}, nil
	}
	return s.SMessage.Body.ExecutionPayloadHeader.HashTreeRoot()
}

func (s *SignedBlindedBeaconBlock) BlockHash() types.Hash {
	if s.SMessage.Body == nil || s.SMessage.Body.ExecutionPayloadHeader == nil {
		return [32]byte{}
	}
	return s.SMessage.Body.ExecutionPayloadHeader.BlockHash
}

func (s *SignedBlindedBeaconBlock) BlockNumber() uint64 {
	if s.SMessage.Body == nil || s.SMessage.Body.ExecutionPayloadHeader == nil {
		return 0
	}
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

func (b *SignedBlindedBeaconBlock) Validate() bool {
	return b.SMessage.Body != nil && b.SMessage.Body.ExecutionPayloadHeader != nil
}

func (b *SignedBlindedBeaconBlock) ComputeSigningRoot(d types.Domain) ([32]byte, error) {
	if b.SMessage.Body == nil ||
		b.SMessage.Body.Eth1Data == nil ||
		b.SMessage.Body.SyncAggregate == nil ||
		b.SMessage.Body.ExecutionPayloadHeader == nil {
		return [32]byte{}, errors.New("empty block body")
	}
	return types.ComputeSigningRoot(&b.SMessage, d)
}

func (s *SignedBlindedBeaconBlock) ToPayloadKey(pk types.PublicKey) (payK structs.PayloadKey, err error) {
	if s.SMessage.Body == nil || s.SMessage.Body.ExecutionPayloadHeader == nil {
		return payK, errors.New("wrong payload key")
	}
	return structs.PayloadKey{
		BlockHash: s.BlockHash(),
		Proposer:  pk,
		Slot:      structs.Slot(s.SMessage.Slot),
	}, nil
}

func (s *SignedBlindedBeaconBlock) ToBeaconBlock(executionPayload structs.ExecutionPayload) (structs.SignedBeaconBlock, error) {
	ep, ok := executionPayload.(*ExecutionPayload)
	if !ok {
		return nil, errors.New("ExecutionPayload is not bellatrix")
	}
	body := s.SMessage.Body
	if body == nil {
		body = &types.BlindedBeaconBlockBody{}
	}

	block := &SignedBeaconBlock{
		BellatrixSignature: s.SSignature,
		BellatrixMessage: &BeaconBlock{
			Slot:          s.SMessage.Slot,
			ProposerIndex: s.SMessage.ProposerIndex,
			ParentRoot:    s.SMessage.ParentRoot,
			StateRoot:     s.SMessage.StateRoot,
			Body: &BeaconBlockBody{
				RandaoReveal:      body.RandaoReveal,
				Eth1Data:          body.Eth1Data,
				Graffiti:          body.Graffiti,
				ProposerSlashings: body.ProposerSlashings,
				AttesterSlashings: body.AttesterSlashings,
				Attestations:      body.Attestations,
				Deposits:          body.Deposits,
				VoluntaryExits:    body.VoluntaryExits,
				SyncAggregate:     body.SyncAggregate,
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

func (s *SignedBuilderBid) Value() types.U256Str {
	return s.BellatrixMessage.BellatrixValue
}

func (s *SignedBuilderBid) Signature() types.Signature {
	return s.BellatrixSignature
}

func (b *SignedBuilderBid) Validate() bool {
	return b.BellatrixMessage.BellatrixValue.String() != "" &&
		b.BellatrixMessage.BellatrixHeader != nil &&
		b.BellatrixMessage.BellatrixHeader.BlockNumber > 0
}

// SignedBeaconBlock https://github.com/ethereum/beacon-APIs/blob/master/types/bellatrix/block.yaml#L55
type SignedBeaconBlock struct {
	BellatrixMessage   *BeaconBlock    `json:"message"`
	BellatrixSignature types.Signature `json:"signature" ssz-size:"96"`
}

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

type BlockBidAndTrace struct {
	Trace   *types.SignedBidTrace
	Bid     GetHeaderResponse
	Payload GetPayloadResponse
}

func (bbat *BlockBidAndTrace) BidValue() types.U256Str {
	return bbat.Bid.BellatrixData.Value()
}

func (bbat *BlockBidAndTrace) Slot() uint64 {
	return bbat.Trace.Message.Slot
}
func (bbat *BlockBidAndTrace) Proposer() types.PublicKey {
	return bbat.Trace.Message.ProposerPubkey
}

func (bbat *BlockBidAndTrace) ExecutionPayload() structs.ExecutionPayload {
	return &bbat.Payload.BellatrixData
}

func (bbat *BlockBidAndTrace) ExecutionHeaderHash() (types.Hash, error) {
	if bbat.Bid.BellatrixData.BellatrixMessage == nil || bbat.Bid.BellatrixData.BellatrixMessage.BellatrixHeader == nil {
		return [32]byte{}, nil
	}
	return bbat.Bid.BellatrixData.BellatrixMessage.BellatrixHeader.HashTreeRoot()
}

func (bbat *BlockBidAndTrace) BuilderPubkey() (pub types.PublicKey) {
	if bbat.Trace == nil || bbat.Trace.Message == nil {
		return pub
	}
	return bbat.Trace.Message.BuilderPubkey
}

func (bbat *BlockBidAndTrace) ToDeliveredTrace(slot uint64) (dt structs.DeliveredTrace, err error) {
	if bbat.Trace == nil || bbat.Trace.Message == nil {
		return dt, errors.New("empty trace contents")
	}
	return structs.DeliveredTrace{
		Trace: structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: types.BidTrace{
					Slot:                 slot,
					ParentHash:           bbat.Payload.BellatrixData.EpParentHash,
					BlockHash:            bbat.Payload.BellatrixData.EpBlockHash,
					BuilderPubkey:        bbat.Trace.Message.BuilderPubkey,
					ProposerPubkey:       bbat.Trace.Message.ProposerPubkey,
					ProposerFeeRecipient: bbat.Trace.Message.ProposerFeeRecipient,
					GasLimit:             bbat.Payload.BellatrixData.EpGasLimit,
					GasUsed:              bbat.Payload.BellatrixData.EpGasUsed,
					Value:                bbat.Trace.Message.Value,
				},
				BlockNumber: bbat.Payload.BellatrixData.EpBlockNumber,
				NumTx:       uint64(len(bbat.Payload.BellatrixData.EpTransactions)),
			},
			Timestamp: bbat.Payload.BellatrixData.EpTimestamp,
		},
		BlockNumber: bbat.Payload.BellatrixData.EpBlockNumber,
	}, nil
}
