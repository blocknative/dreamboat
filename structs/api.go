package structs

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/flashbots/go-boost-utils/types"
)

type HeaderRequest map[string]string

func (hr HeaderRequest) Slot() (Slot, error) {
	slot, err := strconv.ParseUint(hr["slot"], 10, 64)
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
