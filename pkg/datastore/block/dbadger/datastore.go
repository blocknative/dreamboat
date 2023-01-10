package dbadger

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/blocknative/dreamboat/pkg/datastore/block/headerscontroller"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	lru "github.com/hashicorp/golang-lru/v2"
	ds "github.com/ipfs/go-datastore"
)

/*
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
}*/

type DB interface {
	PutWithTTL(context.Context, ds.Key, []byte, time.Duration) error
	Get(context.Context, ds.Key) ([]byte, error)
	GetBatch(ctx context.Context, keys []ds.Key) (batch [][]byte, err error)
}

type Datastore struct {
	DB
	PayloadCache *lru.Cache[structs.PayloadKey, *structs.BlockBidAndTrace]

	hc *headerscontroller.HeaderController
	l  sync.Mutex
}

func NewDatastore(t DB, hc *headerscontroller.HeaderController, payloadCacheSize int) (*Datastore, error) {
	cache, err := lru.New[structs.PayloadKey, *structs.BlockBidAndTrace](payloadCacheSize)
	if err != nil {
		return nil, err
	}

	return &Datastore{
		DB:           t,
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
	return s.DB.PutWithTTL(ctx, PayloadKeyKey(key), data, ttl)
}

func (s *Datastore) GetPayload(ctx context.Context, key structs.PayloadKey) (*structs.BlockBidAndTrace, bool, error) {
	memPayload, ok := s.PayloadCache.Get(key)
	if ok {
		return memPayload, true, nil
	}

	data, err := s.DB.Get(ctx, PayloadKeyKey(key))
	if err != nil {
		return nil, false, err
	}
	var payload structs.BlockBidAndTrace
	err = json.Unmarshal(data, &payload)
	return &payload, false, err
}

func HeaderHashKey(bh types.Hash) ds.Key {
	return ds.NewKey(fmt.Sprintf("header-hash-%s", bh.String()))
}

func HeaderNumKey(bn uint64) ds.Key {
	return ds.NewKey(fmt.Sprintf("header-num-%d", bn))
}

func PayloadKeyKey(key structs.PayloadKey) ds.Key {
	return ds.NewKey(fmt.Sprintf("payload-%s-%s-%d", key.BlockHash.String(), key.Proposer.String(), key.Slot))
}

/*
func ValidatorKey(pk structs.PubKey) ds.Key {
	return ds.NewKey(fmt.Sprintf("valdator-%s", pk.String()))
}*/
