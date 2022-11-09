package datastore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
)

type Datastore struct {
	TTLStorage
	mu sync.Mutex
}

type TTLStorage interface {
	PutWithTTL(context.Context, ds.Key, []byte, time.Duration) error
	Get(context.Context, ds.Key) ([]byte, error)
	GetBatch(ctx context.Context, keys []ds.Key) (batch [][]byte, err error)
	Close() error
}

func NewDatastore(t TTLStorage) *Datastore {
	return &Datastore{TTLStorage: t}
}

func (s *Datastore) PutHeader(ctx context.Context, slot structs.Slot, header structs.HeaderAndTrace, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	headers, err := s.getHeaders(ctx, HeaderKey(slot))
	if errors.Is(err, ds.ErrNotFound) {
		headers = make([]structs.HeaderAndTrace, 0, 1)
	} else if err != nil && !errors.Is(err, ds.ErrNotFound) {
		return err
	}

	if 0 < len(headers) && headers[len(headers)-1].Header.BlockHash == header.Header.BlockHash {
		return nil // deduplicate
	}

	headers = append(headers, header)

	if err := s.TTLStorage.PutWithTTL(ctx, HeaderHashKey(header.Header.BlockHash), HeaderKey(slot).Bytes(), ttl); err != nil {
		return err
	}

	if err := s.TTLStorage.PutWithTTL(ctx, HeaderNumKey(header.Header.BlockNumber), HeaderKey(slot).Bytes(), ttl); err != nil {
		return err
	}

	if err := s.putMaxProfitHeader(ctx, slot, header, ttl); err != nil {
		return fmt.Errorf("failed to set header in max profit list: %w", err)
	}

	data, err := json.Marshal(headers)
	if err != nil {
		return err
	}
	return s.TTLStorage.PutWithTTL(ctx, HeaderKey(slot), data, ttl)
}

func (s *Datastore) GetHeaders(ctx context.Context, query structs.Query) ([]structs.HeaderAndTrace, error) {
	key, err := s.queryToHeaderKey(ctx, query)
	if err != nil {
		return nil, err
	}
	headers, err := s.getHeaders(ctx, key)
	if err != nil {
		return nil, err
	}

	return s.deduplicateHeaders(headers, query), nil
}

func (s *Datastore) getHeaders(ctx context.Context, key ds.Key) ([]structs.HeaderAndTrace, error) {
	data, err := s.TTLStorage.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	return s.unsmarshalHeaders(data)
}

func (s *Datastore) deduplicateHeaders(headers []structs.HeaderAndTrace, query structs.Query) []structs.HeaderAndTrace {
	filtered := headers[:0]
	for _, header := range headers {
		if (query.BlockHash != types.Hash{}) && (query.BlockHash != header.Header.BlockHash) {
			continue
		}
		if (query.BlockNum != 0) && (query.BlockNum != header.Header.BlockNumber) {
			continue
		}
		if (query.Slot != 0) && (uint64(query.Slot) != header.Trace.Slot) {
			continue
		}
		if (query.PubKey != types.PublicKey{}) && (query.PubKey != header.Trace.ProposerPubkey) {
			continue
		}
		filtered = append(filtered, header)
	}

	return filtered
}

func (s *Datastore) PutDelivered(ctx context.Context, slot structs.Slot, trace structs.DeliveredTrace, ttl time.Duration) error {
	if err := s.TTLStorage.PutWithTTL(ctx, DeliveredHashKey(trace.Trace.BlockHash), DeliveredKey(slot).Bytes(), ttl); err != nil {
		return err
	}

	if err := s.TTLStorage.PutWithTTL(ctx, DeliveredNumKey(trace.BlockNumber), DeliveredKey(slot).Bytes(), ttl); err != nil {
		return err
	}

	if err := s.TTLStorage.PutWithTTL(ctx, DeliveredPubkeyKey(trace.Trace.ProposerPubkey), DeliveredKey(slot).Bytes(), ttl); err != nil {
		return err
	}

	data, err := json.Marshal(trace.Trace)
	if err != nil {
		return err
	}

	return s.TTLStorage.PutWithTTL(ctx, DeliveredKey(slot), data, ttl)
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

func (s *Datastore) GetHeaderBatch(ctx context.Context, queries []structs.Query) ([]structs.HeaderAndTrace, error) {
	var batch []structs.HeaderAndTrace

	for _, query := range queries {
		key, err := s.queryToHeaderKey(ctx, query)
		if err != nil {
			return nil, err
		}

		headers, err := s.getHeaders(ctx, key)
		if errors.Is(err, ds.ErrNotFound) {
			continue
		} else if err != nil {
			return nil, err
		}

		batch = append(batch, headers...)
	}

	return batch, nil
}

func (s *Datastore) unsmarshalHeaders(data []byte) ([]structs.HeaderAndTrace, error) {
	var headers []structs.HeaderAndTrace
	if err := json.Unmarshal(data, &headers); err != nil {
		var header structs.HeaderAndTrace
		if err := json.Unmarshal(data, &header); err != nil {
			return nil, err
		}
		return []structs.HeaderAndTrace{header}, nil
	}
	return headers, nil
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

func (s *Datastore) queryToHeaderKey(ctx context.Context, query structs.Query) (ds.Key, error) {
	var (
		rawKey []byte
		err    error
	)

	if (query.BlockHash != types.Hash{}) {
		rawKey, err = s.TTLStorage.Get(ctx, HeaderHashKey(query.BlockHash))
	} else if query.BlockNum != 0 {
		rawKey, err = s.TTLStorage.Get(ctx, HeaderNumKey(query.BlockNum))
	} else {
		rawKey = HeaderKey(query.Slot).Bytes()
	}

	if err != nil {
		return ds.Key{}, err
	}
	return ds.NewKey(string(rawKey)), nil
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

func HeaderKey(slot structs.Slot) ds.Key {
	return ds.NewKey(fmt.Sprintf("header-%d", slot))
}

func HeaderMaxProfitKey(slot Slot) ds.Key {
	return ds.NewKey(fmt.Sprintf("header/max-profit/%d", slot))
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
	return ds.NewKey(fmt.Sprintf("registration-%s", pk.String()))
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
