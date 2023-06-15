package blstools

import (
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
)

func GenerateNewKeypair() (sk *bls.SecretKey, pubKey types.PublicKey, err error) {
	sk, pk, err := bls.GenerateNewKeypair()
	if err != nil {
		return nil, pubKey, err
	}

	pkBytes := pk.Bytes()
	err = pubKey.FromSlice(pkBytes[:]) //nolint

	return sk, pubKey, err
}

func SecretKeyFromBytes(skBytes []byte) (sk *bls.SecretKey, pk types.PublicKey, err error) {
	sk, err = bls.SecretKeyFromBytes(skBytes[:])
	if err != nil {
		return nil, types.PublicKey{}, err
	}

	pubkey, err := bls.PublicKeyFromSecretKey(sk)
	if err != nil {
		return nil, types.PublicKey{}, err
	}

	pubkeyBytes := pubkey.Bytes()
	err = pk.FromSlice(pubkeyBytes[:]) //nolint

	return sk, pk, err
}
