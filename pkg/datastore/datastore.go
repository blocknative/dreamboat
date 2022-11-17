package datastore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/dgraph-io/badger/v2"
	"github.com/flashbots/go-boost-utils/types"
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
	NewTransaction(bool) *badger.Txn
}

type Datastore struct {
	TTLStorage
	Badger

	hc *HeaderController

	PutHeadersCh chan *HRReq

	mu sync.RWMutex
}

func NewDatastore(t TTLStorage, v Badger, hc *HeaderController) *Datastore {
	return &Datastore{
		TTLStorage:   t,
		Badger:       v,
		hc:           hc,
		PutHeadersCh: make(chan *HRReq, 1),
	}
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

func (s *Datastore) GetDelivered(ctx context.Context, query structs.Query) (structs.BidTraceWithTimestamp, error) {
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

func (s *Datastore) GetDeliveredBatch(ctx context.Context, queries []structs.Query) ([]structs.BidTraceWithTimestamp, error) {
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

func (s *Datastore) PutPayload(ctx context.Context, key structs.PayloadKey, payload *structs.BlockBidAndTrace, ttl time.Duration) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.TTLStorage.PutWithTTL(ctx, PayloadKeyKey(key), data, ttl)
}

func (s *Datastore) GetPayload(ctx context.Context, key structs.PayloadKey) (*structs.BlockBidAndTrace, error) {
	data, err := s.TTLStorage.Get(ctx, PayloadKeyKey(key))
	if err != nil {
		return nil, err
	}
	var payload structs.BlockBidAndTrace
	err = json.Unmarshal(data, &payload)
	return &payload, err
}

func (s *Datastore) PutRegistration(ctx context.Context, pk structs.PubKey, registration types.SignedValidatorRegistration, ttl time.Duration) error {
	data, err := json.Marshal(registration)
	if err != nil {
		return err
	}
	return s.TTLStorage.PutWithTTL(ctx, RegistrationKey(pk), data, ttl)
}

func (s *Datastore) PutRegistrationRaw(ctx context.Context, pk structs.PubKey, registration []byte, ttl time.Duration) error {
	return s.TTLStorage.PutWithTTL(ctx, RegistrationKey(pk), registration, ttl)
}

func (s *Datastore) GetRegistration(ctx context.Context, pk structs.PubKey) (types.SignedValidatorRegistration, error) {
	data, err := s.TTLStorage.Get(ctx, RegistrationKey(pk))
	if err != nil {
		return types.SignedValidatorRegistration{}, err
	}
	var registration types.SignedValidatorRegistration
	err = json.Unmarshal(data, &registration)
	return registration, err
}

func (s *Datastore) queryToDeliveredKey(ctx context.Context, query structs.Query) (ds.Key, error) {
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

func (s *Datastore) GetAllRegistration() (map[string]types.SignedValidatorRegistration, error) {
	m := make(map[string]types.SignedValidatorRegistration)

	b := bytes.NewReader(nil)
	nDec := json.NewDecoder(b)

	err := s.Badger.View(func(txn *badger.Txn) error {
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

func ValidatorKey(pk structs.PubKey) ds.Key {
	return ds.NewKey(fmt.Sprintf("valdator-%s", pk.String()))
}

func RegistrationKey(pk structs.PubKey) ds.Key {
	return ds.NewKey(fmt.Sprintf("%s%s", RegistrationPrefix, pk.String()))
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
