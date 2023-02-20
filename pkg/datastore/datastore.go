package datastore

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/dgraph-io/badger/v2"
	lru "github.com/hashicorp/golang-lru/v2"
	ds "github.com/ipfs/go-datastore"
)

type TTLStorage interface {
	PutWithTTL(context.Context, ds.Key, []byte, time.Duration) error
	Get(context.Context, ds.Key) ([]byte, error)
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

func (s *Datastore) CacheBlock(ctx context.Context, block *structs.CompleteBlockstruct) error {
	key := structs.PayloadKey{BlockHash: block.Payload.Payload.Data.BlockHash, Slot: structs.Slot(block.Payload.Trace.Message.Slot), Proposer: block.Payload.Trace.Message.ProposerPubkey}
	s.PayloadCache.Add(key, &block.Payload)
	return nil
}

func (s *Datastore) PutPayload(ctx context.Context, key structs.PayloadKey, payload *structs.BlockBidAndTrace, ttl time.Duration) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.TTLStorage.PutWithTTL(ctx, PayloadKeyKey(key), data, ttl)
}

func (s *Datastore) GetPayload(ctx context.Context, key structs.PayloadKey) (*structs.BlockBidAndTrace, bool, error) {
	memPayload, ok := s.PayloadCache.Get(key)
	if ok {
		return memPayload, true, nil
	}

	data, err := s.TTLStorage.Get(ctx, PayloadKeyKey(key))
	if err != nil {
		return nil, false, err
	}
	var payload structs.BlockBidAndTrace
	err = json.Unmarshal(data, &payload)
	return &payload, false, err
}

func PayloadKeyKey(key structs.PayloadKey) ds.Key {
	return ds.NewKey(fmt.Sprintf("payload-%s-%s-%d", key.BlockHash.String(), key.Proposer.String(), key.Slot))
}
