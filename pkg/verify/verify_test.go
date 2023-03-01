package verify_test

import (
	"testing"

	"github.com/blocknative/dreamboat/blstools"
	"github.com/blocknative/dreamboat/pkg/verify"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/stretchr/testify/require"

	pkg "github.com/blocknative/dreamboat/pkg"
	"github.com/blocknative/dreamboat/test/common"
)

func BenchmarkSignatureValidation(b *testing.B) {
	relaySigningDomain, _ := common.ComputeDomain(
		types.DomainTypeAppBuilder,
		pkg.GenesisForkVersionRopsten,
		types.Root{}.String())
	registration, _ := validValidatorRegistration(b, relaySigningDomain)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := verify.VerifySignature(
			registration.Message,
			relaySigningDomain,
			registration.Message.Pubkey[:],
			registration.Signature[:])
		if err != nil {
			panic(err)
		}
	}
}

func validValidatorRegistration(t require.TestingT, domain types.Domain) (*types.SignedValidatorRegistration, *bls.SecretKey) {
	sk, pubKey, err := blstools.GenerateNewKeypair()
	require.NoError(t, err)

	msg := &types.RegisterValidatorRequestMessage{
		FeeRecipient: types.Address{0x42},
		GasLimit:     15_000_000,
		Timestamp:    1652369368,
		Pubkey:       pubKey,
	}

	signature, err := types.SignMessage(msg, domain, sk)
	require.NoError(t, err)
	return &types.SignedValidatorRegistration{
		Message:   msg,
		Signature: signature,
	}, sk
}
