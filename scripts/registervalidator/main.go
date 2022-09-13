package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	relay "github.com/blocknative/dreamboat/pkg"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/pkg/errors"
)

const (
	url = "http://localhost:18550" + relay.PathRegisterValidator
)

func main() {
	fmt.Print("registering validator... ")
	err := registerValidator()
	if err != nil {
		panic(err)
	}
}

func registerValidator() error {
	builderDomain, err := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		relay.GenesisForkVersionRopsten,
		types.Root{}.String())
	if err != nil {
		return err
	}

	validator, _, err := validValidatorRegistration(builderDomain)
	if err != nil {
		return err
	}

	b, err := json.Marshal([]*types.SignedValidatorRegistration{validator})
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(b))
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return errors.WithMessage(fmt.Errorf("invalid return code, expected 200 - received %d", resp.StatusCode), string(body))
	}

	fmt.Println(resp.Status)

	return nil
}

func validValidatorRegistration(domain types.Domain) (*types.SignedValidatorRegistration, *bls.SecretKey, error) {
	sk, pk, err := bls.GenerateNewKeypair()
	if err != nil {
		return nil, nil, err
	}

	var pubKey types.PublicKey
	pubKey.FromSlice(pk.Compress())

	msg := &types.RegisterValidatorRequestMessage{
		FeeRecipient: types.Address{0x42},
		GasLimit:     15_000_000,
		Timestamp:    1652369368,
		Pubkey:       pubKey,
	}

	signature, err := types.SignMessage(msg, domain, sk)
	if err != nil {
		return nil, nil, err
	}

	return &types.SignedValidatorRegistration{
		Message:   msg,
		Signature: signature,
	}, sk, nil
}
