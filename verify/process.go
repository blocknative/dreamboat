package verify

import (
	"errors"
	"fmt"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
)

var dst = []byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_")

const (
	PublicKeyLength = bls12381.SizeOfG1AffineCompressed
	SecretKeyLength = fr.Bytes
	SignatureLength = bls12381.SizeOfG2AffineCompressed
)

type (
	PublicKey = bls12381.G1Affine
	SecretKey = fr.Element
	Signature = bls12381.G2Affine
)

var (
	_, _, g1One, _            = bls12381.Generators()
	ErrInvalidPubkeyLength    = errors.New("invalid public key length")
	ErrInvalidSecretKeyLength = errors.New("invalid secret key length")
	ErrInvalidSignatureLength = errors.New("invalid signature length")
	ErrSecretKeyIsZero        = errors.New("invalid secret key is zero")
)

func PublicKeyFromBytes(pkBytes []byte) (*PublicKey, error) {
	if len(pkBytes) != PublicKeyLength {
		return nil, ErrInvalidPubkeyLength
	}
	pk := new(PublicKey)
	err := pk.Unmarshal(pkBytes)
	return pk, err
}

func SignatureFromBytes(sigBytes []byte) (*Signature, error) {
	if len(sigBytes) != SignatureLength {
		return nil, ErrInvalidSignatureLength
	}
	sig := new(Signature)
	err := sig.Unmarshal(sigBytes)
	return sig, err
}

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

	pk, err := PublicKeyFromBytes(pkBytes)
	if err != nil {
		return false, err
	}
	sig, err := SignatureFromBytes(sigBytes)
	if err != nil {
		return false, err
	}
	return VerifySignature(sig, pk, msg[:])
}

func VerifySignature(sig *Signature, pk *PublicKey, msg []byte) (bool, error) {
	Q, err := bls12381.HashToG2(msg, dst)
	if err != nil {
		return false, err
	}
	var negP bls12381.G1Affine
	negP.Neg(&g1One)
	return bls12381.PairingCheck(
		[]bls12381.G1Affine{*pk, negP},
		[]bls12381.G2Affine{Q, *sig},
	)
}
