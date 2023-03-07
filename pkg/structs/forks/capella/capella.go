package capella

import (
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/blocknative/dreamboat/pkg/structs/forks"
	"github.com/blocknative/dreamboat/pkg/structs/forks/bellatrix"
	ssz "github.com/ferranbt/fastssz"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
)

type SubmitBlockRequest struct {
	CapellaMessage          types.BidTrace
	CapellaExecutionPayload ExecutionPayload
	CapellaSignature        types.Signature `ssz-size:"96"` //phase0.BLSSignature `ssz-size:"96"`
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

func (s *SubmitBlockRequest) toBlockBidAndTrace(sk *bls.SecretKey, pubkey *types.PublicKey, domain types.Domain) (bbt structs.BlockBidAndTrace, err error) { // TODO(l): remove FB type
	signedBuilderBid, err := s.toSignedBuilderBid(sk, pubkey, domain)
	if err != nil {
		return bbt, err
	}

	return structs.BlockBidAndTrace{
		Trace: &types.SignedBidTrace{
			Message:   &s.CapellaMessage,
			Signature: s.CapellaSignature,
		},
		Bid: &GetHeaderResponse{
			CapellaVersion: types.VersionString("capella"),
			CapellaData:    signedBuilderBid,
		},
		Payload: &structs.GetPayloadResponse{
			Version: types.VersionString("capella"),
			Data:    &s.CapellaExecutionPayload,
		},
	}, nil
}

func (s *SubmitBlockRequest) PreparePayloadContents(sk *bls.SecretKey, pubkey *types.PublicKey, domain types.Domain) (cbs structs.CompleteBlockstruct, err error) {

	cbs.Payload, err = s.toBlockBidAndTrace(sk, pubkey, domain)
	if err != nil {
		return cbs, err
	}
	cbs.Header = structs.HeaderAndTrace{
		Header: cbs.Payload.Bid.Data().Message().Header(),
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

func PayloadToPayloadHeader(p *ExecutionPayload) (*structs.ExecutionPayloadHeader, error) {
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
	w := p.EpWithdrawals.Withdrawals
	if w != nil {
		withdrawalsRoot, err = p.EpWithdrawals.HashTreeRoot()
		if err != nil {
			return nil, err
		}
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
		WithdrawalsRoot: withdrawalsRoot,
	}, nil
}

// BuilderBid https://github.com/ethereum/builder-specs/pull/2/files#diff-b37cbf48e8754483e30e7caaadc5defc8c3c6e1aaf3273ee188d787b7c75d993
type BuilderBid struct {
	CapellaHeader *structs.ExecutionPayloadHeader `json:"header"`
	CapellaValue  types.U256Str                   `json:"value" ssz-size:"32"`
	CapellaPubkey types.PublicKey                 `json:"pubkey" ssz-size:"48"`
}

func (b *BuilderBid) Header() *structs.ExecutionPayloadHeader {
	return b.CapellaHeader
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
	EpWithdrawals Withdrawals `json:"withdrawals" ssz-max:"16"`
}

/*
func (ep *ExecutionPayload) Withdrawals() structs.Withdrawals {
	return ep.EpWithdrawals
}*/

// Withdrawal provides information about a withdrawal.
type Withdrawals struct {
	Withdrawals []*Withdrawal
}

// HashTreeRoot ssz hashes the Withdrawals object
func (w *Withdrawals) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(w)
}

// HashTreeRootWith ssz hashes the Withdrawals object with a hasher
func (w *Withdrawals) HashTreeRootWith(hh ssz.HashWalker) (err error) {
	indx := hh.Index()

	// Field (0) 'Withdrawals'
	{
		subIndx := hh.Index()
		num := uint64(len(w.Withdrawals))
		if num > 16 {
			err = ssz.ErrIncorrectListSize
			return
		}
		for _, elem := range w.Withdrawals {
			if err = elem.HashTreeRootWith(hh); err != nil {
				return
			}
		}
		hh.MerkleizeWithMixin(subIndx, num, 16)
	}

	hh.Merkleize(indx)
	return
}

// GetTree ssz hashes the Withdrawals object
func (w *Withdrawals) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(w)
}

// Withdrawal provides information about a withdrawal.
type Withdrawal struct {
	Index          uint64        `json:"index,string"`
	ValidatorIndex uint64        `json:"validator_index,string"`
	Address        types.Address `json:"address" ssz-size:"20"`
	Amount         uint64        `json:"amount,string"`
}

// HashTreeRoot ssz hashes the Withdrawal object
func (w *Withdrawal) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(w)
}

// HashTreeRootWith ssz hashes the Withdrawal object with a hasher
func (w *Withdrawal) HashTreeRootWith(hh ssz.HashWalker) (err error) {
	indx := hh.Index()

	// Field (0) 'Index'
	hh.PutUint64(uint64(w.Index))

	// Field (1) 'ValidatorIndex'
	hh.PutUint64(uint64(w.ValidatorIndex))

	// Field (2) 'Address'
	hh.PutBytes(w.Address[:])

	// Field (3) 'Amount'
	hh.PutUint64(uint64(w.Amount))

	hh.Merkleize(indx)
	return
}

// GetTree ssz hashes the Withdrawal object
func (w *Withdrawal) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(w)
}

type BlindedBeaconBlockBody struct {
	forks.BlindedBeaconBlockBody

	ExecutionPayloadHeader *structs.ExecutionPayloadHeader `json:"execution_payload_header"`
	BLSToExecutionChanges  []*SignedBLSToExecutionChange   `json:"bls_to_execution_changes" ssz-max:"16"`
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

/*
// BlindedBeaconBlockBody represents the body of a blinded beacon block.
type BlindedBeaconBlockBody struct {
	RANDAOReveal           phase0.BLSSignature `ssz-size:"96"`
	ETH1Data               *phase0.ETH1Data
	Graffiti               [32]byte                      `ssz-size:"32"`
	ProposerSlashings      []*phase0.ProposerSlashing    `ssz-max:"16"`
	AttesterSlashings      []*phase0.AttesterSlashing    `ssz-max:"2"`
	Attestations           []*phase0.Attestation         `ssz-max:"128"`
	Deposits               []*phase0.Deposit             `ssz-max:"16"`
	VoluntaryExits         []*phase0.SignedVoluntaryExit `ssz-max:"16"`
	SyncAggregate          *altair.SyncAggregate
	ExecutionPayloadHeader *capella.ExecutionPayloadHeader
	BLSToExecutionChanges  []*capella.SignedBLSToExecutionChange `ssz-max:"16"`
}
*/

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

func (s *SignedBuilderBid) Message() structs.BuilderBid {
	return s.CapellaMessage
}

func (s *SignedBuilderBid) Signature() types.Signature {
	return s.CapellaSignature
}
