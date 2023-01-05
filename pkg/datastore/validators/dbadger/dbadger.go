package dbadger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/dgraph-io/badger/v2"
	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
)

func RegistrationKey(pk structs.PubKey) ds.Key {
	return ds.NewKey(fmt.Sprintf("%s%s", RegistrationPrefix, pk.String()))
}

const (
	RegistrationPrefix = "registration-"
)

type DB interface {
	View(func(txn *badger.Txn) error) error
	PutWithTTL(context.Context, ds.Key, []byte, time.Duration) error
	Get(context.Context, ds.Key) ([]byte, error)
}

type Datastore struct {
	DB
}

func NewDatastore(t DB) (*Datastore, error) {
	return &Datastore{
		DB: t,
	}, nil
}

func (s *Datastore) PutRegistration(ctx context.Context, pk structs.PubKey, registration types.SignedValidatorRegistration, ttl time.Duration) error {
	data, err := json.Marshal(registration)
	if err != nil {
		return err
	}
	return s.DB.PutWithTTL(ctx, RegistrationKey(pk), data, ttl)
}

func (s *Datastore) PutRegistrationRaw(ctx context.Context, pk structs.PubKey, registration []byte, ttl time.Duration) error {
	return s.DB.PutWithTTL(ctx, RegistrationKey(pk), registration, ttl)
}

func (s *Datastore) GetRegistration(ctx context.Context, pk structs.PubKey) (types.SignedValidatorRegistration, error) {
	data, err := s.DB.Get(ctx, RegistrationKey(pk))
	if err != nil {
		return types.SignedValidatorRegistration{}, err
	}
	var registration types.SignedValidatorRegistration
	err = json.Unmarshal(data, &registration)
	return registration, err
}

func (s *Datastore) GetAllRegistration() (map[string]types.SignedValidatorRegistration, error) {
	m := make(map[string]types.SignedValidatorRegistration)

	b := bytes.NewReader(nil)
	nDec := json.NewDecoder(b)

	err := s.DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte("/" + RegistrationPrefix)

		lenP := len(RegistrationPrefix) + 1
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			k := item.Key()

			err := item.Value(func(v []byte) error {
				b.Reset(v)
				sgr := types.SignedValidatorRegistration{}
				if err := nDec.Decode(&sgr); err != nil {
					return err
				}
				m[string(k)[lenP:]] = sgr
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	return m, err
}
