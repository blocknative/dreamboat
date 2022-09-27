//go:generate mockgen -source=datastore.go -destination=../internal/mock/pkg/datastore.go -package=mock_relay
package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
)

type HeaderAndTrace struct {
	Header *types.ExecutionPayloadHeader
	Trace  *BidTraceWithTimestamp
}

type BlockBidAndTrace struct {
	Trace   *types.SignedBidTrace
	Bid     *types.GetHeaderResponse
	Payload *types.GetPayloadResponse
}

type BidTraceWithTimestamp struct {
	types.BidTrace
	Timestamp uint64 `json:"timestamp,string"`
}

type Query struct {
	Slot      Slot
	BlockHash types.Hash
	BlockNum  uint64
	PubKey    types.PublicKey
}

type PayloadKey struct {
	BlockHash types.Hash
	Proposer  types.PublicKey
	Slot      Slot
}

type DeliveredTrace struct {
	Trace       BidTraceWithTimestamp
	BlockNumber uint64
}

type Datastore interface {
	PutHeader(context.Context, Slot, HeaderAndTrace, time.Duration) error
	GetHeader(context.Context, Query) (HeaderAndTrace, error)
	GetHeaderBatch(context.Context, []Query) ([]HeaderAndTrace, error)
	PutDelivered(context.Context, Slot, DeliveredTrace, time.Duration) error
	GetDelivered(context.Context, Query) (BidTraceWithTimestamp, error)
	GetDeliveredBatch(context.Context, []Query) ([]BidTraceWithTimestamp, error)
	PutPayload(context.Context, PayloadKey, *BlockBidAndTrace, time.Duration) error
	GetPayload(context.Context, PayloadKey) (*BlockBidAndTrace, error)
	PutRegistration(context.Context, PubKey, types.SignedValidatorRegistration, time.Duration) error
	GetRegistration(context.Context, PubKey) (types.SignedValidatorRegistration, error)
}

type DefaultDatastore struct {
	Storage TTLStorage
}

type TTLStorage interface {
	PutWithTTL(context.Context, ds.Key, []byte, time.Duration) error
	Get(context.Context, ds.Key) ([]byte, error)
	GetBatch(ctx context.Context, keys []ds.Key) (batch [][]byte, err error)
	Close() error
}

func (s DefaultDatastore) PutHeader(ctx context.Context, slot Slot, header HeaderAndTrace, ttl time.Duration) error {
	if err := s.Storage.PutWithTTL(ctx, HeaderHashKey(header.Header.BlockHash), HeaderKey(slot).Bytes(), ttl); err != nil {
		return err
	}

	if err := s.Storage.PutWithTTL(ctx, HeaderNumKey(header.Header.BlockNumber), HeaderKey(slot).Bytes(), ttl); err != nil {
		return err
	}

	data, err := json.Marshal(header)
	if err != nil {
		return err
	}
	return s.Storage.PutWithTTL(ctx, HeaderKey(slot), data, ttl)
}

func (s DefaultDatastore) GetHeader(ctx context.Context, query Query) (HeaderAndTrace, error) {
	key, err := s.queryToHeaderKey(ctx, query)
	if err != nil {
		return HeaderAndTrace{}, err
	}
	return s.getHeader(ctx, key)
}

func (s DefaultDatastore) getHeader(ctx context.Context, key ds.Key) (HeaderAndTrace, error) {
	data, err := s.Storage.Get(ctx, key)
	if err != nil {
		return HeaderAndTrace{}, err
	}

	var trace HeaderAndTrace
	err = json.Unmarshal(data, &trace)
	return trace, err
}

func (s DefaultDatastore) PutDelivered(ctx context.Context, slot Slot, trace DeliveredTrace, ttl time.Duration) error {
	if err := s.Storage.PutWithTTL(ctx, DeliveredHashKey(trace.Trace.BlockHash), DeliveredKey(slot).Bytes(), ttl); err != nil {
		return err
	}

	if err := s.Storage.PutWithTTL(ctx, DeliveredNumKey(trace.BlockNumber), DeliveredKey(slot).Bytes(), ttl); err != nil {
		return err
	}

	if err := s.Storage.PutWithTTL(ctx, DeliveredPubkeyKey(trace.Trace.ProposerPubkey), DeliveredKey(slot).Bytes(), ttl); err != nil {
		return err
	}

	data, err := json.Marshal(trace.Trace)
	if err != nil {
		return err
	}

	return s.Storage.PutWithTTL(ctx, DeliveredKey(slot), data, ttl)
}

func (s DefaultDatastore) GetDelivered(ctx context.Context, query Query) (BidTraceWithTimestamp, error) {
	key, err := s.queryToDeliveredKey(ctx, query)
	if err != nil {
		return BidTraceWithTimestamp{}, err
	}
	return s.getDelivered(ctx, key)
}

func (s DefaultDatastore) getDelivered(ctx context.Context, key ds.Key) (BidTraceWithTimestamp, error) {
	data, err := s.Storage.Get(ctx, key)
	if err != nil {
		return BidTraceWithTimestamp{}, err
	}

	var trace BidTraceWithTimestamp
	err = json.Unmarshal(data, &trace)
	return trace, err
}

func (s DefaultDatastore) GetDeliveredBatch(ctx context.Context, queries []Query) ([]BidTraceWithTimestamp, error) {
	keys := make([]ds.Key, 0, len(queries))
	for _, query := range queries {
		key, err := s.queryToDeliveredKey(ctx, query)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}

	batch, err := s.Storage.GetBatch(ctx, keys)
	if err != nil {
		return nil, err
	}

	traceBatch := make([]BidTraceWithTimestamp, 0, len(batch))
	for _, data := range batch {
		var trace BidTraceWithTimestamp
		if err = json.Unmarshal(data, &trace); err != nil {
			return nil, err
		}
		traceBatch = append(traceBatch, trace)
	}

	return traceBatch, err
}

func (s DefaultDatastore) GetHeaderBatch(ctx context.Context, queries []Query) ([]HeaderAndTrace, error) {
	keys := make([]ds.Key, 0, len(queries))
	for _, query := range queries {
		key, err := s.queryToHeaderKey(ctx, query)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}

	batch, err := s.Storage.GetBatch(ctx, keys)
	if err != nil {
		return nil, err
	}

	headerBatch := make([]HeaderAndTrace, 0, len(batch))
	for _, data := range batch {
		var header HeaderAndTrace
		if err = json.Unmarshal(data, &header); err != nil {
			return nil, err
		}
		headerBatch = append(headerBatch, header)
	}

	return headerBatch, err
}

func (s DefaultDatastore) PutPayload(ctx context.Context, key PayloadKey, payload *BlockBidAndTrace, ttl time.Duration) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.Storage.PutWithTTL(ctx, PayloadKeyKey(key), data, ttl)
}

func (s DefaultDatastore) GetPayload(ctx context.Context, key PayloadKey) (*BlockBidAndTrace, error) {
	data, err := s.Storage.Get(ctx, PayloadKeyKey(key))
	if err != nil {
		return nil, err
	}
	var payload BlockBidAndTrace
	err = json.Unmarshal(data, &payload)
	return &payload, err
}

func (s DefaultDatastore) PutRegistration(ctx context.Context, pk PubKey, registration types.SignedValidatorRegistration, ttl time.Duration) error {
	data, err := json.Marshal(registration)
	if err != nil {
		return err
	}
	return s.Storage.PutWithTTL(ctx, RegistrationKey(pk), data, ttl)
}

func (s DefaultDatastore) GetRegistration(ctx context.Context, pk PubKey) (types.SignedValidatorRegistration, error) {
	data, err := s.Storage.Get(ctx, RegistrationKey(pk))
	if err != nil {
		return types.SignedValidatorRegistration{}, err
	}
	var registration types.SignedValidatorRegistration
	err = json.Unmarshal(data, &registration)
	return registration, err
}

func (s DefaultDatastore) queryToHeaderKey(ctx context.Context, query Query) (ds.Key, error) {
	var (
		rawKey []byte
		err    error
	)

	if (query.BlockHash != types.Hash{}) {
		rawKey, err = s.Storage.Get(ctx, HeaderHashKey(query.BlockHash))
	} else if query.BlockNum != 0 {
		rawKey, err = s.Storage.Get(ctx, HeaderNumKey(query.BlockNum))
	} else {
		rawKey = HeaderKey(query.Slot).Bytes()
	}

	if err != nil {
		return ds.Key{}, err
	}
	return ds.NewKey(string(rawKey)), nil
}

func (s DefaultDatastore) queryToDeliveredKey(ctx context.Context, query Query) (ds.Key, error) {
	var (
		rawKey []byte
		err    error
	)

	if (query.BlockHash != types.Hash{}) {
		rawKey, err = s.Storage.Get(ctx, DeliveredHashKey(query.BlockHash))
	} else if query.BlockNum != 0 {
		rawKey, err = s.Storage.Get(ctx, DeliveredNumKey(query.BlockNum))
	} else if (query.PubKey != types.PublicKey{}) {
		rawKey, err = s.Storage.Get(ctx, DeliveredPubkeyKey(query.PubKey))
	} else {
		rawKey = DeliveredKey(query.Slot).Bytes()
	}

	if err != nil {
		return ds.Key{}, err
	}
	return ds.NewKey(string(rawKey)), nil
}

func HeaderKey(slot Slot) ds.Key {
	return ds.NewKey(fmt.Sprintf("header-%d", slot))
}

func HeaderHashKey(bh types.Hash) ds.Key {
	return ds.NewKey(fmt.Sprintf("header-hash-%s", bh.String()))
}

func HeaderNumKey(bn uint64) ds.Key {
	return ds.NewKey(fmt.Sprintf("header-num-%d", bn))
}

func DeliveredKey(slot Slot) ds.Key {
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

func PayloadKeyKey(key PayloadKey) ds.Key {
	return ds.NewKey(fmt.Sprintf("payload-%s-%s-%d", key.BlockHash.String(), key.Proposer.String(), key.Slot))
}

func ValidatorKey(pk PubKey) ds.Key {
	return ds.NewKey(fmt.Sprintf("valdator-%s", pk.String()))
}

func RegistrationKey(pk PubKey) ds.Key {
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
