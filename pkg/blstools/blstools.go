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
	err = pubKey.FromSlice(pk.Compress()) //nolint

	return sk, pubKey, err
}
