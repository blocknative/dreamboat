package datastore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/blocknative/dreamboat/structs"
	"github.com/blocknative/dreamboat/structs/forks/bellatrix"
	"github.com/blocknative/dreamboat/structs/forks/capella"
	"github.com/dgraph-io/badger/v2"
	lru "github.com/hashicorp/golang-lru/v2"
	ds "github.com/ipfs/go-datastore"
)

var ErrNotFound = errors.New("not found")

type DBInter interface {
	View(func(txn *badger.Txn) error) error
	NewTransaction(bool) *badger.Txn
}

type TTLStorage interface {
	PutWithTTL(context.Context, ds.Key, []byte, time.Duration) error
	Get(context.Context, ds.Key) ([]byte, error)
}

func PayloadKeyKey(key structs.PayloadKey) ds.Key {
	return ds.NewKey(fmt.Sprintf("payload-%s-%s-%d", key.BlockHash.String(), key.Proposer.String(), key.Slot))
}

type Datastore struct {
	TTLStorage
	DBInter
	PayloadCache *lru.Cache[structs.PayloadKey, structs.BlockBidAndTrace]
}

func NewDatastore(t TTLStorage, db DBInter, payloadCacheSize int) (*Datastore, error) {
	cache, err := lru.New[structs.PayloadKey, structs.BlockBidAndTrace](payloadCacheSize)
	if err != nil {
		return nil, err
	}

	return &Datastore{
		TTLStorage:   t,
		PayloadCache: cache,
		DBInter:      db,
	}, nil
}

func (s *Datastore) CacheBlock(ctx context.Context, key structs.PayloadKey, block *structs.CompleteBlockstruct) error {
	s.PayloadCache.Add(key, block.Payload)
	return nil
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

	switch fork {
	case structs.ForkBellatrix:
		payload = &bellatrix.BlockBidAndTrace{}
		err = json.Unmarshal(data, &payload)
	case structs.ForkCapella:
		payload = &capella.BlockBidAndTrace{}
		err = json.Unmarshal(data, &payload)
	default:
		return payload, false, errors.New("unknown fork")
	}

	return payload, false, err
}

func (s *Datastore) GetSlotRawPayload(ctx context.Context, key structs.PayloadKey) (output [][]byte, err error) {
	if key.Slot == 0 {
		return nil, errors.New("wrong slot number")
	}

	err = s.DBInter.View(func(txn *badger.Txn) error {
		op := badger.DefaultIteratorOptions
		op.PrefetchValues = false

		it := txn.NewIterator(op)
		defer it.Close()

		prefix := []byte("/payload-")
		var suffix []byte
		if key.BlockHash != [32]byte{} && key.Proposer != [48]byte{} {
			suffix = []byte(fmt.Sprintf("%s-%s-%d", key.BlockHash.String(), key.Proposer.String(), key.Slot))
		} else if key.Proposer != [48]byte{} {
			suffix = []byte(fmt.Sprintf("%s-%d", key.Proposer.String(), key.Slot))
		} else {
			suffix = []byte(fmt.Sprintf("%d", key.Slot))
		}

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			if !bytes.HasSuffix(item.Key(), suffix) {
				continue
			}

			c, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			output = append(output, c)
			output = append(output, []byte("\n"))
		}
		return nil
	})

	return output, err
}
