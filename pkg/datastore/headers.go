package datastore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/dgraph-io/badger/v2"
	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
)

const (
	RegistrationPrefix = "registration-"
)

// NEW Indexes
func HeaderNewKey(slot structs.Slot) ds.Key {
	return ds.NewKey(fmt.Sprintf("hi/%d", slot))
}

func HeaderKeyContent(slot structs.Slot, bh types.Hash) ds.Key {
	return ds.NewKey(fmt.Sprintf("hc/%d/%s", slot, bh.String()))
}

func HeaderKey(slot structs.Slot) ds.Key {
	return ds.NewKey(fmt.Sprintf("header-%d", slot))
}

type StoredIndex struct {
	Index     []IndexEl
	MaxProfit [32]byte
}

type IndexEl struct {
	Hash          [32]byte
	Value         [32]byte
	BuilderPubkey [48]byte
}

type HRReq struct {
	Hash          [32]byte
	Value         [32]byte
	BuilderPubkey [48]byte
	Slot          uint64
	TTL           time.Duration
	Ret           chan error
}

var errChPool = sync.Pool{
	New: func() any {
		return make(chan error, 1)
	},
}

var HRReqPool = sync.Pool{
	New: func() any {
		return &HRReq{}
	},
}

type HeaderController struct {
	map[uint64]HNT
}

func PutHeaderController(hc *HeaderController, s Datastore) {
	rm := make(map[uint64]*HNTs)
	for h := range s.PutHeadersCh {
		dbHeaders, ok := rm[h.Slot]
		if !ok {

		}
		dbHeaders.Add(IndexEl{Hash: h.Hash, Value: h.Value, BuilderPubkey: h.BuilderPubkey})
		/*
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

			err := storeHeader(s.Badger, h.hr, dbHeaders.Serialize(), dbHeaders.SerializeMaxProfit(), h.ttl)
			if err == nil {
				rm[h.hr.Trace.Slot] = dbHeaders
			}
			h.ret <- err
		*/
	}
}

func (s *Datastore) GetMaxProfitHeader(ctx context.Context, slot structs.Slot) (structs.HeaderAndTrace, error) {

	// Check memory
		s.hc.
	// Check new

	// Check old (until ttl)

	return s.getHeaders(ctx, HeaderMaxProfitKey(slot))
}

func (s *Datastore) PutHeader(ctx context.Context, hr structs.HR, ttl time.Duration) error {
	// we don't need to lock here, as the value would be always different from different block
	if err := s.TTLStorage.PutWithTTL(ctx, HeaderKeyContent(hr.Slot, hr.Trace.BlockHash), hr.Marshaled, ttl); err != nil {
		return err
	}

	errCH := errChPool.Get().(chan error)
	defer errChPool.Put(errCH)

	req := HRReqPool.Get().(*HRReq)
	defer HRReqPool.Put(req)

	req.TTL = ttl
	req.Ret = errCH
	req.Slot = req.Slot
	req.Hash = hr.Trace.BlockHash
	req.BuilderPubkey = hr.Trace.BuilderPubkey
	req.Value = hr.Trace.Value

	s.PutHeadersCh <- req

	return <-errCH
}

func storeHeader(s Badger, h structs.HR, headersData, maxProfit []byte, ttl time.Duration) error {
	txn := s.NewTransaction(true)
	defer txn.Discard()

	if err := txn.SetEntry(badger.NewEntry(HeaderHashKey(h.Header.BlockHash).Bytes(), HeaderKey(h.Slot).Bytes()).WithTTL(ttl)); err != nil {
		return err
	}

	if err := txn.SetEntry(badger.NewEntry(HeaderNumKey(h.Header.BlockNumber).Bytes(), HeaderKey(h.Slot).Bytes()).WithTTL(ttl)); err != nil {
		return err
	}

	/*
		if err := txn.SetEntry(badger.NewEntry(HeaderKey(h.Slot).Bytes(), headersData).WithTTL(ttl)); err != nil {
			return err
		}

		if err := txn.SetEntry(badger.NewEntry(HeaderMaxProfitKey(h.Slot).Bytes(), maxProfit).WithTTL(ttl)); err != nil {
			return err
		}
	*/
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

/*
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
}*/
