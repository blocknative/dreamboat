package structs

import (
	ssz "github.com/ferranbt/fastssz"
	"github.com/flashbots/go-boost-utils/types"
)

// Withdrawal provides information about a withdrawal.
type Withdrawals []*Withdrawal

// Withdrawal provides information about a withdrawal.
type HashWithdrawals struct {
	Withdrawals Withdrawals `ssz-max:"16"`
}

// SizeSSZ returns the ssz encoded size in bytes for the Withdrawals object
func (w *HashWithdrawals) SizeSSZ() (size int) {
	size = 4

	// Field (0) 'Withdrawals'
	size += len(w.Withdrawals) * 44

	return
}

// HashTreeRoot ssz hashes the Withdrawals object
func (w *HashWithdrawals) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(w)
}

// HashTreeRootWith ssz hashes the Withdrawals object with a hasher
func (w *HashWithdrawals) HashTreeRootWith(hh ssz.HashWalker) (err error) {
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
func (w *HashWithdrawals) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(w)
}

// Withdrawal provides information about a withdrawal.
type Withdrawal struct {
	Index          uint64        `json:"index,string"`
	ValidatorIndex uint64        `json:"validator_index,string"`
	Address        types.Address `json:"address" ssz-size:"20"`
	Amount         uint64        `json:"amount,string"`
}

func (w *Withdrawal) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, 0)
	buf, err := w.MarshalSSZTo(buf)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// MarshalSSZTo ssz marshals the Withdrawal object to a target array
func (w *Withdrawal) MarshalSSZTo(buf []byte) ([]byte, error) {
	// Field (0) 'Index'
	buf = ssz.MarshalUint64(buf, w.Index)

	// Field (1) 'ValidatorIndex'
	buf = ssz.MarshalUint64(buf, w.ValidatorIndex)

	// Field (2) 'Address'
	buf = append(buf, w.Address[:]...)

	// Field (3) 'Amount'
	buf = ssz.MarshalUint64(buf, w.Amount)

	return buf, nil
}

// UnmarshalSSZ ssz unmarshals the Withdrawal object
func (w *Withdrawal) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 44 {
		return ssz.ErrSize
	}

	// Field (0) 'Index'
	w.Index = uint64(ssz.UnmarshallUint64(buf[0:8]))

	// Field (1) 'ValidatorIndex'
	w.ValidatorIndex = uint64(ssz.UnmarshallUint64(buf[8:16]))

	// Field (2) 'Address'
	copy(w.Address[:], buf[16:36])

	// Field (3) 'Amount'
	w.Amount = uint64(ssz.UnmarshallUint64(buf[36:44]))

	return err
}

// SizeSSZ returns the ssz encoded size in bytes for the Withdrawal object
func (w *Withdrawal) SizeSSZ() (size int) {
	size = 44
	return
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
