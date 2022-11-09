package structs

import (
	"fmt"

	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
)

type Slot uint64

func (s Slot) Loggable() map[string]any {
	return map[string]any{
		"slot":  s,
		"epoch": s.Epoch(),
	}
}

func (s Slot) Epoch() Epoch {
	return Epoch(s / SlotsPerEpoch)
}

func (s Slot) HeaderKey() ds.Key {
	return ds.NewKey(fmt.Sprintf("header-%d", s))
}

func (s Slot) PayloadKey() ds.Key {
	return ds.NewKey(fmt.Sprintf("payload-%d", s))
}

type Epoch uint64

func (e Epoch) Loggable() map[string]any {
	return map[string]any{
		"epoch": e,
	}
}

type PubKey struct{ types.PublicKey }

func (pk PubKey) Loggable() map[string]any {
	return map[string]any{
		"pubkey": pk,
	}
}

func (pk PubKey) Bytes() []byte {
	return pk.PublicKey[:]
}

func (pk PubKey) ValidatorKey() ds.Key {
	return ds.NewKey(fmt.Sprintf("valdator-%s", pk))
}

func (pk PubKey) RegistrationKey() ds.Key {
	return ds.NewKey(fmt.Sprintf("registration-%s", pk))
}

type TraceQuery struct {
	Slot          Slot
	BlockHash     types.Hash
	BlockNum      uint64
	Pubkey        types.PublicKey
	Cursor, Limit uint64
}

func (q TraceQuery) HasSlot() bool {
	return q.Slot != Slot(0)
}

func (q TraceQuery) HasBlockHash() bool {
	return q.BlockHash != types.Hash{}
}

func (q TraceQuery) HasBlockNum() bool {
	return q.BlockNum != 0
}

func (q TraceQuery) HasPubkey() bool {
	return q.Pubkey != types.PublicKey{}
}

func (q TraceQuery) HasCursor() bool {
	return q.Cursor != 0
}

func (q TraceQuery) HasLimit() bool {
	return q.Limit != 0
}
