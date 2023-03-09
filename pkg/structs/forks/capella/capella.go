package capella

import (
	"errors"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/blocknative/dreamboat/pkg/structs/forks"
	"github.com/blocknative/dreamboat/pkg/structs/forks/bellatrix"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ssz "github.com/ferranbt/fastssz"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
)

type SubmitBlockRequest struct {
	CapellaMessage          types.BidTrace   `json:"message"`
	CapellaExecutionPayload ExecutionPayload `json:"execution_payload"`
	CapellaSignature        types.Signature  `json:"signature" ssz-size:"96"` //phase0.BLSSignature `ssz-size:"96"`
}

func (b *SubmitBlockRequest) Slot() uint64 {
	return b.CapellaMessage.Slot
}

func (b *SubmitBlockRequest) BlockHash() types.Hash {
	return b.CapellaExecutionPayload.EpBlockHash
}

func (b *SubmitBlockRequest) BuilderPubkey() types.PublicKey {
	return b.CapellaMessage.BuilderPubkey
}

func (b *SubmitBlockRequest) ProposerPubkey() types.PublicKey {
	return b.CapellaMessage.ProposerPubkey
}

func (b *SubmitBlockRequest) ProposerFeeRecipient() types.Address {
	return b.CapellaMessage.ProposerFeeRecipient
}

func (b *SubmitBlockRequest) Value() types.U256Str {
	return b.CapellaMessage.Value
}

func (b *SubmitBlockRequest) Signature() types.Signature {
	return b.CapellaSignature
}

func (b *SubmitBlockRequest) Timestamp() uint64 {
	return b.CapellaExecutionPayload.EpTimestamp
}

func (b *SubmitBlockRequest) Random() types.Hash {
	return b.CapellaExecutionPayload.EpRandom
}

func (b *SubmitBlockRequest) NumTx() uint64 {
	return uint64(len(b.CapellaExecutionPayload.EpTransactions))
}

func (b *SubmitBlockRequest) Withdrawals() structs.Withdrawals {
	return b.CapellaExecutionPayload.Withdrawals()
}

func (b *SubmitBlockRequest) ComputeSigningRoot(d types.Domain) ([32]byte, error) {
	return types.ComputeSigningRoot(&b.CapellaMessage, d)
}

func (s *SubmitBlockRequest) ToPayloadKey() structs.PayloadKey {
	return structs.PayloadKey{
		BlockHash: s.CapellaMessage.BlockHash,
		Proposer:  s.CapellaMessage.ProposerPubkey,
		Slot:      structs.Slot(s.CapellaMessage.Slot),
	}
}

func (s *SubmitBlockRequest) toBlockBidAndTrace(signedBuilderBid *SignedBuilderBid) (bbt structs.BlockBidAndTrace) { // TODO(l): remove FB type
	return &BlockBidAndTrace{
		Trace: &types.SignedBidTrace{
			Message:   &s.CapellaMessage,
			Signature: s.CapellaSignature,
		},
		Bid: GetHeaderResponse{
			CapellaVersion: types.VersionString("capella"),
			CapellaData:    signedBuilderBid,
		},
		Payload: GetPayloadResponse{
			CapellaVersion: types.VersionString("capella"),
			CapellaData:    s.CapellaExecutionPayload,
		},
	}
}

func (s *SubmitBlockRequest) PreparePayloadContents(sk *bls.SecretKey, pubkey *types.PublicKey, domain types.Domain) (cbs structs.CompleteBlockstruct, err error) {
	signedBuilderBid, err := s.toSignedBuilderBid(sk, pubkey, domain)
	if err != nil {
		return cbs, err
	}

	cbs.Payload = s.toBlockBidAndTrace(signedBuilderBid)

	cbs.Header = structs.HeaderAndTrace{
		Header: signedBuilderBid.CapellaMessage.CapellaHeader,
		Trace: &structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: types.BidTrace{
					Slot:                 s.Slot(),
					ParentHash:           s.CapellaExecutionPayload.EpParentHash,
					BlockHash:            s.CapellaExecutionPayload.EpBlockHash,
					BuilderPubkey:        s.CapellaMessage.BuilderPubkey,
					ProposerPubkey:       s.CapellaMessage.ProposerPubkey,
					ProposerFeeRecipient: s.CapellaMessage.ProposerFeeRecipient,
					Value:                s.Value(),
					GasLimit:             s.CapellaMessage.GasLimit,
					GasUsed:              s.CapellaMessage.GasUsed,
				},
				BlockNumber: s.CapellaExecutionPayload.EpBlockNumber,
				NumTx:       uint64(len(s.CapellaExecutionPayload.EpTransactions)),
			},
			Timestamp:   uint64(time.Now().UnixMilli() / 1_000),
			TimestampMs: uint64(time.Now().UnixMilli()),
		},
	}
	return cbs, nil
}

func (s *SubmitBlockRequest) toSignedBuilderBid(sk *bls.SecretKey, pubkey *types.PublicKey, domain types.Domain) (*SignedBuilderBid, error) {
	header, err := PayloadToPayloadHeader(&s.CapellaExecutionPayload)
	if err != nil {
		return nil, err
	}

	builderBid := BuilderBid{
		CapellaValue:  s.Value(),
		CapellaHeader: header,
		CapellaPubkey: *pubkey,
	}

	sig, err := types.SignMessage(&builderBid, domain, sk)
	if err != nil {
		return nil, err
	}

	return &SignedBuilderBid{
		CapellaMessage:   &builderBid,
		CapellaSignature: sig,
	}, nil
}

func PayloadToPayloadHeader(p *ExecutionPayload) (*ExecutionPayloadHeader, error) {
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

	var withdrawalsRoot [32]byte
	w := p.EpWithdrawals
	if w != nil {
		withdrawalsRoot, err = p.EpWithdrawals.HashTreeRoot()
		if err != nil {
			return nil, err
		}
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
		WithdrawalsRoot: withdrawalsRoot,
	}, nil
}

// BuilderBid https://github.com/ethereum/builder-specs/pull/2/files#diff-b37cbf48e8754483e30e7caaadc5defc8c3c6e1aaf3273ee188d787b7c75d993
type BuilderBid struct {
	CapellaHeader *ExecutionPayloadHeader `json:"header"`
	CapellaValue  types.U256Str           `json:"value" ssz-size:"32"`
	CapellaPubkey types.PublicKey         `json:"pubkey" ssz-size:"48"`
}

func (b *BuilderBid) Value() types.U256Str {
	return b.CapellaValue
}

func (b *BuilderBid) Pubkey() types.PublicKey {
	return b.CapellaPubkey
}

// HashTreeRoot ssz hashes the BuilderBid object
func (b *BuilderBid) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(b)
}

// HashTreeRootWith ssz hashes the BuilderBid object with a hasher
func (b *BuilderBid) HashTreeRootWith(hh ssz.HashWalker) (err error) {
	indx := hh.Index()

	// Field (0) 'Header'
	if err = b.CapellaHeader.HashTreeRootWith(hh); err != nil {
		return
	}

	// Field (1) 'Value'
	hh.PutBytes(b.CapellaValue[:])

	// Field (2) 'Pubkey'
	hh.PutBytes(b.CapellaPubkey[:])

	hh.Merkleize(indx)
	return
}

// GetTree ssz hashes the BuilderBid object
func (b *BuilderBid) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(b)
}

// ExecutionPayload represents an execution layer payload.
type ExecutionPayload struct {
	bellatrix.ExecutionPayload
	EpWithdrawals structs.Withdrawals `json:"withdrawals" ssz-max:"16"`
}

func (ep *ExecutionPayload) Withdrawals() structs.Withdrawals {
	return ep.EpWithdrawals
}

// GetHeaderResponse is the response payload from the getHeader request: https://github.com/ethereum/builder-specs/pull/2/files#diff-c80f52e38c99b1049252a99215450a29fd248d709ffd834a9480c98a233bf32c
type GetHeaderResponse struct {
	CapellaVersion types.VersionString `json:"version"`
	CapellaData    *SignedBuilderBid   `json:"data"`
}

func (g *GetHeaderResponse) Version() types.VersionString {
	return g.CapellaVersion

}
func (g *GetHeaderResponse) Data() structs.SignedBuilderBid {
	return g.CapellaData
}

type SignedBuilderBid struct {
	CapellaMessage   *BuilderBid     `json:"message"`
	CapellaSignature types.Signature `json:"signature" ssz-size:"96"`
}

// func (s *SignedBuilderBid) Message() structs.BuilderBid {
// 	return s.CapellaMessage
// }

func (s *SignedBuilderBid) Signature() types.Signature {
	return s.CapellaSignature
}

func (s *SignedBuilderBid) Value() types.U256Str {
	return s.CapellaMessage.CapellaValue
}

// SignedBlindedBeaconBlock https://github.com/ethereum/beacon-APIs/blob/master/types/bellatrix/block.yaml#L83
type SignedBlindedBeaconBlock struct {
	SMessage   BlindedBeaconBlock `json:"message"`
	SSignature types.Signature    `json:"signature" ssz-size:"96"`
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
	return types.ComputeSigningRoot(&b.SMessage, d)
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
		return nil, errors.New("ExecutionPayload is not Capella")
	}

	block := &SignedBeaconBlock{
		CapellaSignature: s.SSignature,
		CapellaMessage: &BeaconBlock{
			Slot:          s.SMessage.Slot,
			ProposerIndex: s.SMessage.ProposerIndex,
			ParentRoot:    s.SMessage.ParentRoot,
			StateRoot:     s.SMessage.StateRoot,
			Body: &BeaconBlockBody{
				BLSToExecutionChanges: s.SMessage.Body.BLSToExecutionChanges,
				RandaoReveal:          s.SMessage.Body.RandaoReveal,
				Eth1Data:              s.SMessage.Body.Eth1Data,
				Graffiti:              s.SMessage.Body.Graffiti,
				ProposerSlashings:     s.SMessage.Body.ProposerSlashings,
				AttesterSlashings:     s.SMessage.Body.AttesterSlashings,
				Attestations:          s.SMessage.Body.Attestations,
				Deposits:              s.SMessage.Body.Deposits,
				VoluntaryExits:        s.SMessage.Body.VoluntaryExits,
				SyncAggregate:         s.SMessage.Body.SyncAggregate,
				ExecutionPayload:      ep,
			},
		},
	}

	if block.CapellaMessage.Body.ProposerSlashings == nil {
		block.CapellaMessage.Body.ProposerSlashings = []*types.ProposerSlashing{}
	}
	if block.CapellaMessage.Body.AttesterSlashings == nil {
		block.CapellaMessage.Body.AttesterSlashings = []*types.AttesterSlashing{}
	}
	if block.CapellaMessage.Body.Attestations == nil {
		block.CapellaMessage.Body.Attestations = []*types.Attestation{}
	}
	if block.CapellaMessage.Body.Deposits == nil {
		block.CapellaMessage.Body.Deposits = []*types.Deposit{}
	}

	if block.CapellaMessage.Body.VoluntaryExits == nil {
		block.CapellaMessage.Body.VoluntaryExits = []*types.SignedVoluntaryExit{}
	}

	if block.CapellaMessage.Body.Eth1Data == nil {
		block.CapellaMessage.Body.Eth1Data = &types.Eth1Data{}
	}

	if block.CapellaMessage.Body.SyncAggregate == nil {
		block.CapellaMessage.Body.SyncAggregate = &types.SyncAggregate{}
	}

	if block.CapellaMessage.Body.ExecutionPayload == nil {
		block.CapellaMessage.Body.ExecutionPayload = &ExecutionPayload{}
	}

	if block.CapellaMessage.Body.ExecutionPayload.EpExtraData == nil {
		block.CapellaMessage.Body.ExecutionPayload.EpExtraData = types.ExtraData{}
	}

	if block.CapellaMessage.Body.ExecutionPayload.EpTransactions == nil {
		block.CapellaMessage.Body.ExecutionPayload.EpTransactions = []hexutil.Bytes{}
	}

	return block, nil
}

type ExecutionPayloadHeader struct {
	types.ExecutionPayloadHeader
	WithdrawalsRoot types.Root `json:"withdrawals_root,omitempty" ssz-size:"32"`
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

// SignedBeaconBlock https://github.com/ethereum/beacon-APIs/blob/master/types/bellatrix/block.yaml#L55
type SignedBeaconBlock struct {
	CapellaMessage   *BeaconBlock    `json:"message"`
	CapellaSignature types.Signature `json:"signature" ssz-size:"96"`
}

func (s *SignedBeaconBlock) Message() structs.BeaconBlock {
	return s.CapellaMessage
}

func (s *SignedBeaconBlock) Signature() types.Signature {
	return s.CapellaSignature
}

type BlockBidAndTrace struct {
	Trace   *types.SignedBidTrace
	Bid     GetHeaderResponse
	Payload GetPayloadResponse
}

func (bbat *BlockBidAndTrace) BidValue() types.U256Str {
	return bbat.Bid.CapellaData.Value()
}

func (bbat *BlockBidAndTrace) ExecutionPayload() structs.ExecutionPayload {
	return &bbat.Payload.CapellaData
}

func (bbat *BlockBidAndTrace) ToDeliveredTrace(slot uint64) structs.DeliveredTrace {
	return structs.DeliveredTrace{
		Trace: structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: types.BidTrace{
					Slot:                 slot,
					ParentHash:           bbat.Payload.CapellaData.EpParentHash,
					BlockHash:            bbat.Payload.CapellaData.EpBlockHash,
					BuilderPubkey:        bbat.Trace.Message.BuilderPubkey,
					ProposerPubkey:       bbat.Trace.Message.ProposerPubkey,
					ProposerFeeRecipient: bbat.Trace.Message.ProposerFeeRecipient,
					GasLimit:             bbat.Payload.CapellaData.EpGasLimit,
					GasUsed:              bbat.Payload.CapellaData.EpGasUsed,
					Value:                bbat.Trace.Message.Value,
				},
				BlockNumber: bbat.Payload.CapellaData.EpBlockNumber,
				NumTx:       uint64(len(bbat.Payload.CapellaData.EpTransactions)),
			},
			Timestamp: bbat.Payload.CapellaData.EpTimestamp,
		},
		BlockNumber: bbat.Payload.CapellaData.EpBlockNumber,
	}
}

type GetPayloadResponse struct {
	CapellaVersion types.VersionString `json:"version"`
	CapellaData    ExecutionPayload    `json:"data"`
}

func (s *GetPayloadResponse) Data() structs.ExecutionPayload {
	return &s.CapellaData
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
	BLSToExecutionChanges []*SignedBLSToExecutionChange `json:"bls_to_execution_changes" ssz-max:"16"`
	RandaoReveal          types.Signature               `json:"randao_reveal" ssz-size:"96"`
	Eth1Data              *types.Eth1Data               `json:"eth1_data"`
	Graffiti              types.Hash                    `json:"graffiti" ssz-size:"32"`
	ProposerSlashings     []*types.ProposerSlashing     `json:"proposer_slashings" ssz-max:"16"`
	AttesterSlashings     []*types.AttesterSlashing     `json:"attester_slashings" ssz-max:"2"`
	Attestations          []*types.Attestation          `json:"attestations" ssz-max:"128"`
	Deposits              []*types.Deposit              `json:"deposits" ssz-max:"16"`
	VoluntaryExits        []*types.SignedVoluntaryExit  `json:"voluntary_exits" ssz-max:"16"`
	SyncAggregate         *types.SyncAggregate          `json:"sync_aggregate"`
	ExecutionPayload      *ExecutionPayload             `json:"execution_payload"`
}

// BlindedBeaconBlock https://github.com/ethereum/beacon-APIs/blob/master/types/bellatrix/block.yaml#L74
type BlindedBeaconBlock struct {
	Slot          uint64                  `json:"slot,string"`
	ProposerIndex uint64                  `json:"proposer_index,string"`
	ParentRoot    types.Root              `json:"parent_root" ssz-size:"32"`
	StateRoot     types.Root              `json:"state_root" ssz-size:"32"`
	Body          *BlindedBeaconBlockBody `json:"body"`
}

// HashTreeRoot ssz hashes the BlindedBeaconBlock object
func (b *BlindedBeaconBlock) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(b)
}

// HashTreeRootWith ssz hashes the BlindedBeaconBlock object with a hasher
func (b *BlindedBeaconBlock) HashTreeRootWith(hh ssz.HashWalker) (err error) {
	indx := hh.Index()

	// Field (0) 'Slot'
	hh.PutUint64(b.Slot)

	// Field (1) 'ProposerIndex'
	hh.PutUint64(b.ProposerIndex)

	// Field (2) 'ParentRoot'
	hh.PutBytes(b.ParentRoot[:])

	// Field (3) 'StateRoot'
	hh.PutBytes(b.StateRoot[:])

	// Field (4) 'Body'
	if err = b.Body.HashTreeRootWith(hh); err != nil {
		return
	}

	hh.Merkleize(indx)
	return
}

// GetTree ssz hashes the BlindedBeaconBlock object
func (b *BlindedBeaconBlock) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(b)
}

type BlindedBeaconBlockBody struct {
	forks.BlindedBeaconBlockBody

	ExecutionPayloadHeader *ExecutionPayloadHeader       `json:"execution_payload_header"`
	BLSToExecutionChanges  []*SignedBLSToExecutionChange `json:"bls_to_execution_changes" ssz-max:"16"`
}

// SignedBLSToExecutionChange provides information about a signed BLS to execution change.
type SignedBLSToExecutionChange struct {
	Message   *BLSToExecutionChange `json:"message"`
	Signature types.Signature       `json:"signature" ssz-size:"96"`
}

// BLSToExecutionChange provides information about a change of withdrawal credentials.
type BLSToExecutionChange struct {
	ValidatorIndex     uint64          `json:"validator_index,string"`
	FromBLSPubkey      types.PublicKey `json:"from_bls_pubkey" ssz-size:"48"`
	ToExecutionAddress types.Address   `json:"to_execution_address" ssz-size:"20"`
}

// HashTreeRoot ssz hashes the BlindedBeaconBlockBody object
func (b *BlindedBeaconBlockBody) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(b)
}

// HashTreeRootWith ssz hashes the BlindedBeaconBlockBody object with a hasher
func (b *BlindedBeaconBlockBody) HashTreeRootWith(hh ssz.HashWalker) (err error) {
	indx := hh.Index()

	// Field (0) 'RandaoReveal'
	hh.PutBytes(b.RandaoReveal[:])

	// Field (1) 'Eth1Data'
	if err = b.Eth1Data.HashTreeRootWith(hh); err != nil {
		return
	}

	// Field (2) 'Graffiti'
	hh.PutBytes(b.Graffiti[:])

	// Field (3) 'ProposerSlashings'
	{
		subIndx := hh.Index()
		num := uint64(len(b.ProposerSlashings))
		if num > 16 {
			err = ssz.ErrIncorrectListSize
			return
		}
		for _, elem := range b.ProposerSlashings {
			if err = elem.HashTreeRootWith(hh); err != nil {
				return
			}
		}
		hh.MerkleizeWithMixin(subIndx, num, 16)
	}

	// Field (4) 'AttesterSlashings'
	{
		subIndx := hh.Index()
		num := uint64(len(b.AttesterSlashings))
		if num > 2 {
			err = ssz.ErrIncorrectListSize
			return
		}
		for _, elem := range b.AttesterSlashings {
			if err = elem.HashTreeRootWith(hh); err != nil {
				return
			}
		}
		hh.MerkleizeWithMixin(subIndx, num, 2)
	}

	// Field (5) 'Attestations'
	{
		subIndx := hh.Index()
		num := uint64(len(b.Attestations))
		if num > 128 {
			err = ssz.ErrIncorrectListSize
			return
		}
		for _, elem := range b.Attestations {
			if err = elem.HashTreeRootWith(hh); err != nil {
				return
			}
		}
		hh.MerkleizeWithMixin(subIndx, num, 128)
	}

	// Field (6) 'Deposits'
	{
		subIndx := hh.Index()
		num := uint64(len(b.Deposits))
		if num > 16 {
			err = ssz.ErrIncorrectListSize
			return
		}
		for _, elem := range b.Deposits {
			if err = elem.HashTreeRootWith(hh); err != nil {
				return
			}
		}
		hh.MerkleizeWithMixin(subIndx, num, 16)
	}

	// Field (7) 'VoluntaryExits'
	{
		subIndx := hh.Index()
		num := uint64(len(b.VoluntaryExits))
		if num > 16 {
			err = ssz.ErrIncorrectListSize
			return
		}
		for _, elem := range b.VoluntaryExits {
			if err = elem.HashTreeRootWith(hh); err != nil {
				return
			}
		}
		hh.MerkleizeWithMixin(subIndx, num, 16)
	}

	// Field (8) 'SyncAggregate'
	if err = b.SyncAggregate.HashTreeRootWith(hh); err != nil {
		return
	}

	// Field (9) 'ExecutionPayloadHeader'
	if err = b.ExecutionPayloadHeader.HashTreeRootWith(hh); err != nil {
		return
	}

	hh.Merkleize(indx)
	return
}

// GetTree ssz hashes the BlindedBeaconBlockBody object
func (b *BlindedBeaconBlockBody) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(b)
}
