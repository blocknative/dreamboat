package datastore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/blocknative/dreamboat/structs"
	"github.com/blocknative/dreamboat/structs/forks/bellatrix"
	"github.com/blocknative/dreamboat/structs/forks/capella"
	"github.com/dgraph-io/badger/v2"
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
}

func NewDatastore(t TTLStorage, db DBInter) *Datastore {
	return &Datastore{
		TTLStorage: t,
		DBInter:    db,
	}
}

func (s *Datastore) PutPayload(ctx context.Context, key structs.PayloadKey, payload structs.BlockAndTraceExtended, ttl time.Duration) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.TTLStorage.PutWithTTL(ctx, PayloadKeyKey(key), data, ttl)
}

func (s *Datastore) GetPayload(ctx context.Context, fork structs.ForkVersion, key structs.PayloadKey) (payload structs.BlockAndTraceExtended, err error) {
	data, err := s.TTLStorage.Get(ctx, PayloadKeyKey(key))
	if err != nil {
		return nil, err
	}

	switch fork {
	case structs.ForkBellatrix:
		payload = &bellatrix.BlockBidAndTrace{}
		err = json.Unmarshal(data, &payload)
	case structs.ForkCapella:
		payload = &capella.BlockAndTraceExtended{}
		err = json.Unmarshal(data, &payload)
	default:
		return payload, errors.New("unknown fork")
	}

	return payload, err
}
