package capella

import (
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/blocknative/dreamboat/pkg/structs/forks"
	"github.com/blocknative/dreamboat/pkg/structs/forks/bellatrix"
	ssz "github.com/ferranbt/fastssz"
	"github.com/flashbots/go-boost-utils/types"
)

type CapellaSubmitBlockRequest struct {
	Message          types.BidTrace
	ExecutionPayload ExecutionPayload
	Signature        types.Signature `ssz-size:"96"` //phase0.BLSSignature `ssz-size:"96"`
}

// ExecutionPayload represents an execution layer payload.
type ExecutionPayload struct {
	bellatrix.ExecutionPayload
	EpWithdrawals Withdrawals `json:"withdrawals" ssz-max:"16"`
}

func (ep *ExecutionPayload) Withdrawals() Withdrawals {
	return ep.EpWithdrawals
}

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
