package badger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/blocknative/dreamboat/structs"
	"github.com/dgraph-io/badger/v2"
	"github.com/flashbots/go-boost-utils/types"
	"golang.org/x/exp/constraints"

	ds "github.com/ipfs/go-datastore"
)

func DeliveredKey(slot structs.Slot) ds.Key {
	return ds.NewKey(fmt.Sprintf("delivered-%d", slot))
}

func DeliveredHashKey(bh types.Hash) ds.Key {
	return ds.NewKey(fmt.Sprintf("delivered-hash-%s", bh.String()))
}

func DeliveredNumKey(bn uint64) ds.Key {
	return ds.NewKey(fmt.Sprintf("delivered-num-%d", bn))
}

func DeliveredPubkeyKey(pk types.PublicKey) ds.Key {
	return ds.NewKey(fmt.Sprintf("delivered-pk-%s", pk.String()))
}

func (s *Datastore) PutDelivered(ctx context.Context, slot structs.Slot, trace structs.DeliveredTrace) error {
	data, err := json.Marshal(trace.Trace)
	if err != nil {
		return err
	}

	txn := s.DBInter.NewTransaction(true)
	defer txn.Discard()
	if err := txn.SetEntry(badger.NewEntry(DeliveredHashKey(trace.Trace.BlockHash).Bytes(), DeliveredKey(slot).Bytes()).WithTTL(s.TTL)); err != nil {
		return err
	}
	if err := txn.SetEntry(badger.NewEntry(DeliveredNumKey(trace.BlockNumber).Bytes(), DeliveredKey(slot).Bytes()).WithTTL(s.TTL)); err != nil {
		return err
	}
	if err := txn.SetEntry(badger.NewEntry(DeliveredPubkeyKey(trace.Trace.ProposerPubkey).Bytes(), DeliveredKey(slot).Bytes()).WithTTL(s.TTL)); err != nil {
		return err
	}
	if err := txn.SetEntry(badger.NewEntry(DeliveredKey(slot).Bytes(), data).WithTTL(s.TTL)); err != nil {
		return err
	}

	return txn.Commit()
}

func (s *Datastore) GetDeliveredPayloads(ctx context.Context, w io.Writer, headSlot uint64, query structs.PayloadTraceQuery) error {
	var (
		key ds.Key
		err error
	)

	// TODO(l): check if that one is even needed (probably not)
	if query.HasSlot() {
		key, err = s.queryToDeliveredKey(ctx, structs.PayloadQuery{Slot: query.Slot})
	} else if query.HasBlockHash() {
		key, err = s.queryToDeliveredKey(ctx, structs.PayloadQuery{BlockHash: query.BlockHash})
	} else if query.HasBlockNum() {
		key, err = s.queryToDeliveredKey(ctx, structs.PayloadQuery{BlockNum: query.BlockNum})
	} else if query.HasPubkey() {
		key, err = s.queryToDeliveredKey(ctx, structs.PayloadQuery{PubKey: query.ProposerPubkey})
	}

	if err != nil {
		if errors.Is(err, ds.ErrNotFound) {
			return json.NewEncoder(w).Encode([]structs.BidTraceExtended{})
		}
		return err
	}

	if key.String() == "" {
		start := headSlot
		if query.Cursor != 0 {
			start = min(headSlot, query.Cursor)
		}
		delivered, err := s.getlatestDelivered(ctx, start, int(query.Limit))
		if err != nil {
			return err
		}
		return json.NewEncoder(w).Encode(delivered)
	}

	data, err := s.DB.Get(ctx, key)
	if err != nil {
		if errors.Is(err, ds.ErrNotFound) {
			return json.NewEncoder(w).Encode([]structs.BidTraceExtended{})
		}
		return err
	}

	var trace structs.BidTraceWithTimestamp
	if err = json.Unmarshal(data, &trace); err != nil {
		return err
	}

	return json.NewEncoder(w).Encode([]structs.BidTraceExtended{trace.BidTraceExtended})
}

func (s *Datastore) queryToDeliveredKey(ctx context.Context, query structs.PayloadQuery) (ds.Key, error) {
	var (
		rawKey []byte
		err    error
	)

	if (query.BlockHash != types.Hash{}) {
		rawKey, err = s.DB.Get(ctx, DeliveredHashKey(query.BlockHash))
	} else if query.BlockNum != 0 {
		rawKey, err = s.DB.Get(ctx, DeliveredNumKey(query.BlockNum))
	} else if (query.PubKey != types.PublicKey{}) {
		rawKey, err = s.DB.Get(ctx, DeliveredPubkeyKey(query.PubKey))
	} else {
		rawKey = DeliveredKey(query.Slot).Bytes()
	}

	if err != nil {
		return ds.Key{}, err
	}
	return ds.NewKey(string(rawKey)), nil
}

func (s *Datastore) getlatestDelivered(ctx context.Context, start uint64, limit int) (el []structs.BidTraceExtended, err error) {
	initialSlot := start

	readr := bytes.NewReader(nil)
	dec := json.NewDecoder(readr)
	el = []structs.BidTraceExtended{}

	for {
		payload, err := s.DB.Get(ctx, DeliveredKey(structs.Slot(initialSlot)))
		if err != nil {
			if errors.Is(err, ds.ErrNotFound) {
				initialSlot--
				if start-initialSlot >= maxSlotLagPayloads {
					return el, nil
				}
				continue
			}
			return el, err
		}
		readr.Reset(payload)

		bte := structs.BidTraceWithTimestamp{}
		if err = dec.Decode(&bte); err != nil {
			return el, err
		}

		el = append(el, bte.BidTraceExtended)
		if len(el) == limit {
			return el, nil
		} else if len(el) > limit {
			return el[0:limit], nil
		}

		if start-initialSlot >= maxSlotLagPayloads {
			return el, nil
		}
		initialSlot--
	}
}

func min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}
