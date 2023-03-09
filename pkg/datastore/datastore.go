package datastore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/blocknative/dreamboat/pkg/structs/forks/bellatrix"
	"github.com/blocknative/dreamboat/pkg/structs/forks/capella"
	"github.com/dgraph-io/badger/v2"
	"github.com/flashbots/go-boost-utils/types"
	lru "github.com/hashicorp/golang-lru/v2"
	ds "github.com/ipfs/go-datastore"
)

type TTLStorage interface {
	PutWithTTL(context.Context, ds.Key, []byte, time.Duration) error
	Get(context.Context, ds.Key) ([]byte, error)
	GetBatch(ctx context.Context, keys []ds.Key) (batch [][]byte, err error)
	Close() error
}

type Badger interface {
	View(func(txn *badger.Txn) error) error
	Update(func(txn *badger.Txn) error) error
	NewTransaction(bool) *badger.Txn
}

type Datastore struct {
	TTLStorage
	Badger
	PayloadCache *lru.Cache[structs.PayloadKey, *structs.BlockBidAndTrace]

	hc *HeaderController
	l  sync.Mutex
}

func NewDatastore(t TTLStorage, v Badger, hc *HeaderController, payloadCacheSize int) (*Datastore, error) {
	cache, err := lru.New[structs.PayloadKey, *structs.BlockBidAndTrace](payloadCacheSize)
	if err != nil {
		return nil, err
	}

	return &Datastore{
		TTLStorage:   t,
		Badger:       v,
		hc:           hc,
		PayloadCache: cache,
	}, nil
}

func (s *Datastore) CacheBlock(ctx context.Context, key structs.PayloadKey, block *structs.CompleteBlockstruct) error {
	s.PayloadCache.Add(key, &block.Payload)
	return nil
}

func (s *Datastore) PutDelivered(ctx context.Context, slot structs.Slot, trace structs.DeliveredTrace, ttl time.Duration) error {
	data, err := json.Marshal(trace.Trace)
	if err != nil {
		return err
	}

	txn := s.Badger.NewTransaction(true)
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

func (s *Datastore) CheckSlotDelivered(ctx context.Context, slot uint64) (bool, error) {
	tx := s.Badger.NewTransaction(false)
	defer tx.Discard()

	_, err := tx.Get(DeliveredKey(structs.Slot(slot)).Bytes())
	if err == badger.ErrKeyNotFound {
		return false, nil
	}
	return (err == nil), err
}

func (s *Datastore) GetDelivered(ctx context.Context, query structs.PayloadQuery) (structs.BidTraceWithTimestamp, error) {
	key, err := s.queryToDeliveredKey(ctx, query)
	if err != nil {
		return structs.BidTraceWithTimestamp{}, err
	}
	return s.getDelivered(ctx, key)
}

func (s *Datastore) getDelivered(ctx context.Context, key ds.Key) (structs.BidTraceWithTimestamp, error) {
	data, err := s.TTLStorage.Get(ctx, key)
	if err != nil {
		return structs.BidTraceWithTimestamp{}, err
	}

	var trace structs.BidTraceWithTimestamp
	err = json.Unmarshal(data, &trace)
	return trace, err
}

func (s *Datastore) GetDeliveredBatch(ctx context.Context, queries []structs.PayloadQuery) ([]structs.BidTraceWithTimestamp, error) {
	keys := make([]ds.Key, 0, len(queries))
	for _, query := range queries {
		key, err := s.queryToDeliveredKey(ctx, query)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}

	batch, err := s.TTLStorage.GetBatch(ctx, keys)
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

func (s *Datastore) PutPayload(ctx context.Context, key structs.PayloadKey, payload structs.BlockBidAndTrace, ttl time.Duration) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.TTLStorage.PutWithTTL(ctx, PayloadKeyKey(key), data, ttl)
}

func (s *Datastore) GetPayload(ctx context.Context, fork structs.ForkVersion, key structs.PayloadKey) (payload structs.BlockBidAndTrace, cache bool, err error) {
	memPayload, ok := s.PayloadCache.Get(key)
	if ok {
		return memPayload, true, nil
	}

	data, err := s.TTLStorage.Get(ctx, PayloadKeyKey(key))
	if err != nil {
		return nil, false, err
	}

	if fork == structs.ForkBellatrix {
		payload = bellatrix.BlockBidAndTrace{}
		err = json.Unmarshal(data, &payload)
	} else if fork == structs.ForkCapella {
		payload = capella.BlockBidAndTrace{}
		err = json.Unmarshal(data, &payload)
	} else {
		return payload, false, errors.New("unknown fork")
	}

	return &payload, false, err
}

func (s *Datastore) queryToDeliveredKey(ctx context.Context, query structs.PayloadQuery) (ds.Key, error) {
	var (
		rawKey []byte
		err    error
	)

	if (query.BlockHash != types.Hash{}) {
		rawKey, err = s.TTLStorage.Get(ctx, DeliveredHashKey(query.BlockHash))
	} else if query.BlockNum != 0 {
		rawKey, err = s.TTLStorage.Get(ctx, DeliveredNumKey(query.BlockNum))
	} else if (query.PubKey != types.PublicKey{}) {
		rawKey, err = s.TTLStorage.Get(ctx, DeliveredPubkeyKey(query.PubKey))
	} else {
		rawKey = DeliveredKey(query.Slot).Bytes()
	}

	if err != nil {
		return ds.Key{}, err
	}
	return ds.NewKey(string(rawKey)), nil
}

func HeaderHashKey(bh types.Hash) ds.Key {
	return ds.NewKey(fmt.Sprintf("header-hash-%s", bh.String()))
}

func HeaderNumKey(bn uint64) ds.Key {
	return ds.NewKey(fmt.Sprintf("header-num-%d", bn))
}

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

func PayloadKeyKey(key structs.PayloadKey) ds.Key {
	return ds.NewKey(fmt.Sprintf("payload-%s-%s-%d", key.BlockHash.String(), key.Proposer.String(), key.Slot))
}

type TTLDatastoreBatcher struct {
	ds.TTLDatastore
}

func (bb *TTLDatastoreBatcher) GetBatch(ctx context.Context, keys []ds.Key) (batch [][]byte, err error) {
	for _, key := range keys {
		data, err := bb.TTLDatastore.Get(ctx, key)
		if err != nil {
			continue
		}
		batch = append(batch, data)
	}

	return
}
