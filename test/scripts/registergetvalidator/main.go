package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/blocknative/dreamboat/api"
	"github.com/blocknative/dreamboat/blstools"
	"github.com/blocknative/dreamboat/test/common"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
)

const (
	url = "http://localhost:18550" + api.PathRegisterValidator
)

func main() {
	fmt.Print("registering validator... ")
	err := registerValidator()
	if err != nil {
		panic(err)
	}
}

func registerValidator() error {
	builderDomain, err := common.ComputeDomain(
		types.DomainTypeAppBuilder,
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
		return fmt.Errorf("invalid return code, expected 200 - received %d - %s", resp.StatusCode, string(body))
	}

	fmt.Println(resp.Status)

	return nil
}

func validValidatorRegistration(domain types.Domain) (*types.SignedValidatorRegistration, *bls.SecretKey, error) {
	sk, pubKey, err := blstools.GenerateNewKeypair()
	if err != nil {
		return nil, nil, err
	}
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
