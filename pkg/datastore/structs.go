package datastore

import (
	"fmt"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
)

func HeaderHashKey(bh types.Hash) ds.Key {
	return ds.NewKey(fmt.Sprintf("header-hash-%s", bh.String()))
}

func HeaderNumKey(bn uint64) ds.Key {
	return ds.NewKey(fmt.Sprintf("header-num-%d", bn))
}

func PayloadKeyKey(key structs.PayloadKey) ds.Key {
	return ds.NewKey(fmt.Sprintf("payload-%s-%s-%d", key.BlockHash.String(), key.Proposer.String(), key.Slot))
}

const (
	HeaderPrefix        = "header-"
	HeaderContentPrefix = "hc/"
)

var (
	HeaderPrefixBytes = []byte("header-")
)

func HeaderKeyContent(slot uint64, blockHash string) ds.Key {
	return ds.NewKey(fmt.Sprintf("hc/%d/%s", slot, blockHash))
}

func HeaderMaxNewKey(slot uint64) ds.Key {
	return ds.NewKey(fmt.Sprintf("hm/%d", slot))
}

func HeaderKey(slot uint64) ds.Key {
	return ds.NewKey(fmt.Sprintf("%s%d", HeaderPrefix, slot))
}
