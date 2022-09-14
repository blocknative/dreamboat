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
	GetHeader(context.Context, Slot) (HeaderAndTrace, error)
	GetHeaderByBlockHash(context.Context, types.Hash) (HeaderAndTrace, error)
	GetHeaderByBlockNum(context.Context, uint64) (HeaderAndTrace, error)
	GetHeaderBatch(context.Context, []Slot) ([]HeaderAndTrace, error)
	PutDelivered(context.Context, Slot, DeliveredTrace, time.Duration) error
	GetDelivered(context.Context, Slot) (BidTraceWithTimestamp, error)
	GetDeliveredBatch(context.Context, []Slot) ([]BidTraceWithTimestamp, error)
	GetDeliveredByBlockHash(context.Context, types.Hash) (BidTraceWithTimestamp, error)
	GetDeliveredByBlockNum(context.Context, uint64) (BidTraceWithTimestamp, error)
	GetDeliveredByPubkey(context.Context, types.PublicKey) (BidTraceWithTimestamp, error)
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

func (s DefaultDatastore) GetHeader(ctx context.Context, slot Slot) (HeaderAndTrace, error) {
	data, err := s.Storage.Get(ctx, HeaderKey(slot))
	if err != nil {
		return HeaderAndTrace{}, err
	}

	var trace HeaderAndTrace
	err = json.Unmarshal(data, &trace)
	return trace, err
}

func (s DefaultDatastore) GetHeaderByBlockHash(ctx context.Context, bh types.Hash) (HeaderAndTrace, error) {
	rawKey, err := s.Storage.Get(ctx, HeaderHashKey(bh))
	if err != nil {
		return HeaderAndTrace{}, err
	}

	data, err := s.Storage.Get(ctx, ds.NewKey(string(rawKey)))
	if err != nil {
		return HeaderAndTrace{}, err
	}

	var trace HeaderAndTrace
	err = json.Unmarshal(data, &trace)
	return trace, err
}

func (s DefaultDatastore) GetHeaderByBlockNum(ctx context.Context, bn uint64) (HeaderAndTrace, error) {
	rawKey, err := s.Storage.Get(ctx, HeaderNumKey(bn))
	if err != nil {
		return HeaderAndTrace{}, err
	}

	data, err := s.Storage.Get(ctx, ds.NewKey(string(rawKey)))
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

func (s DefaultDatastore) GetDelivered(ctx context.Context, slot Slot) (BidTraceWithTimestamp, error) {
	return s.getDelivered(ctx, DeliveredKey(slot))
}

func (s DefaultDatastore) GetDeliveredByBlockHash(ctx context.Context, bh types.Hash) (BidTraceWithTimestamp, error) {
	rawKey, err := s.Storage.Get(ctx, DeliveredHashKey(bh))
	if err != nil {
		return BidTraceWithTimestamp{}, err
	}
	return s.getDelivered(ctx, ds.NewKey(string(rawKey)))
}

func (s DefaultDatastore) GetDeliveredByBlockNum(ctx context.Context, bn uint64) (BidTraceWithTimestamp, error) {
	rawKey, err := s.Storage.Get(ctx, DeliveredNumKey(bn))
	if err != nil {
		return BidTraceWithTimestamp{}, err
	}
	return s.getDelivered(ctx, ds.NewKey(string(rawKey)))
}

func (s DefaultDatastore) GetDeliveredByPubkey(ctx context.Context, pk types.PublicKey) (BidTraceWithTimestamp, error) {
	rawKey, err := s.Storage.Get(ctx, DeliveredPubkeyKey(pk))
	if err != nil {
		return BidTraceWithTimestamp{}, err
	}
	return s.getDelivered(ctx, ds.NewKey(string(rawKey)))
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

func (s DefaultDatastore) GetDeliveredBatch(ctx context.Context, slots []Slot) ([]BidTraceWithTimestamp, error) {
	keys := make([]ds.Key, 0, len(slots))
	for _, slot := range slots {
		keys = append(keys, DeliveredKey(slot))
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

func (s DefaultDatastore) GetHeaderBatch(ctx context.Context, slots []Slot) ([]HeaderAndTrace, error) {
	keys := make([]ds.Key, 0, len(slots))
	for _, slot := range slots {
		keys = append(keys, HeaderKey(slot))
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
