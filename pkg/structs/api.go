package structs

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/flashbots/go-boost-utils/types"
)

type SignedValidatorRegistration struct {
	types.SignedValidatorRegistration
	Raw json.RawMessage
}

func (s *SignedValidatorRegistration) UnmarshalJSON(b []byte) error {
	sv := types.SignedValidatorRegistration{}
	err := json.Unmarshal(b, &sv)
	if err != nil {
		return err
	}
	s.SignedValidatorRegistration = sv
	s.Raw = b
	return nil
}

type HeaderRequest map[string]string

func (hr HeaderRequest) Slot() (Slot, error) {
	slot, err := strconv.Atoi(hr["slot"])
	return Slot(slot), err
}

func (hr HeaderRequest) ParentHash() (types.Hash, error) {
	var parentHash types.Hash
	err := parentHash.UnmarshalText([]byte(strings.ToLower(hr["parent_hash"])))
	return parentHash, err
}

func (hr HeaderRequest) Pubkey() (PubKey, error) {
	var pk PubKey
	if err := pk.UnmarshalText([]byte(strings.ToLower(hr["pubkey"]))); err != nil {
		return PubKey{}, fmt.Errorf("invalid public key")
	}
	return pk, nil
}
