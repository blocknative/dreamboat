package verify

import (
	"fmt"

	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	blst "github.com/supranational/blst/bindings/go"
)

var dst = []byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_")

const BLST_SUCCESS = 0x0

func VerifySignatureBytes(msg [32]byte, sigBytes, pkBytes []byte) (ok bool, err error) {
	defer func() { // better safe than sorry
		if r := recover(); r != nil {
			var isErr bool
			err, isErr = r.(error)
			if !isErr {
				err = fmt.Errorf("verify signature bytes panic: %v", r)
			}
		}
	}()

	sig, err := bls.SignatureFromBytes(sigBytes)
	if err != nil {
		return false, err
	}

	pk, err := bls.PublicKeyFromBytes(pkBytes)
	if err != nil {
		return false, err
	}

	if pk == nil || len(msg) == 0 || sig == nil {
		return false, nil
	}

	if blst.CoreVerifyPkInG1(pk, sig, true, msg[:], dst, nil) != BLST_SUCCESS {
		return false, bls.ErrInvalidSignature
	}

	return true, err
}

func VerifySignature(obj types.HashTreeRoot, d types.Domain, pkBytes, sigBytes []byte) (bool, error) {
	msg, err := types.ComputeSigningRoot(obj, d)
	if err != nil {
		return false, err
	}

	return VerifySignatureBytes(msg, sigBytes, pkBytes)
}
