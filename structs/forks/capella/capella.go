package capella

import (
	"errors"
	"time"

	"github.com/blocknative/dreamboat/structs"
	"github.com/blocknative/dreamboat/structs/forks"
	"github.com/blocknative/dreamboat/structs/forks/bellatrix"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ssz "github.com/ferranbt/fastssz"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
)

type SubmitBlockRequest struct {
	CapellaRaw              []byte           `json:"-"`
	CapellaMessage          types.BidTrace   `json:"message"`
	CapellaExecutionPayload ExecutionPayload `json:"execution_payload"`
	CapellaSignature        types.Signature  `json:"signature" ssz-size:"96"`
}

func (b *SubmitBlockRequest) Validate() bool {
	return b.CapellaMessage.Value.String() != "" &&
		b.CapellaMessage.Slot != 0 &&
		b.CapellaExecutionPayload.EpBlockNumber > 0 &&
		b.CapellaExecutionPayload.EpTimestamp > 0
}

func (b *SubmitBlockRequest) Raw() []byte {
	return b.CapellaRaw
}

func (b *SubmitBlockRequest) TraceBlockHash() types.Hash {
	return b.CapellaMessage.BlockHash
}

func (b *SubmitBlockRequest) TraceParentHash() types.Hash {
	return b.CapellaMessage.ParentHash
}

func (b *SubmitBlockRequest) Slot() uint64 {
	return b.CapellaMessage.Slot
}

func (b *SubmitBlockRequest) BlockHash() types.Hash {
	return b.CapellaExecutionPayload.EpBlockHash
}

func (b *SubmitBlockRequest) ParentHash() types.Hash {
	return b.CapellaExecutionPayload.EpParentHash
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
	if b.CapellaExecutionPayload.EpTransactions == nil {
		return 0
	}
	return uint64(len(b.CapellaExecutionPayload.EpTransactions))
}

func (b *SubmitBlockRequest) Withdrawals() structs.Withdrawals {
	return b.CapellaExecutionPayload.EpWithdrawals
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

func (s *SubmitBlockRequest) toBlockBidAndTrace(signedBuilderBid SignedBuilderBid) (bbt structs.BlockBidAndTrace) {
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
		Trace: structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: types.BidTrace{
					Slot:                 s.Slot(),
					ParentHash:           s.CapellaExecutionPayload.EpParentHash,
					BlockHash:            s.CapellaExecutionPayload.EpBlockHash,
					BuilderPubkey:        s.CapellaMessage.BuilderPubkey,
					ProposerPubkey:       s.CapellaMessage.ProposerPubkey,
					ProposerFeeRecipient: s.CapellaMessage.ProposerFeeRecipient,
					Value:                s.CapellaMessage.Value,
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

func (s *SubmitBlockRequest) toSignedBuilderBid(sk *bls.SecretKey, pubkey *types.PublicKey, domain types.Domain) (sbb SignedBuilderBid, err error) {
	header, err := PayloadToPayloadHeader(&s.CapellaExecutionPayload)
	if err != nil {
		return sbb, err
	}

	builderBid := BuilderBid{
		CapellaValue:  s.CapellaMessage.Value,
		CapellaHeader: header,
		CapellaPubkey: *pubkey,
	}

	sig, err := types.SignMessage(&builderBid, domain, sk)
	if err != nil {
		return sbb, err
	}

	return SignedBuilderBid{
		CapellaMessage:   builderBid,
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
		hW := structs.HashWithdrawals{Withdrawals: p.EpWithdrawals}
		withdrawalsRoot, err = hW.HashTreeRoot()
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

	if b.CapellaHeader == nil {
		return errors.New("empty bid header")
	}
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

// GetHeaderResponse is the response payload from the getHeader request: https://github.com/ethereum/builder-specs/pull/2/files#diff-c80f52e38c99b1049252a99215450a29fd248d709ffd834a9480c98a233bf32c
type GetHeaderResponse struct {
	CapellaVersion types.VersionString `json:"version"`
	CapellaData    SignedBuilderBid    `json:"data"`
}

func (g *GetHeaderResponse) Version() types.VersionString {
	return g.CapellaVersion

}
func (g *GetHeaderResponse) Data() structs.SignedBuilderBid {
	return &g.CapellaData
}

type SignedBuilderBid struct {
	CapellaMessage   BuilderBid      `json:"message"`
	CapellaSignature types.Signature `json:"signature" ssz-size:"96"`
}

func (b *SignedBuilderBid) Validate() bool {
	return b.CapellaMessage.CapellaValue.String() != "" &&
		b.CapellaMessage.CapellaHeader != nil &&
		b.CapellaMessage.CapellaHeader.BlockNumber > 0
}

func (s *SignedBuilderBid) Signature() types.Signature {
	return s.CapellaSignature
}

func (s *SignedBuilderBid) Value() types.U256Str {
	return s.CapellaMessage.CapellaValue
}

// HashTreeRoot ssz hashes the SignedBuilderBid object
func (s *SignedBuilderBid) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(s)
}

// HashTreeRootWith ssz hashes the SignedBuilderBid object with a hasher
func (s *SignedBuilderBid) HashTreeRootWith(hh ssz.HashWalker) (err error) {
	indx := hh.Index()

	// Field (0) 'Message'
	if err = s.CapellaMessage.HashTreeRootWith(hh); err != nil {
		return
	}

	// Field (1) 'Signature'
	hh.PutBytes(s.CapellaSignature[:])

	hh.Merkleize(indx)
	return
}

// GetTree ssz hashes the SignedBuilderBid object
func (s *SignedBuilderBid) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(s)
}

// SignedBlindedBeaconBlock https://github.com/ethereum/beacon-APIs/blob/master/types/bellatrix/block.yaml#L83
type SignedBlindedBeaconBlock struct {
	SRaw       []byte             `json:"-"`
	SMessage   BlindedBeaconBlock `json:"message"`
	SSignature types.Signature    `json:"signature" ssz-size:"96"`
}

func (b *SignedBlindedBeaconBlock) Validate() bool {
	return b.SMessage.Body != nil && b.SMessage.Body.ExecutionPayloadHeader != nil
}

func (b *SignedBlindedBeaconBlock) Raw() []byte {
	return b.SRaw
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
		BlockHash: s.SMessage.Body.ExecutionPayloadHeader.BlockHash,
		Proposer:  pk,
		Slot:      structs.Slot(s.SMessage.Slot),
	}, nil
}

func (s *SignedBlindedBeaconBlock) ToBeaconBlock(executionPayload structs.ExecutionPayload) (structs.SignedBeaconBlock, error) {
	ep, ok := executionPayload.(*ExecutionPayload)
	if !ok {
		return nil, errors.New("ExecutionPayload is not Capella")
	}

	body := s.SMessage.Body
	if body == nil {
		body = &BlindedBeaconBlockBody{}
	}

	block := &SignedBeaconBlock{
		CapellaSignature: s.SSignature,
		CapellaMessage: &BeaconBlock{
			Slot:          s.SMessage.Slot,
			ProposerIndex: s.SMessage.ProposerIndex,
			ParentRoot:    s.SMessage.ParentRoot,
			StateRoot:     s.SMessage.StateRoot,
			Body: &BeaconBlockBody{
				BLSToExecutionChanges: body.BLSToExecutionChanges,
				RandaoReveal:          body.RandaoReveal,
				Eth1Data:              body.Eth1Data,
				Graffiti:              body.Graffiti,
				ProposerSlashings:     body.ProposerSlashings,
				AttesterSlashings:     body.AttesterSlashings,
				Attestations:          body.Attestations,
				Deposits:              body.Deposits,
				VoluntaryExits:        body.VoluntaryExits,
				SyncAggregate:         body.SyncAggregate,
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

// MarshalSSZ ssz marshals the ExecutionPayloadHeader object
func (e *ExecutionPayloadHeader) MarshalSSZ() ([]byte, error) {
	return ssz.MarshalSSZ(e)
}

// MarshalSSZTo ssz marshals the ExecutionPayloadHeader object to a target array
func (e *ExecutionPayloadHeader) MarshalSSZTo(buf []byte) (dst []byte, err error) {
	dst = buf
	offset := int(568)

	// Field (0) 'ParentHash'
	dst = append(dst, e.ParentHash[:]...)

	// Field (1) 'FeeRecipient'
	dst = append(dst, e.FeeRecipient[:]...)

	// Field (2) 'StateRoot'
	dst = append(dst, e.StateRoot[:]...)

	// Field (3) 'ReceiptsRoot'
	dst = append(dst, e.ReceiptsRoot[:]...)

	// Field (4) 'LogsBloom'
	dst = append(dst, e.LogsBloom[:]...)

	// Field (5) 'PrevRandao'
	dst = append(dst, e.Random[:]...)

	// Field (6) 'BlockNumber'
	dst = ssz.MarshalUint64(dst, e.BlockNumber)

	// Field (7) 'GasLimit'
	dst = ssz.MarshalUint64(dst, e.GasLimit)

	// Field (8) 'GasUsed'
	dst = ssz.MarshalUint64(dst, e.GasUsed)

	// Field (9) 'Timestamp'
	dst = ssz.MarshalUint64(dst, e.Timestamp)

	// Offset (10) 'ExtraData'
	dst = ssz.WriteOffset(dst, offset)
	offset += len(e.ExtraData)

	// Field (11) 'BaseFeePerGas'
	dst = append(dst, e.BaseFeePerGas[:]...)

	// Field (12) 'BlockHash'
	dst = append(dst, e.BlockHash[:]...)

	// Field (13) 'TransactionsRoot'
	dst = append(dst, e.TransactionsRoot[:]...)

	// Field (14) 'WithdrawalsRoot'
	dst = append(dst, e.WithdrawalsRoot[:]...)

	// Field (10) 'ExtraData'
	if size := len(e.ExtraData); size > 32 {
		err = ssz.ErrBytesLengthFn("ExecutionPayloadHeader.ExtraData", size, 32)
		return
	}
	dst = append(dst, e.ExtraData...)

	return
}

// UnmarshalSSZ ssz unmarshals the ExecutionPayloadHeader object
func (e *ExecutionPayloadHeader) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 568 {
		return ssz.ErrSize
	}

	tail := buf
	var o10 uint64

	// Field (0) 'ParentHash'
	copy(e.ParentHash[:], buf[0:32])

	// Field (1) 'FeeRecipient'
	copy(e.FeeRecipient[:], buf[32:52])

	// Field (2) 'StateRoot'
	copy(e.StateRoot[:], buf[52:84])

	// Field (3) 'ReceiptsRoot'
	copy(e.ReceiptsRoot[:], buf[84:116])

	// Field (4) 'LogsBloom'
	copy(e.LogsBloom[:], buf[116:372])

	// Field (5) 'PrevRandao'
	copy(e.Random[:], buf[372:404])

	// Field (6) 'BlockNumber'
	e.BlockNumber = ssz.UnmarshallUint64(buf[404:412])

	// Field (7) 'GasLimit'
	e.GasLimit = ssz.UnmarshallUint64(buf[412:420])

	// Field (8) 'GasUsed'
	e.GasUsed = ssz.UnmarshallUint64(buf[420:428])

	// Field (9) 'Timestamp'
	e.Timestamp = ssz.UnmarshallUint64(buf[428:436])

	// Offset (10) 'ExtraData'
	if o10 = ssz.ReadOffset(buf[436:440]); o10 > size {
		return ssz.ErrOffset
	}

	if o10 < 568 {
		return ssz.ErrInvalidVariableOffset
	}

	// Field (11) 'BaseFeePerGas'
	copy(e.BaseFeePerGas[:], buf[440:472])

	// Field (12) 'BlockHash'
	copy(e.BlockHash[:], buf[472:504])

	// Field (13) 'TransactionsRoot'
	copy(e.TransactionsRoot[:], buf[504:536])

	// Field (14) 'WithdrawalsRoot'
	copy(e.WithdrawalsRoot[:], buf[536:568])

	// Field (10) 'ExtraData'
	{
		buf = tail[o10:]
		if len(buf) > 32 {
			return ssz.ErrBytesLength
		}
		if cap(e.ExtraData) == 0 {
			e.ExtraData = make([]byte, 0, len(buf))
		}
		e.ExtraData = append(e.ExtraData, buf...)
	}
	return err
}

// SizeSSZ returns the ssz encoded size in bytes for the ExecutionPayloadHeader object
func (e *ExecutionPayloadHeader) SizeSSZ() (size int) {
	size = 568

	// Field (10) 'ExtraData'
	size += len(e.ExtraData)

	return
}

// HashTreeRoot ssz hashes the ExecutionPayloadHeader object
func (e *ExecutionPayloadHeader) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(e)
}

// HashTreeRootWith ssz hashes the ExecutionPayloadHeader object with a hasher
func (e *ExecutionPayloadHeader) HashTreeRootWith(hh ssz.HashWalker) (err error) {
	indx := hh.Index()

	// Field (0) 'ParentHash'
	hh.PutBytes(e.ParentHash[:])

	// Field (1) 'FeeRecipient'
	hh.PutBytes(e.FeeRecipient[:])

	// Field (2) 'StateRoot'
	hh.PutBytes(e.StateRoot[:])

	// Field (3) 'ReceiptsRoot'
	hh.PutBytes(e.ReceiptsRoot[:])

	// Field (4) 'LogsBloom'
	hh.PutBytes(e.LogsBloom[:])

	// Field (5) 'PrevRandao'
	hh.PutBytes(e.Random[:])

	// Field (6) 'BlockNumber'
	hh.PutUint64(e.BlockNumber)

	// Field (7) 'GasLimit'
	hh.PutUint64(e.GasLimit)

	// Field (8) 'GasUsed'
	hh.PutUint64(e.GasUsed)

	// Field (9) 'Timestamp'
	hh.PutUint64(e.Timestamp)

	// Field (10) 'ExtraData'
	{
		elemIndx := hh.Index()
		byteLen := uint64(len(e.ExtraData))
		if byteLen > 32 {
			err = ssz.ErrIncorrectListSize
			return
		}
		hh.PutBytes(e.ExtraData)
		hh.MerkleizeWithMixin(elemIndx, byteLen, (32+31)/32)
	}

	// Field (11) 'BaseFeePerGas'
	hh.PutBytes(e.BaseFeePerGas[:])

	// Field (12) 'BlockHash'
	hh.PutBytes(e.BlockHash[:])

	// Field (13) 'TransactionsRoot'
	hh.PutBytes(e.TransactionsRoot[:])

	// Field (14) 'WithdrawalsRoot'
	hh.PutBytes(e.WithdrawalsRoot[:])

	hh.Merkleize(indx)
	return
}

// GetTree ssz hashes the ExecutionPayloadHeader object
func (e *ExecutionPayloadHeader) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(e)
}

// SignedBeaconBlock https://github.com/ethereum/beacon-APIs/blob/master/types/bellatrix/block.yaml#L55
type SignedBeaconBlock struct {
	CapellaMessage   *BeaconBlock    `json:"message"`
	CapellaSignature types.Signature `json:"signature" ssz-size:"96"`
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
	return bbat.Bid.CapellaData.CapellaMessage.CapellaValue
}

func (bbat *BlockBidAndTrace) ExecutionPayload() structs.ExecutionPayload {
	return &bbat.Payload.CapellaData
}

func (bbat *BlockBidAndTrace) ExecutionHeaderHash() (types.Hash, error) {
	if bbat.Bid.CapellaData.CapellaMessage.CapellaHeader == nil {
		return [32]byte{}, nil
	}
	return bbat.Bid.CapellaData.CapellaMessage.CapellaHeader.HashTreeRoot()
}

func (bbat *BlockBidAndTrace) BuilderPubkey() (pub types.PublicKey) {
	if bbat.Trace == nil || bbat.Trace.Message == nil {
		return pub
	}
	return bbat.Trace.Message.BuilderPubkey
}

func (bbat *BlockBidAndTrace) ToDeliveredTrace(slot uint64) (dt structs.DeliveredTrace, err error) {
	if bbat.Trace == nil || bbat.Trace.Message == nil {
		return dt, errors.New("empty message")
	}
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
	}, nil
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

// HashTreeRoot ssz hashes the BlindedBeaconBlockBody object
func (b *BlindedBeaconBlockBody) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(b)
}

// HashTreeRootWith ssz hashes the BlindedBeaconBlockBody object with a hasher
func (b *BlindedBeaconBlockBody) HashTreeRootWith(hh ssz.HashWalker) (err error) {
	indx := hh.Index()

	// Field (0) 'RANDAOReveal'
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

	// Field (10) 'BLSToExecutionChanges'
	{
		subIndx := hh.Index()
		num := uint64(len(b.BLSToExecutionChanges))
		if num > 16 {
			err = ssz.ErrIncorrectListSize
			return
		}
		for _, elem := range b.BLSToExecutionChanges {
			if err = elem.HashTreeRootWith(hh); err != nil {
				return
			}
		}
		hh.MerkleizeWithMixin(subIndx, num, 16)
	}

	hh.Merkleize(indx)
	return
}

// GetTree ssz hashes the BlindedBeaconBlockBody object
func (b *BlindedBeaconBlockBody) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(b)
}

// SignedBLSToExecutionChange provides information about a signed BLS to execution change.
type SignedBLSToExecutionChange struct {
	Message   *BLSToExecutionChange `json:"message"`
	Signature types.Signature       `json:"signature" ssz-size:"96"`
}

// SizeSSZ returns the ssz encoded size in bytes for the SignedBLSToExecutionChange object
func (s *SignedBLSToExecutionChange) SizeSSZ() (size int) {
	size = 172
	return
}

// HashTreeRoot ssz hashes the SignedBLSToExecutionChange object
func (s *SignedBLSToExecutionChange) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(s)
}

// HashTreeRootWith ssz hashes the SignedBLSToExecutionChange object with a hasher
func (s *SignedBLSToExecutionChange) HashTreeRootWith(hh ssz.HashWalker) (err error) {
	indx := hh.Index()

	// Field (0) 'Message'
	if s.Message == nil {
		s.Message = new(BLSToExecutionChange)
	}
	if err = s.Message.HashTreeRootWith(hh); err != nil {
		return
	}

	// Field (1) 'Signature'
	hh.PutBytes(s.Signature[:])

	hh.Merkleize(indx)
	return
}

// GetTree ssz hashes the SignedBLSToExecutionChange object
func (s *SignedBLSToExecutionChange) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(s)
}

// BLSToExecutionChange provides information about a change of withdrawal credentials.
type BLSToExecutionChange struct {
	ValidatorIndex     uint64          `json:"validator_index,string"`
	FromBLSPubkey      types.PublicKey `json:"from_bls_pubkey" ssz-size:"48"`
	ToExecutionAddress types.Address   `json:"to_execution_address" ssz-size:"20"`
}

// SizeSSZ returns the ssz encoded size in bytes for the BLSToExecutionChange object
func (b *BLSToExecutionChange) SizeSSZ() (size int) {
	size = 76
	return
}

// HashTreeRoot ssz hashes the BLSToExecutionChange object
func (b *BLSToExecutionChange) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(b)
}

// HashTreeRootWith ssz hashes the BLSToExecutionChange object with a hasher
func (b *BLSToExecutionChange) HashTreeRootWith(hh ssz.HashWalker) (err error) {
	indx := hh.Index()

	// Field (0) 'ValidatorIndex'
	hh.PutUint64(uint64(b.ValidatorIndex))

	// Field (1) 'FromBLSPubkey'
	hh.PutBytes(b.FromBLSPubkey[:])

	// Field (2) 'ToExecutionAddress'
	hh.PutBytes(b.ToExecutionAddress[:])

	hh.Merkleize(indx)
	return
}

// GetTree ssz hashes the BLSToExecutionChange object
func (b *BLSToExecutionChange) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(b)
}
