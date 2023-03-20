package badger

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/blocknative/dreamboat/structs"
	"github.com/dgraph-io/badger/v2"
	"github.com/flashbots/go-boost-utils/types"
	"golang.org/x/exp/constraints"

	ds "github.com/ipfs/go-datastore"
)

var (
	DurationPerSlot = time.Second * 12
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

func (s *Datastore) PutDelivered(ctx context.Context, slot structs.Slot, trace structs.DeliveredTrace, ttl time.Duration) error {
	data, err := json.Marshal(trace.Trace)
	if err != nil {
		return err
	}

	txn := s.DBInter.NewTransaction(true)
	defer txn.Discard()
	if err := txn.SetEntry(badger.NewEntry(DeliveredHashKey(trace.Trace.BlockHash).Bytes(), DeliveredKey(slot).Bytes()).WithTTL(ttl)); err != nil {
		return err
	}
	if err := txn.SetEntry(badger.NewEntry(DeliveredNumKey(trace.BlockNumber).Bytes(), DeliveredKey(slot).Bytes()).WithTTL(ttl)); err != nil {
		return err
	}
	if err := txn.SetEntry(badger.NewEntry(DeliveredPubkeyKey(trace.Trace.ProposerPubkey).Bytes(), DeliveredKey(slot).Bytes()).WithTTL(ttl)); err != nil {
		return err
	}
	if err := txn.SetEntry(badger.NewEntry(DeliveredKey(slot).Bytes(), data).WithTTL(ttl)); err != nil {
		return err
	}

	return txn.Commit()
}

/*
func (s *Datastore) CheckSlotDelivered(ctx context.Context, slot uint64) (bool, error) {
	tx := s.DBInter.NewTransaction(false)
	defer tx.Discard()

	_, err := tx.Get(DeliveredKey(structs.Slot(slot)).Bytes())
	if err == badger.ErrKeyNotFound {
		return false, nil
	}
	return (err == nil), err
}
*/

func (s *Datastore) GetDeliveredPayloads(ctx context.Context, headSlot uint64, query structs.PayloadTraceQuery) ([]structs.BidTraceExtended, error) {
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
		key, err = s.queryToDeliveredKey(ctx, structs.PayloadQuery{PubKey: query.Pubkey})
	}

	if err != nil {
		return nil, err
	}
	if key.String() == "" {
		start := headSlot
		if query.Cursor != 0 {
			start = min(headSlot, query.Cursor)
		}
		return s.getTailDelivered(ctx, start, query.Limit)
	}

	data, err := s.DB.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var trace structs.BidTraceWithTimestamp
	err = json.Unmarshal(data, &trace)

	return []structs.BidTraceExtended{trace.BidTraceExtended}, err
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

func (s *Datastore) getTailDelivered(ctx context.Context, start, limit uint64) ([]structs.BidTraceExtended, error) {

	stop := start - min(uint64(s.TTL/DurationPerSlot), start)
	batch := make([]structs.BidTraceWithTimestamp, 0, limit)

	queries := make([]uint64, 0, limit)

	for highSlot := start; len(batch) < int(limit) && stop <= highSlot; highSlot -= min(limit, highSlot) {
		queries = queries[:0]
		for s := highSlot; highSlot-limit < s && stop <= s; s-- {
			queries = append(queries, s)
		}

		nextBatch, err := s.GetDeliveredBatch(ctx, queries)
		if err != nil {
			// r.l.WithError(err).Warn("failed getting header batch")
			continue
		}

		batch = append(batch, nextBatch[:min(int(limit)-len(batch), len(nextBatch))]...)
	}

	events := make([]structs.BidTraceExtended, 0, len(batch))
	for _, event := range batch {
		events = append(events, event.BidTraceExtended)
	}
	return events, nil
}

func (s *Datastore) GetDeliveredBatch(ctx context.Context, queries []uint64) ([]structs.BidTraceWithTimestamp, error) {
	keys := make([]ds.Key, 0, len(queries))
	for _, query := range queries {
		keys = append(keys, ds.NewKey(string(DeliveredKey(structs.Slot(query)).Bytes())))
	}

	var (
		batch [][]byte
		err   error
	)
	for _, key := range keys {
		data, err := s.DB.Get(ctx, key)
		if err != nil {
			continue
		}
		batch = append(batch, data)
	}
	if err != nil {
		return nil, err
	}

	traceBatch := make([]structs.BidTraceWithTimestamp, 0, len(batch))
	for _, data := range batch {
		var trace structs.BidTraceWithTimestamp
		if err = json.Unmarshal(data, &trace); err != nil {
			return nil, err
		}
		traceBatch = append(traceBatch, trace)
	}

	return traceBatch, err
}

func min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}
