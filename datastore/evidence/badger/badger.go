package badger

import (
	"context"
	"time"

	"github.com/dgraph-io/badger/v2"

	ds "github.com/ipfs/go-datastore"
)

const (
	maxSlotLagBlocks   = 50
	maxSlotLagPayloads = 350
)

type DBInter interface {
	View(func(txn *badger.Txn) error) error
	NewTransaction(bool) *badger.Txn
}

type DB interface {
	Get(context.Context, ds.Key) ([]byte, error)
}

type Datastore struct {
	DB
	DBInter

	TTL time.Duration
}

func NewDatastore(t DB, d DBInter, ttl time.Duration) *Datastore {
	return &Datastore{
		DB:      t,
		DBInter: d,
		TTL:     ttl,
	}
}
