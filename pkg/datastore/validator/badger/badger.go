package badger

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/blocknative/dreamboat/pkg/datastore"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
)

func RegistrationKey(pk types.PublicKey) ds.Key {
	return ds.NewKey(fmt.Sprintf("%s%s", "registration-", pk.String()))
}

func RegistrationTimeKey(pk types.PublicKey) ds.Key {
	return ds.NewKey(fmt.Sprintf("%s%s", "reg-t-", pk.String()))
}

type DB interface {
	Get(context.Context, ds.Key) ([]byte, error)
	PutWithTTL(context.Context, ds.Key, []byte, time.Duration) error
}

type Datastore struct {
	DB
	l   sync.Mutex
	TTL time.Duration
}

func NewDatastore(t DB, ttl time.Duration) *Datastore {
	return &Datastore{
		DB:  t,
		TTL: ttl,
	}
}

func (s *Datastore) PutNewerRegistration(ctx context.Context, pk types.PublicKey, registration types.SignedValidatorRegistration) error {
	data, err := json.Marshal(registration)
	if err != nil {
		return err
	}

	s.l.Lock()
	defer s.l.Unlock()
	// check for newer in database
	time, err := s.DB.Get(ctx, RegistrationTimeKey(pk))
	if err == nil && len(time) == 8 {
		if registration.Message.Timestamp < binary.LittleEndian.Uint64(time) {
			return nil // already have newer
		}
	}

	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, registration.Message.Timestamp)

	// transaction maybe ?
	if err := s.DB.PutWithTTL(ctx, RegistrationTimeKey(pk), b, s.TTL); err != nil {
		return err
	}

	return s.DB.PutWithTTL(ctx, RegistrationKey(pk), data, s.TTL)
}

func (s *Datastore) GetRegistration(ctx context.Context, pk types.PublicKey) (svr types.SignedValidatorRegistration, err error) {
	data, err := s.DB.Get(ctx, RegistrationKey(pk))
	if err != nil {
		if err == ds.ErrNotFound {
			return svr, datastore.ErrNotFound
		}
		return svr, err
	}
	svr = types.SignedValidatorRegistration{}
	err = json.Unmarshal(data, &svr)
	return svr, err
}

func (s *Datastore) PopulateAllRegistrations(ctx context.Context, out chan structs.ValidatorCacheEntry) error {
	// NOOP - to be supported
	return nil
}
