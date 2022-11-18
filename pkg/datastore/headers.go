package datastore

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
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
//func HeaderNewKey(slot structs.Slot) ds.Key {
//	return ds.NewKey(fmt.Sprintf("hi/%d", slot))
//}

func HeaderKeyContent(slot structs.Slot, blockHash string) ds.Key {
	return ds.NewKey(fmt.Sprintf("hc/%d/%s", slot, blockHash))
}

func HeaderMaxNewKey(slot structs.Slot) ds.Key {
	return ds.NewKey(fmt.Sprintf("hm/%d", slot))
}

// OLD Entries
func HeaderKey(slot structs.Slot) ds.Key {
	return ds.NewKey(fmt.Sprintf("header-%d", slot))
}

func HeaderMaxProfitKey(slot structs.Slot) ds.Key {
	return ds.NewKey(fmt.Sprintf("header/max-profit/%d", slot))
}

type StoredIndex struct {
	Index     []IndexEl
	MaxProfit IndexEl
}

type IndexEl struct {
	Hash          [32]byte
	Value         *big.Int //[32]byte
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
	content    map[uint64]*HNTs
	latestSlot *uint64
	cl         sync.RWMutex
}

func NewHeaderController() *HeaderController {
	u := uint64(0)
	return &HeaderController{
		content:    make(map[uint64]*HNTs),
		latestSlot: &u,
	}
}

// top level lock
func (hc *HeaderController) GetLatestSlot() (slot uint64) {
	return atomic.LoadUint64(hc.latestSlot)
}

/*
// top level lock
func (hc *HeaderController) GetMap(slot uint64) *HNTs {
	hc.cl.RLock()
	h, ok := hc.content[slot]
	hc.cl.RUnlock()

	if !ok {
		hc.cl.Lock()
		h = NewHNTs()
		hc.content[slot] = h
		hc.cl.Unlock()
	}
	return h
}*/

func (hc *HeaderController) Add(slot uint64, hnt structs.HeaderAndTrace) error {
	hc.cl.Lock()
	defer hc.cl.Unlock()

	h, ok := hc.content[slot]
	if !ok {
		h = NewHNTs()
		hc.content[slot] = h
	}

	return h.AddContent(hnt)
}

func (hc *HeaderController) GetContent(slot uint64, limit int) (elements []structs.HeaderAndTrace, lastSlot uint64) {
	for {
		hc.cl.RLock()
		h, ok := hc.content[slot]
		hc.cl.RUnlock()
		if !ok {
			return elements, lastSlot
		}
		lastSlot = slot
		elements = append(elements, h.GetContent()...)
		if len(elements) >= limit {
			return elements, lastSlot
		}
		slot--
	}

}

func (hc *HeaderController) GetMaxProfit(slot uint64) (hnt structs.HeaderAndTrace, ok bool) {
	hc.cl.RLock()
	defer hc.cl.RUnlock()
	s, ok := hc.content[slot]
	if !ok {
		return hnt, false
	}
	return s.GetMaxProfit()

}

/*
func PutHeaderController(hc *HeaderController, s Datastore) {
	rm := make(map[uint64]*HNTs)
	for h := range s.PutHeadersCh {
		dbHeaders, ok := rm[h.Slot]
		if !ok {

		}
		dbHeaders.Add(IndexEl{Hash: h.Hash, Value: h.Value, BuilderPubkey: h.BuilderPubkey})

			// dbHeaders, ok := rm[h.hr.Trace.Slot]
			// if !ok {
			// 	hnt, err := getHNT(ctx, s, h.hr.Slot)
			// 	if err != nil {
			// 		h.ret <- err
			// 		continue
			// 	}
			// 	dbHeaders = hnt
			// }
			// dbHeaders.Add(h.hr)

			// err := storeHeader(s.Badger, h.hr, dbHeaders.Serialize(), dbHeaders.SerializeMaxProfit(), h.ttl)
			// if err == nil {
			// 	rm[h.hr.Trace.Slot] = dbHeaders
			// }
			// h.ret <- err
	}
}
*/

func (s *Datastore) GetMaxProfitHeader(ctx context.Context, slot structs.Slot) (structs.HeaderAndTrace, error) {
	// Check memory
	p, ok := s.hc.GetMaxProfit(uint64(slot))
	if ok {
		return p, nil
	}

	// Check new
	p, err := s.getMaxHeader(ctx, slot)
	if err != nil {
		return p, nil
	}

	// Check old (until ttl passes)
	headers, err := s.getHeaders(ctx, HeaderMaxProfitKey(slot))
	if err != nil {
		return p, nil
	}

	if len(headers) == 0 {
		return p, fmt.Errorf("there is no header")
	}

	return headers[0], nil
}

var ErrNotFound = errors.New("not found")

func (s *Datastore) getMaxHeader(ctx context.Context, slot structs.Slot) (h structs.HeaderAndTrace, err error) {
	txn := s.Badger.NewTransaction(false)
	defer txn.Discard()
	defer txn.Commit()

	item, err := txn.Get(HeaderMaxNewKey(slot).Bytes())
	if err != nil {
		return h, err
	}

	item, err = txn.Get(HeaderKeyContent(slot, item.String()).Bytes())
	if err != nil {
		return h, err
	}

	h = structs.HeaderAndTrace{}
	err = item.Value(func(val []byte) error {
		return json.Unmarshal(val, &h)
	})

	return h, err
}

func (s *Datastore) PutHeader(ctx context.Context, hr structs.HR, ttl time.Duration) (err error) {
	if err := storeHeader(s.Badger, hr, ttl); err != nil {
		return err
	}

	return s.hc.Add(uint64(hr.Slot), hr.HeaderAndTrace)
}

func storeHeader(s Badger, h structs.HR, ttl time.Duration) error {
	txn := s.NewTransaction(true)
	defer txn.Discard()

	// we don't need to lock here, as the value would be always different from different block
	if err := txn.SetEntry(badger.NewEntry(HeaderKeyContent(h.Slot, h.Trace.BlockHash.String()).Bytes(), h.Marshaled).WithTTL(ttl)); err != nil {
		return err
	}

	slot := make([]byte, 8)
	binary.LittleEndian.PutUint64(slot, uint64(h.Slot))

	if err := txn.SetEntry(badger.NewEntry(HeaderHashKey(h.Header.BlockHash).Bytes(), slot).WithTTL(ttl)); err != nil {
		return err
	}

	// not needed every time
	if err := txn.SetEntry(badger.NewEntry(HeaderNumKey(h.Header.BlockNumber).Bytes(), slot).WithTTL(ttl)); err != nil {
		return err
	}

	//if err := s.TTLStorage.PutWithTTL(ctx, HeaderKeyContent(hr.Trace.BlockHash.String()), hr.Marshaled, ttl+time.Minute); err != nil {
	//	return err
	//}

	/*
		if err := txn.SetEntry(badger.NewEntry(HeaderKey(h.Slot).Bytes(), headersData).WithTTL(ttl)); err != nil {
			return err
		}

	*/
	return txn.Commit()
}

/*
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
}*/

// HeaderKeyContent(slot structs.Slot, blockHash string) ds.Key
func (s *Datastore) GetHeadersBySlot(ctx context.Context, slot uint64) ([]structs.HeaderAndTrace, error) {
	el, _ := s.hc.GetContent(slot, 1)
	if el != nil {
		return el, nil
	}

}

func (s *Datastore) GetHeadersByBlockNum(ctx context.Context, blockNumber uint64) ([]structs.HeaderAndTrace, error) {
	slot, err := s.TTLStorage.Get(ctx, HeaderNumKey(blockNumber))
	if err != nil {
		return nil, err
	}
	return s.GetHeadersBySlot(ctx, binary.LittleEndian.Uint64(slot))
}

func (s *Datastore) GetHeadersByBlockHash(ctx context.Context, hash types.Hash) ([]structs.HeaderAndTrace, error) {
	slot, err := s.TTLStorage.Get(ctx, HeaderHashKey(hash))
	if err != nil {
		return nil, err
	}
	return s.GetHeadersBySlot(ctx, binary.LittleEndian.Uint64(slot))
}

func (s *Datastore) GetLatestHeaders(ctx context.Context, limit uint64) ([]structs.HeaderAndTrace, error) {

	ls := s.hc.GetLatestSlot()
	el, lastSlot := s.hc.GetContent(ls, int(limit))
	if el != nil {
		// all from memory
		if len(el) >= int(limit) {
			return el[:limit], nil
		}
	}

	initialLSlot := lastSlot
	for {
		data, err := s.TTLStorage.Get(ctx, HeaderKey(structs.Slot(initialLSlot)))
		if err != nil {
			return nil, err
		}

	}

}

/*
func (s *Datastore) GetHeaders(ctx context.Context, query structs.HeaderQuery) ([]structs.HeaderAndTrace, error) {
	//key, err := s.queryToHeaderKey(ctx, query)
	if err != nil {
		return nil, err
	}
	headers, err := s.getHeaders(ctx, key)
	if err != nil {
		return nil, err
	}

	return s.deduplicateHeaders(headers, query), nil
}
*/
/*
func (s *Datastore) getHeaders(ctx context.Context, key ds.Key) ([]structs.HeaderAndTrace, error) {

	return s.unsmarshalHeaders(data)
}*/

/*
func (s *Datastore) deduplicateHeaders(headers []structs.HeaderAndTrace, query structs.HeaderQuery) []structs.HeaderAndTrace {
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

		filtered = append(filtered, header)
	}

	return filtered
}
*/

/*
func (s *Datastore) GetHeaderBatch(ctx context.Context, queries []structs.HeaderQuery) ([]structs.HeaderAndTrace, error) {
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
*/
/*
func (s *Datastore) queryToHeaderKey(ctx context.Context, query structs.HeaderQuery) (ds.Key, error) {
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
*/
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
