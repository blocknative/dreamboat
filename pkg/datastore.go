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

type Datastore interface {
	PutHeader(context.Context, Slot, HeaderAndTrace, time.Duration) error
	PutDelivered(context.Context, Slot, time.Duration) error
	GetHeader(context.Context, Slot, bool) (HeaderAndTrace, error)
	GetHeaderBatch(context.Context, []Slot, bool) ([]HeaderAndTrace, error)
	GetHeaderByBlockHash(context.Context, types.Hash, bool) (HeaderAndTrace, error)
	GetHeaderByBlockNum(context.Context, uint64, bool) (HeaderAndTrace, error)
	GetHeaderByPubkey(context.Context, types.PublicKey, bool) (HeaderAndTrace, error)
	PutPayload(context.Context, types.Hash, *BlockBidAndTrace, time.Duration) error
	GetPayload(context.Context, types.Hash) (*BlockBidAndTrace, error)
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
	if err := s.Storage.PutWithTTL(ctx, HeaderHashKey(header.Trace.BlockHash), HeaderKey(slot).Bytes(), ttl); err != nil {
		return err
	}

	if err := s.Storage.PutWithTTL(ctx, HeaderNumKey(header.Header.BlockNumber), HeaderKey(slot).Bytes(), ttl); err != nil {
		return err
	}

	if err := s.Storage.PutWithTTL(ctx, HeaderPubkeyKey(header.Trace.ProposerPubkey), HeaderKey(slot).Bytes(), ttl); err != nil {
		return err
	}

	data, err := json.Marshal(header)
	if err != nil {
		return err
	}
	return s.Storage.PutWithTTL(ctx, HeaderKey(slot), data, ttl)
}

func (s DefaultDatastore) PutDelivered(ctx context.Context, slot Slot, ttl time.Duration) error {
	return s.Storage.PutWithTTL(ctx, DeliveredKey(HeaderKey(slot)), []byte{0}, ttl)
}

func (s DefaultDatastore) GetHeader(ctx context.Context, slot Slot, delivered bool) (HeaderAndTrace, error) {
	return s.getHeader(ctx, HeaderKey(slot), delivered)
}

func (s DefaultDatastore) GetHeaderByBlockHash(ctx context.Context, bh types.Hash, delivered bool) (HeaderAndTrace, error) {
	rawKey, err := s.Storage.Get(ctx, HeaderHashKey(bh))
	if err != nil {
		return HeaderAndTrace{}, err
	}
	return s.getHeader(ctx, ds.NewKey(string(rawKey)), delivered)
}

func (s DefaultDatastore) GetHeaderByBlockNum(ctx context.Context, bn uint64, delivered bool) (HeaderAndTrace, error) {
	rawKey, err := s.Storage.Get(ctx, HeaderNumKey(bn))
	if err != nil {
		return HeaderAndTrace{}, err
	}
	return s.getHeader(ctx, ds.NewKey(string(rawKey)), delivered)
}

func (s DefaultDatastore) GetHeaderByPubkey(ctx context.Context, pk types.PublicKey, delivered bool) (HeaderAndTrace, error) {
	rawKey, err := s.Storage.Get(ctx, HeaderPubkeyKey(pk))
	if err != nil {
		return HeaderAndTrace{}, err
	}
	return s.getHeader(ctx, ds.NewKey(string(rawKey)), delivered)
}

func (s DefaultDatastore) getHeader(ctx context.Context, key ds.Key, delivered bool) (HeaderAndTrace, error) {
	if delivered && !s.isDelivered(ctx, key) {
		return HeaderAndTrace{}, ds.ErrNotFound
	}

	data, err := s.Storage.Get(ctx, key)
	if err != nil {
		return HeaderAndTrace{}, err
	}
	var header HeaderAndTrace
	err = json.Unmarshal(data, &header)
	return header, err
}

func (s DefaultDatastore) isDelivered(ctx context.Context, key ds.Key) bool {
	_, err := s.Storage.Get(ctx, DeliveredKey(key))
	return err == nil
}

func (s DefaultDatastore) GetHeaderBatch(ctx context.Context, slots []Slot, delivered bool) ([]HeaderAndTrace, error) {
	keys := make([]ds.Key, 0, len(slots))
	for _, slot := range slots {
		if delivered && !s.isDelivered(ctx, HeaderKey(slot)) {
			continue
		}
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

func (s DefaultDatastore) PutPayload(ctx context.Context, h types.Hash, payload *BlockBidAndTrace, ttl time.Duration) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.Storage.PutWithTTL(ctx, PayloadKey(h), data, ttl)
}

func (s DefaultDatastore) GetPayload(ctx context.Context, h types.Hash) (*BlockBidAndTrace, error) {
	data, err := s.Storage.Get(ctx, PayloadKey(h))
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

func DeliveredKey(key ds.Key) ds.Key {
	return ds.NewKey(fmt.Sprintf("delivered-%s", key.String()))
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

func HeaderPubkeyKey(pk types.PublicKey) ds.Key {
	return ds.NewKey(fmt.Sprintf("header-pk-%s", pk.String()))
}

func PayloadKey(h types.Hash) ds.Key {
	return ds.NewKey(fmt.Sprintf("payload-%s", h.String()))
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
