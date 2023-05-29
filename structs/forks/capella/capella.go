package capella

import (
	"errors"
	"fmt"
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

// UnmarshalSSZ ssz unmarshals the SubmitBlockRequest object
func (s *SubmitBlockRequest) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 336 {
		return ssz.ErrSize
	}

	tail := buf
	var o1 uint64

	if err = s.CapellaMessage.UnmarshalSSZ(buf[0:236]); err != nil {
		return err
	}

	// Offset (1) 'ExecutionPayload'
	if o1 = ssz.ReadOffset(buf[236:240]); o1 > size {
		return ssz.ErrOffset
	}

	if o1 < 336 {
		return ssz.ErrInvalidVariableOffset
	}

	// Field (2) 'Signature'
	copy(s.CapellaSignature[:], buf[240:336])

	// Field (1) 'ExecutionPayload'
	{
		buf = tail[o1:]
		if err = s.CapellaExecutionPayload.UnmarshalSSZ(buf); err != nil {
			return err
		}
	}
	return err
}

// SizeSSZ returns the ssz encoded size in bytes for the SubmitBlockRequest object
func (s *SubmitBlockRequest) SizeSSZ() (size int) {
	size = 336

	// Field (1) 'ExecutionPayload'
	size += s.CapellaExecutionPayload.SizeSSZ()

	return
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

func (s *SubmitBlockRequest) toBlockBidAndTrace(signedBuilderBid SignedBuilderBid) (structs.BlockAndTraceExtended, error) {
	execHeaderHash, err := signedBuilderBid.CapellaMessage.CapellaHeader.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to calculate header hash: %w", err)
	}

	return &BlockAndTraceExtended{
		CapellaTrace: SignedBidTrace{
			Message: BidTrace{
				Slot:                 s.CapellaMessage.Slot,
				ParentHash:           s.CapellaMessage.ParentHash,
				BlockHash:            s.CapellaMessage.BlockHash,
				BuilderPubkey:        s.CapellaMessage.BuilderPubkey,
				ProposerPubkey:       s.CapellaMessage.ProposerPubkey,
				ProposerFeeRecipient: s.CapellaMessage.ProposerFeeRecipient,
				GasLimit:             s.CapellaMessage.GasLimit,
				GasUsed:              s.CapellaMessage.GasUsed,
				Value:                s.CapellaMessage.Value,
			},
			Signature: s.CapellaSignature,
		},
		CapellaPayload: GetPayloadResponse{
			CapellaVersion: types.VersionString("capella"),
			CapellaData:    s.CapellaExecutionPayload,
		},
		CapellaExecutionHeaderHash: execHeaderHash,
	}, nil
}

func (s *SubmitBlockRequest) PreparePayloadContents(sk *bls.SecretKey, pubkey *types.PublicKey, domain types.Domain) (cbs structs.CompleteBlockstruct, err error) {
	signedBuilderBid, err := s.toSignedBuilderBid(sk, pubkey, domain)
	if err != nil {
		return cbs, err
	}

	cbs.Payload, err = s.toBlockBidAndTrace(signedBuilderBid)
	if err != nil {
		return cbs, fmt.Errorf("failed to create BlockBidAndTrace: %w", err)
	}

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

func (b *BuilderBid) Header() structs.ExecutionPayloadHeader {
	return b.CapellaHeader
}

// MarshalSSZ ssz marshals the BuilderBid object
func (b *BuilderBid) MarshalSSZ() ([]byte, error) {
	return ssz.MarshalSSZ(b)
}

// MarshalSSZTo ssz marshals the BuilderBid object to a target array
func (b *BuilderBid) MarshalSSZTo(buf []byte) (dst []byte, err error) {
	dst = buf
	offset := int(84)

	// Offset (0) 'Header'
	dst = ssz.WriteOffset(dst, offset)
	if b.CapellaHeader == nil {
		b.CapellaHeader = new(ExecutionPayloadHeader)
	}
	offset += b.CapellaHeader.SizeSSZ()

	// Field (1) 'Value'
	for i := 0; i < 32; i++ {
		dst = append(dst, b.CapellaValue[31-i])
	}

	// Field (2) 'Pubkey'
	dst = append(dst, b.CapellaPubkey[:]...)

	// Field (0) 'Header'
	if dst, err = b.CapellaHeader.MarshalSSZTo(dst); err != nil {
		return
	}

	return
}

// UnmarshalSSZ ssz unmarshals the BuilderBid object
func (b *BuilderBid) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 84 {
		return ssz.ErrSize
	}

	tail := buf
	var o0 uint64

	// Offset (0) 'Header'
	if o0 = ssz.ReadOffset(buf[0:4]); o0 > size {
		return ssz.ErrOffset
	}

	if o0 < 84 {
		return ssz.ErrInvalidVariableOffset
	}

	// Field (1) 'Value'
	value := make([]byte, 32)
	for i := 0; i < 32; i++ {
		value[i] = buf[35-i]
	}

	b.CapellaValue.FromSlice(value)

	// Field (2) 'Pubkey'
	copy(b.CapellaPubkey[:], buf[36:84])

	// Field (0) 'Header'
	{
		buf = tail[o0:]
		if b.CapellaHeader == nil {
			b.CapellaHeader = new(ExecutionPayloadHeader)
		}
		if err = b.CapellaHeader.UnmarshalSSZ(buf); err != nil {
			return err
		}
	}
	return err
}

// SizeSSZ returns the ssz encoded size in bytes for the BuilderBid object
func (b *BuilderBid) SizeSSZ() (size int) {
	size = 84

	// Field (0) 'Header'
	if b.CapellaHeader == nil {
		b.CapellaHeader = new(ExecutionPayloadHeader)
	}
	size += b.CapellaHeader.SizeSSZ()

	return
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

type BuilderBidExtended struct {
	CapellaBuilderBid BuilderBid      `json:"bid"`
	CapellaProposer   types.PublicKey `json:"proposer"`
	CapellaSlot       uint64          `json:"slot"`
}

// MarshalSSZ ssz marshals the BuilderBid object
func (b *BuilderBidExtended) MarshalSSZ() ([]byte, error) {
	return ssz.MarshalSSZ(b)
}

// MarshalSSZTo ssz marshals the BuilderBid object to a target array
func (b *BuilderBidExtended) MarshalSSZTo(buf []byte) (dst []byte, err error) {
	dst = buf
	offset := int(60)

	// Offset (0) 'BuilderBid'
	dst = ssz.WriteOffset(dst, offset)

	// Field (1) 'Proposer'
	dst = append(dst, b.CapellaProposer[:]...)

	// Field (2) 'Slot'
	dst = ssz.MarshalUint64(dst, b.CapellaSlot)

	// Field (0) 'BuilderBid'
	if dst, err = b.CapellaBuilderBid.MarshalSSZTo(dst); err != nil {
		return
	}

	return
}

// UnmarshalSSZ ssz unmarshals the BuilderBid object
func (b *BuilderBidExtended) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 60 {
		return ssz.ErrSize
	}

	tail := buf
	var o0 uint64

	// Offset (0) 'BuilderBid'
	if o0 = ssz.ReadOffset(buf[0:4]); o0 > size {
		return ssz.ErrOffset
	}

	if o0 < 60 {
		return ssz.ErrInvalidVariableOffset
	}

	// Field (1) 'Proposer'
	copy(b.CapellaProposer[:], buf[4:52])

	b.CapellaSlot = ssz.UnmarshallUint64(buf[52:60])
	// Field (0) 'Header'
	{
		buf = tail[o0:]
		if err = b.CapellaBuilderBid.UnmarshalSSZ(buf); err != nil {
			return err
		}
	}
	return err
}

// SizeSSZ returns the ssz encoded size in bytes for the BuilderBid object
func (b *BuilderBidExtended) SizeSSZ() (size int) {
	size = 60

	// Field (0) 'BuilderBid'
	size += b.CapellaBuilderBid.SizeSSZ()

	return
}

// HashTreeRoot ssz hashes the BuilderBid object
func (b *BuilderBidExtended) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(b)
}

// HashTreeRootWith ssz hashes the BuilderBid object with a hasher
func (b *BuilderBidExtended) HashTreeRootWith(hh ssz.HashWalker) (err error) {
	indx := hh.Index()

	// Field (0) 'BuilderBid'
	if err = b.CapellaBuilderBid.HashTreeRootWith(hh); err != nil {
		return
	}

	// Field (1) 'Proposer'
	hh.PutBytes(b.CapellaProposer[:])

	// Field (2) 'Slot'
	hh.PutUint64(b.CapellaSlot)

	hh.Merkleize(indx)
	return
}

// GetTree ssz hashes the SignedBuilderBid object
func (s *BuilderBidExtended) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(s)
}

func (b BuilderBidExtended) BuilderBid() structs.BuilderBid {
	return &b.CapellaBuilderBid
}

func (b BuilderBidExtended) Proposer() types.PublicKey {
	return b.CapellaProposer
}

func (b BuilderBidExtended) Slot() uint64 {
	return b.CapellaSlot
}

// ExecutionPayload represents an execution layer payload.
type ExecutionPayload struct {
	bellatrix.ExecutionPayload
	EpWithdrawals structs.Withdrawals `json:"withdrawals" ssz-max:"16"`
}

// MarshalSSZ ssz marshals the ExecutionPayload object
func (e *ExecutionPayload) MarshalSSZ() ([]byte, error) {
	return ssz.MarshalSSZ(e)
}

// MarshalSSZTo ssz marshals the ExecutionPayload object to a target array
func (e *ExecutionPayload) MarshalSSZTo(buf []byte) (dst []byte, err error) {
	dst = buf
	offset := int(512)

	// Field (0) 'ParentHash'
	dst = append(dst, e.EpParentHash[:]...)

	// Field (1) 'FeeRecipient'
	dst = append(dst, e.EpFeeRecipient[:]...)

	// Field (2) 'StateRoot'
	dst = append(dst, e.EpStateRoot[:]...)

	// Field (3) 'ReceiptsRoot'
	dst = append(dst, e.EpReceiptsRoot[:]...)

	// Field (4) 'LogsBloom'
	dst = append(dst, e.EpLogsBloom[:]...)

	// Field (5) 'PrevRandao'
	dst = append(dst, e.EpRandom[:]...)

	// Field (6) 'BlockNumber'
	dst = ssz.MarshalUint64(dst, e.EpBlockNumber)

	// Field (7) 'GasLimit'
	dst = ssz.MarshalUint64(dst, e.EpGasLimit)

	// Field (8) 'GasUsed'
	dst = ssz.MarshalUint64(dst, e.EpGasUsed)

	// Field (9) 'Timestamp'
	dst = ssz.MarshalUint64(dst, e.EpTimestamp)

	// Offset (10) 'ExtraData'
	dst = ssz.WriteOffset(dst, offset)
	offset += len(e.EpExtraData)

	// Field (11) 'BaseFeePerGas'
	dst = append(dst, e.EpBaseFeePerGas[:]...)

	// Field (12) 'BlockHash'
	dst = append(dst, e.EpBlockHash[:]...)

	// Offset (13) 'Transactions'
	dst = ssz.WriteOffset(dst, offset)
	for ii := 0; ii < len(e.EpTransactions); ii++ {
		offset += 4
		offset += len(e.EpTransactions[ii])
	}

	// Offset (14) 'Withdrawals'
	dst = ssz.WriteOffset(dst, offset)
	offset += len(e.EpWithdrawals) * 44

	// Field (10) 'ExtraData'
	if size := len(e.EpExtraData); size > 32 {
		err = ssz.ErrBytesLengthFn("ExecutionPayload.ExtraData", size, 32)
		return
	}
	dst = append(dst, e.EpExtraData...)

	// Field (13) 'Transactions'
	if size := len(e.EpTransactions); size > 1048576 {
		err = ssz.ErrListTooBigFn("ExecutionPayload.Transactions", size, 1048576)
		return
	}
	{
		offset = 4 * len(e.EpTransactions)
		for ii := 0; ii < len(e.EpTransactions); ii++ {
			dst = ssz.WriteOffset(dst, offset)
			offset += len(e.EpTransactions[ii])
		}
	}
	for ii := 0; ii < len(e.EpTransactions); ii++ {
		if size := len(e.EpTransactions[ii]); size > 1073741824 {
			err = ssz.ErrBytesLengthFn("ExecutionPayload.Transactions[ii]", size, 1073741824)
			return
		}
		dst = append(dst, e.EpTransactions[ii]...)
	}

	// Field (14) 'Withdrawals'
	if size := len(e.EpWithdrawals); size > 16 {
		err = ssz.ErrListTooBigFn("ExecutionPayload.Withdrawals", size, 16)
		return
	}
	for ii := 0; ii < len(e.EpWithdrawals); ii++ {
		if dst, err = e.EpWithdrawals[ii].MarshalSSZTo(dst); err != nil {
			return
		}
	}

	return
}

// UnmarshalSSZ ssz unmarshals the ExecutionPayload object
func (e *ExecutionPayload) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 512 {
		return ssz.ErrSize
	}

	tail := buf
	var o10, o13, o14 uint64

	// Field (0) 'ParentHash'
	copy(e.EpParentHash[:], buf[0:32])

	// Field (1) 'FeeRecipient'
	copy(e.EpFeeRecipient[:], buf[32:52])

	// Field (2) 'StateRoot'
	copy(e.EpStateRoot[:], buf[52:84])

	// Field (3) 'ReceiptsRoot'
	copy(e.EpReceiptsRoot[:], buf[84:116])

	// Field (4) 'LogsBloom'
	copy(e.EpLogsBloom[:], buf[116:372])

	// Field (5) 'PrevRandao'
	copy(e.EpRandom[:], buf[372:404])

	// Field (6) 'BlockNumber'
	e.EpBlockNumber = ssz.UnmarshallUint64(buf[404:412])

	// Field (7) 'GasLimit'
	e.EpGasLimit = ssz.UnmarshallUint64(buf[412:420])

	// Field (8) 'GasUsed'
	e.EpGasUsed = ssz.UnmarshallUint64(buf[420:428])

	// Field (9) 'Timestamp'
	e.EpTimestamp = ssz.UnmarshallUint64(buf[428:436])

	// Offset (10) 'ExtraData'
	if o10 = ssz.ReadOffset(buf[436:440]); o10 > size {
		return ssz.ErrOffset
	}

	if o10 < 512 {
		return ssz.ErrInvalidVariableOffset
	}

	// Field (11) 'BaseFeePerGas'
	copy(e.EpBaseFeePerGas[:], buf[440:472])

	// Field (12) 'BlockHash'
	copy(e.EpBlockHash[:], buf[472:504])

	// Offset (13) 'Transactions'
	if o13 = ssz.ReadOffset(buf[504:508]); o13 > size || o10 > o13 {
		return ssz.ErrOffset
	}

	// Offset (14) 'Withdrawals'
	if o14 = ssz.ReadOffset(buf[508:512]); o14 > size || o13 > o14 {
		return ssz.ErrOffset
	}

	// Field (10) 'ExtraData'
	{
		buf = tail[o10:o13]
		if len(buf) > 32 {
			return ssz.ErrBytesLength
		}
		if cap(e.EpExtraData) == 0 {
			e.EpExtraData = make([]byte, 0, len(buf))
		}
		e.EpExtraData = append(e.EpExtraData, buf...)
	}

	// Field (13) 'Transactions'
	{
		buf = tail[o13:o14]
		num, err := ssz.DecodeDynamicLength(buf, 1048576)
		if err != nil {
			return err
		}
		e.EpTransactions = make([]hexutil.Bytes, num)
		err = ssz.UnmarshalDynamic(buf, num, func(indx int, buf []byte) (err error) {
			if len(buf) > 1073741824 {
				return ssz.ErrBytesLength
			}
			if cap(e.EpTransactions[indx]) == 0 {
				e.EpTransactions[indx] = make([]byte, 0, len(buf))
			}
			e.EpTransactions[indx] = append(e.EpTransactions[indx], buf...)
			return nil
		})
		if err != nil {
			return err
		}
	}

	// Field (14) 'Withdrawals'
	{
		buf = tail[o14:]
		num, err := ssz.DivideInt2(len(buf), 44, 16)
		if err != nil {
			return err
		}
		e.EpWithdrawals = make([]*structs.Withdrawal, num)
		for ii := 0; ii < num; ii++ {
			if e.EpWithdrawals[ii] == nil {
				e.EpWithdrawals[ii] = new(structs.Withdrawal)
			}
			if err = e.EpWithdrawals[ii].UnmarshalSSZ(buf[ii*44 : (ii+1)*44]); err != nil {
				return err
			}
		}
	}
	return err
}

// SizeSSZ returns the ssz encoded size in bytes for the ExecutionPayload object
func (e *ExecutionPayload) SizeSSZ() (size int) {
	size = 512

	// Field (10) 'ExtraData'
	size += len(e.EpExtraData)

	// Field (13) 'Transactions'
	for ii := 0; ii < len(e.EpTransactions); ii++ {
		size += 4
		size += len(e.EpTransactions[ii])
	}

	// Field (14) 'Withdrawals'
	size += len(e.EpWithdrawals) * 44

	return
}

// HashTreeRoot ssz hashes the ExecutionPayload object
func (e *ExecutionPayload) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(e)
}

// HashTreeRootWith ssz hashes the ExecutionPayload object with a hasher
func (e *ExecutionPayload) HashTreeRootWith(hh ssz.HashWalker) (err error) {
	indx := hh.Index()

	// Field (0) 'ParentHash'
	hh.PutBytes(e.EpParentHash[:])

	// Field (1) 'FeeRecipient'
	hh.PutBytes(e.EpFeeRecipient[:])

	// Field (2) 'StateRoot'
	hh.PutBytes(e.EpStateRoot[:])

	// Field (3) 'ReceiptsRoot'
	hh.PutBytes(e.EpReceiptsRoot[:])

	// Field (4) 'LogsBloom'
	hh.PutBytes(e.EpLogsBloom[:])

	// Field (5) 'PrevRandao'
	hh.PutBytes(e.EpRandom[:])

	// Field (6) 'BlockNumber'
	hh.PutUint64(e.EpBlockNumber)

	// Field (7) 'GasLimit'
	hh.PutUint64(e.EpGasLimit)

	// Field (8) 'GasUsed'
	hh.PutUint64(e.EpGasUsed)

	// Field (9) 'Timestamp'
	hh.PutUint64(e.EpTimestamp)

	// Field (10) 'ExtraData'
	{
		elemIndx := hh.Index()
		byteLen := uint64(len(e.EpExtraData))
		if byteLen > 32 {
			err = ssz.ErrIncorrectListSize
			return
		}
		hh.PutBytes(e.EpExtraData)
		hh.MerkleizeWithMixin(elemIndx, byteLen, (32+31)/32)
	}

	// Field (11) 'BaseFeePerGas'
	hh.PutBytes(e.EpBaseFeePerGas[:])

	// Field (12) 'BlockHash'
	hh.PutBytes(e.EpBlockHash[:])

	// Field (13) 'Transactions'
	{
		subIndx := hh.Index()
		num := uint64(len(e.EpTransactions))
		if num > 1048576 {
			err = ssz.ErrIncorrectListSize
			return
		}
		for _, elem := range e.EpTransactions {
			{
				elemIndx := hh.Index()
				byteLen := uint64(len(elem))
				if byteLen > 1073741824 {
					err = ssz.ErrIncorrectListSize
					return
				}
				hh.AppendBytes32(elem)
				hh.MerkleizeWithMixin(elemIndx, byteLen, (1073741824+31)/32)
			}
		}
		hh.MerkleizeWithMixin(subIndx, num, 1048576)
	}

	// Field (14) 'Withdrawals'
	{
		subIndx := hh.Index()
		num := uint64(len(e.EpWithdrawals))
		if num > 16 {
			err = ssz.ErrIncorrectListSize
			return
		}
		for _, elem := range e.EpWithdrawals {
			if err = elem.HashTreeRootWith(hh); err != nil {
				return
			}
		}
		hh.MerkleizeWithMixin(subIndx, num, 16)
	}

	hh.Merkleize(indx)
	return
}

// GetTree ssz hashes the ExecutionPayload object
func (e *ExecutionPayload) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(e)
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

type BlockAndTraceExtended struct {
	CapellaPayload             GetPayloadResponse
	CapellaTrace               SignedBidTrace
	CapellaExecutionHeaderHash types.Hash `json:"execution_header_hash" ssz-size:"32"`
}

func (bte *BlockAndTraceExtended) MarshalSSZ() ([]byte, error) {
	return ssz.MarshalSSZ(bte)
}

// MarshalSSZTo ssz marshals the BlockAndTraceExtended object to a target array
func (bte *BlockAndTraceExtended) MarshalSSZTo(buf []byte) ([]byte, error) {
	// Field (0) 'CapellaPayload'
	payloadOffset := 4 + 332 + 32
	buf = ssz.WriteOffset(buf, payloadOffset)

	// Field (1) 'CapellaTrace'
	buf, err := bte.CapellaTrace.MarshalSSZTo(buf)
	if err != nil {
		return buf, fmt.Errorf("failed to marshal trace: %w", err)
	}

	// Field (2) 'CapellaExecutionHeaderHash'
	buf = append(buf, bte.CapellaExecutionHeaderHash[:]...)

	// Field (0) 'CapellaPayload'
	buf, err = bte.CapellaPayload.MarshalSSZTo(buf)
	if err != nil {
		return buf, fmt.Errorf("failed to marshal payload: %w", err)
	}

	return buf, nil
}

func (bte *BlockAndTraceExtended) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 368 {
		return ssz.ErrSize
	}

	tail := buf
	var payloadOffset uint64

	// Offset (0) 'CapellaPayload'
	if payloadOffset = ssz.ReadOffset(buf[0:4]); payloadOffset > size {
		return ssz.ErrOffset
	}

	if payloadOffset < 368 {
		return ssz.ErrInvalidVariableOffset
	}

	// Offset (1) 'CapellaTrace'
	if err = bte.CapellaTrace.UnmarshalSSZ(buf[4:336]); err != nil {
		return fmt.Errorf("failed to unmarshal trace message: %w", err)
	}

	// Field (2) 'CapellaExecutionHeaderHash'
	copy(bte.CapellaExecutionHeaderHash[:], buf[336:368])

	// Field (0) 'CapellaPayload'
	{
		buf = tail[payloadOffset:]
		if err = bte.CapellaPayload.UnmarshalSSZ(buf); err != nil {
			return fmt.Errorf("failed to unmarshal payload data: %w", err)
		}
	}

	return nil
}

// SizeSSZ returns the ssz encoded size in bytes for the ExecutionPayloadHeader object
func (bte *BlockAndTraceExtended) SizeSSZ() (size int) {
	size = 368

	// Field (0) 'CapellaPayload'
	size += bte.CapellaPayload.SizeSSZ()

	return
}

func (bbat *BlockAndTraceExtended) BidValue() types.U256Str {
	return bbat.CapellaTrace.Message.Value
}

func (bbat *BlockAndTraceExtended) Slot() uint64 {
	return bbat.CapellaTrace.Message.Slot
}
func (bbat *BlockAndTraceExtended) Proposer() types.PublicKey {
	return bbat.CapellaTrace.Message.ProposerPubkey
}

func (bbat *BlockAndTraceExtended) ExecutionPayload() structs.ExecutionPayload {
	return &bbat.CapellaPayload.CapellaData
}

func (bbat *BlockAndTraceExtended) ExecutionHeaderHash() (types.Hash, error) {
	return bbat.CapellaExecutionHeaderHash, nil
}

func (bbat *BlockAndTraceExtended) BuilderPubkey() (pub types.PublicKey) {
	return bbat.CapellaTrace.Message.BuilderPubkey
}

func (bbat *BlockAndTraceExtended) ToDeliveredTrace(slot uint64) (dt structs.DeliveredTrace, err error) {
	return structs.DeliveredTrace{
		Trace: structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: types.BidTrace{
					Slot:                 slot,
					ParentHash:           bbat.CapellaPayload.CapellaData.EpParentHash,
					BlockHash:            bbat.CapellaPayload.CapellaData.EpBlockHash,
					BuilderPubkey:        bbat.CapellaTrace.Message.BuilderPubkey,
					ProposerPubkey:       bbat.CapellaTrace.Message.ProposerPubkey,
					ProposerFeeRecipient: bbat.CapellaTrace.Message.ProposerFeeRecipient,
					GasLimit:             bbat.CapellaPayload.CapellaData.EpGasLimit,
					GasUsed:              bbat.CapellaPayload.CapellaData.EpGasUsed,
					Value:                bbat.CapellaTrace.Message.Value,
				},
				BlockNumber: bbat.CapellaPayload.CapellaData.EpBlockNumber,
				NumTx:       uint64(len(bbat.CapellaPayload.CapellaData.EpTransactions)),
			},
			Timestamp: bbat.CapellaPayload.CapellaData.EpTimestamp,
		},
		BlockNumber: bbat.CapellaPayload.CapellaData.EpBlockNumber,
	}, nil
}

type GetPayloadResponse struct {
	CapellaVersion types.VersionString `json:"version"`
	CapellaData    ExecutionPayload    `json:"data"`
}

func (gpr *GetPayloadResponse) SizeSSZ() (size int) {
	// Field (0) 'CapellaVersion'
	size = 4 + len(gpr.CapellaVersion)

	// Field (1) 'CapellaData'
	size += 4 + gpr.CapellaData.SizeSSZ()

	return
}

func (gpr *GetPayloadResponse) MarshalSSZ() ([]byte, error) {
	return ssz.MarshalSSZ(gpr)
}

// MarshalSSZTo ssz marshals the GetPayloadResponse object to a target array
func (gpr *GetPayloadResponse) MarshalSSZTo(buf []byte) ([]byte, error) {
	// Field (0) 'CapellaVersion'
	versionOffset := 4 + 4
	buf = ssz.WriteOffset(buf, versionOffset)

	// Field (1) 'CapellaData'
	dataOffset := versionOffset + len(gpr.CapellaVersion)
	buf = ssz.WriteOffset(buf, dataOffset)

	// Field (0) 'CapellaVersion'
	buf = append(buf, []byte(gpr.CapellaVersion)...)

	// Field (1) 'CapellaData'
	return gpr.CapellaData.MarshalSSZTo(buf)
}

func (gpr *GetPayloadResponse) UnmarshalSSZ(buf []byte) error {
	size := uint64(len(buf))
	if size < 8 {
		return ssz.ErrSize
	}

	var versionOffset, dataOffset uint64

	// Offset (0) 'CapellaVersion'
	if versionOffset = ssz.ReadOffset(buf[0:4]); versionOffset > size {
		return ssz.ErrOffset
	}

	if versionOffset < 8 {
		return ssz.ErrInvalidVariableOffset
	}

	// Offset (1) 'CapellaData'
	if dataOffset = ssz.ReadOffset(buf[4:8]); dataOffset > size || versionOffset > dataOffset {
		return ssz.ErrOffset
	}

	// Field (0) 'CapellaVersion'
	gpr.CapellaVersion = types.VersionString(buf[versionOffset:dataOffset])

	// Field (1) 'CapellaData'
	{
		if err := gpr.CapellaData.UnmarshalSSZ(buf[dataOffset:]); err != nil {
			return err
		}
	}

	return nil
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

// SignedBidTrace is a BidTrace with a signature
type SignedBidTrace struct {
	Signature types.Signature `json:"signature" ssz-size:"96"`
	Message   BidTrace        `json:"message"`
}

func (sbt *SignedBidTrace) MarshalSSZ() ([]byte, error) {
	return ssz.MarshalSSZ(sbt)
}

// MarshalSSZTo ssz marshals the SignedBidTrace object to a target array
func (sbt *SignedBidTrace) MarshalSSZTo(buf []byte) ([]byte, error) {
	// Field (0) 'Signature'
	buf = append(buf, sbt.Signature[:]...)

	// Field (1) 'Message'
	return sbt.Message.MarshalSSZTo(buf)
}

func (sbt *SignedBidTrace) UnmarshalSSZ(buf []byte) error {
	size := uint64(len(buf))
	if size < 332 {
		return ssz.ErrSize
	}

	// Field (0) 'Signature'
	copy(sbt.Signature[:], buf[:96])

	// Field (1) 'Message'
	if err := sbt.Message.UnmarshalSSZ(buf[96:332]); err != nil {
		return err
	}

	return nil
}

func (sbt *SignedBidTrace) SizeSSZ() int {
	return 332
}

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

func (bt *BidTrace) MarshalSSZ() ([]byte, error) {
	return ssz.MarshalSSZ(bt)
}

// MarshalSSZTo ssz marshals the BidTrace object to a target array
func (bt *BidTrace) MarshalSSZTo(buf []byte) ([]byte, error) {
	// Field (0) 'Slot'
	buf = ssz.MarshalUint64(buf, bt.Slot)

	// Field (1) 'ParentHash'
	buf = append(buf, bt.ParentHash[:]...)

	// Field (2) 'BlockHash'
	buf = append(buf, bt.BlockHash[:]...)

	// Field (3) 'BuilderPubkey'
	buf = append(buf, bt.BuilderPubkey[:]...)

	// Field (4) 'ProposerPubkey'
	buf = append(buf, bt.ProposerPubkey[:]...)

	// Field (5) 'ProposerFeeRecipient'
	buf = append(buf, bt.ProposerFeeRecipient[:]...)

	// Field (6) 'GasLimit'
	buf = ssz.MarshalUint64(buf, bt.GasLimit)

	// Field (7) 'GasUsed'
	buf = ssz.MarshalUint64(buf, bt.GasUsed)

	// Field (8) 'Value'
	buf = append(buf, bt.Value[:]...)

	return buf, nil
}

func (bt *BidTrace) UnmarshalSSZ(buf []byte) error {
	size := uint64(len(buf))
	if size < 236 {
		return ssz.ErrSize
	}

	// Field (0) 'Slot'
	bt.Slot = ssz.UnmarshallUint64(buf[:8])

	// Field (1) 'ParentHash'
	copy(bt.ParentHash[:], buf[8:40])

	// Field (2) 'BlockHash'
	copy(bt.BlockHash[:], buf[40:72])

	// Field (3) 'BuilderPubkey'
	copy(bt.BuilderPubkey[:], buf[72:120])

	// Field (4) 'ProposerPubkey'
	copy(bt.ProposerPubkey[:], buf[120:168])

	// Field (5) 'ProposerFeeRecipient'
	copy(bt.ProposerFeeRecipient[:], buf[168:188])

	// Field (6) 'GasLimit'
	bt.GasLimit = ssz.UnmarshallUint64(buf[188:196])

	// Field (7) 'GasUsed'
	bt.GasUsed = ssz.UnmarshallUint64(buf[196:204])

	// Field (8) 'Value'
	copy(bt.Value[:], buf[204:236])

	return nil
}

func (bt *BidTrace) SizeSSZ() int {
	return 236
}
