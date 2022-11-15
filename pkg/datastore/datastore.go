package datastore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/dgraph-io/badger/v2"
	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
)

var errChPool = sync.Pool{
	New: func() any {
		return make(chan error, 1)
	},
}

const (
	RegistrationPrefix = "registration-"
)

type TTLStorage interface {
	PutWithTTL(context.Context, ds.Key, []byte, time.Duration) error
	Get(context.Context, ds.Key) ([]byte, error)
	GetBatch(ctx context.Context, keys []ds.Key) (batch [][]byte, err error)

	Close() error
}

type Viewer interface {
	View(func(txn *badger.Txn) error) error
	NewTransaction(bool) *badger.Txn
}

type Datastore struct {
	TTLStorage
	Viewer
	mu sync.Mutex

	PutHeadersCh chan HRReq
}

func NewDatastore(t TTLStorage, v Viewer) *Datastore {
	return &Datastore{
		TTLStorage:   t,
		Viewer:       v,
		PutHeadersCh: make(chan HRReq, 1),
	}
}

type HRReq struct {
	hr  structs.HR
	ttl time.Duration
	ret chan error
}

func (s *Datastore) PutHeaderOptimized(ctx context.Context, hr structs.HR, ttl time.Duration) error {
	errCH := errChPool.Get().(chan error)
	defer errChPool.Put(errCH)
	s.PutHeadersCh <- HRReq{
		ttl: ttl,
		hr:  hr,
		ret: errCH,
	}

	return <-errCH
}

func (s *Datastore) PutHeaderController() {
	rm := make(map[uint64]*HNTs)
	ctx := context.Background()

	for h := range s.PutHeadersCh {
		dbHeaders, ok := rm[h.hr.Trace.Slot]
		if !ok {
			hnt, err := getHNT(ctx, s, h.hr.Slot)
			if err != nil {
				h.ret <- err
				continue
			}
			dbHeaders = hnt
		}
		dbHeaders.Add(h.hr)

		err := storeHeader(s.Viewer, h.hr, dbHeaders.Serialize(), dbHeaders.SerializeMaxProfit(), h.ttl)
		if err == nil {
			rm[h.hr.Trace.Slot] = dbHeaders
		}
		h.ret <- err
	}
}

func storeHeader(s Viewer, h structs.HR, headersData, maxProfit []byte, ttl time.Duration) error {
	txn := s.NewTransaction(true)
	defer txn.Discard()

	if err := txn.SetEntry(badger.NewEntry(HeaderHashKey(h.Header.BlockHash).Bytes(), HeaderKey(h.Slot).Bytes()).WithTTL(ttl)); err != nil {
		return err
	}

	if err := txn.SetEntry(badger.NewEntry(HeaderNumKey(h.Header.BlockNumber).Bytes(), HeaderKey(h.Slot).Bytes()).WithTTL(ttl)); err != nil {
		return err
	}

	if err := txn.SetEntry(badger.NewEntry(HeaderKey(h.Slot).Bytes(), headersData).WithTTL(ttl)); err != nil {
		return err
	}

	if err := txn.SetEntry(badger.NewEntry(HeaderMaxProfitKey(h.Slot).Bytes(), maxProfit).WithTTL(ttl)); err != nil {
		return err
	}

	return txn.Commit()
}

func getHNT(ctx context.Context, s *Datastore, slot structs.Slot) (*HNTs, error) {
	dbHeaders := NewHNTs()
	data, err := s.TTLStorage.Get(ctx, HeaderKey(slot))
	if err != nil {
		if errors.Is(err, ds.ErrNotFound) {
			return dbHeaders, nil
		}
		return dbHeaders, err
	}
	if err := json.Unmarshal(data, &dbHeaders.S); err != nil {
		return dbHeaders, err
	}
	dbHeaders.LoadMaxProfit()
	return dbHeaders, nil
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

func (s *Datastore) putMaxProfitHeader(ctx context.Context, slot structs.Slot, header structs.HeaderAndTrace, ttl time.Duration) error {
	headers, err := s.getHeaders(ctx, HeaderMaxProfitKey(slot))
	if errors.Is(err, ds.ErrNotFound) {
		headers = make([]structs.HeaderAndTrace, 0, 1)
	} else if err != nil && !errors.Is(err, ds.ErrNotFound) {
		return err
	}

	// remove submission from same builder
	i := 0
	for ; i < len(headers); i++ {
		if headers[i].Trace.BuilderPubkey == header.Trace.BuilderPubkey {
			headers[i] = header
			break
		}
	}
	if i == len(headers) {
		headers = append(headers, header)
	}

	// sort by bid value DESC
	sort.Slice(headers, func(i, j int) bool {
		return headers[i].Trace.Value.Cmp(&headers[j].Trace.Value) > 0
	})

	data, err := json.Marshal(headers)
	if err != nil {
		return err
	}
	return s.TTLStorage.PutWithTTL(ctx, HeaderMaxProfitKey(slot), data, ttl)
}

func (s *Datastore) GetMaxProfitHeadersDesc(ctx context.Context, slot structs.Slot) ([]structs.HeaderAndTrace, error) {
	return s.getHeaders(ctx, HeaderMaxProfitKey(slot))
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
	data, err := json.Marshal(trace.Trace)
	if err != nil {
		return err
	}

	txn := s.Viewer.NewTransaction(true)
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

func (s *Datastore) GetAllRegistration() (map[string]types.SignedValidatorRegistration, error) {
	m := make(map[string]types.SignedValidatorRegistration)

	b := bytes.NewReader(nil)
	nDec := json.NewDecoder(b)

	err := s.Viewer.View(func(txn *badger.Txn) error {
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

func HeaderKey(slot structs.Slot) ds.Key {
	return ds.NewKey(fmt.Sprintf("header-%d", slot))
}

func HeaderMaxProfitKey(slot structs.Slot) ds.Key {
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
