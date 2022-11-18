package datastore

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/dgraph-io/badger/v2"
	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
)

const (
	HeaderPrefix        = "header-"
	HeaderContentPrefix = "hc/"
)

var (
	HeaderPrefixBytes = []byte("header-")
)

func HeaderKeyContent(slot structs.Slot, blockHash string) ds.Key {
	return ds.NewKey(fmt.Sprintf("hc/%d/%s", slot, blockHash))
}

func HeaderMaxNewKey(slot structs.Slot) ds.Key {
	return ds.NewKey(fmt.Sprintf("hm/%d", slot))
}

func HeaderKey(slot uint64) ds.Key {
	return ds.NewKey(fmt.Sprintf("%s%d", HeaderPrefix, slot))
}

// OLD Entries
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

func (hc *HeaderController) AddMultiple(slot uint64, hnt []structs.HeaderAndTrace) (err error) {
	hc.cl.Lock()
	defer hc.cl.Unlock()

	h, ok := hc.content[slot]
	if !ok {
		h = NewHNTs()
		hc.content[slot] = h
	}

	for _, s := range hnt {
		if err := h.AddContent(s); err != nil {
			return err
		}
	}

	return nil
}

func (hc *HeaderController) Add(slot uint64, hnt structs.HeaderAndTrace) (newCreated bool, err error) {
	hc.cl.Lock()
	defer hc.cl.Unlock()

	h, ok := hc.content[slot]
	if !ok {
		h = NewHNTs()
		hc.content[slot] = h
		newCreated = true
	}
	return newCreated, h.AddContent(hnt)
}

func (hc *HeaderController) GetContent(startingSlot uint64, limit int) (elements []structs.HeaderAndTrace, lastSlot uint64) {
	for {
		hc.cl.RLock()
		h, ok := hc.content[startingSlot]
		hc.cl.RUnlock()
		if !ok {
			return elements, lastSlot
		}
		lastSlot = startingSlot
		c, _ := h.GetContent()
		elements = append(elements, c...)
		if len(elements) >= limit {
			return elements, lastSlot
		}
		startingSlot--
		// implement step limit
	}
}

func (hc *HeaderController) GetSingleSlot(slot uint64) (elements []structs.HeaderAndTrace, revision uint64, err error) {
	hc.cl.RLock()
	h, ok := hc.content[slot]
	hc.cl.RUnlock()

	if !ok {
		return nil, 0, nil
	}
	elements, revision = h.GetContent()
	return elements, revision, err
}

func (hc *HeaderController) UnlinkElement(slot, expectedRevision uint64) (success bool) {
	hc.cl.Lock()
	defer hc.cl.Unlock()

	s, ok := hc.content[slot]
	if !ok {
		return true
	}

	if expectedRevision != s.GetRevision() {
		return false
	}

	// we're only unlinking from map here
	// it should be later on eligible for GC
	delete(hc.content, slot)
	return true
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
	headers, err := s.deprecatedGetHeaders(ctx, HeaderMaxProfitKey(slot))
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

	newlyCreated, err := s.hc.Add(uint64(hr.Slot), hr.HeaderAndTrace)
	if err != nil {
		return err
	}

	if !newlyCreated {
		return // success
	}
	// check and load keys if exists
	return s.loadKeys(ctx, uint64(hr.Slot))
}

func (s *Datastore) loadKeys(ctx context.Context, slot uint64) error {
	s.l.Lock()
	defer s.l.Unlock()

	// need to load keys to memory
	data, err := s.TTLStorage.Get(ctx, HeaderKey(slot))
	if err != nil {
		if errors.Is(err, badger.ErrEmptyKey) {
			return nil // success - key is empty, nothing to load
		}
		return err
	}
	var h []structs.HeaderAndTrace
	if err := json.Unmarshal(data, &h); err != nil {
		return err
	}

	return s.hc.AddMultiple(slot, h)
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

	return txn.Commit()
}

func (s *Datastore) GetHeadersBySlot(ctx context.Context, slot uint64) ([]structs.HeaderAndTrace, error) {
	el, _ := s.hc.GetContent(slot, 1)
	if el != nil {
		return el, nil
	}

	data, err := s.TTLStorage.Get(ctx, HeaderKey(slot))
	if err != nil && errors.Is(err, badger.ErrEmptyKey) {
		return el, err
	}

	el = []structs.HeaderAndTrace{}
	if err = json.Unmarshal(data, &el); err != nil {
		return el, err
	}

	return el, err
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
	if el == nil {
		el = []structs.HeaderAndTrace{}
	}

	// all from memory
	if len(el) >= int(limit) {
		return el[:limit], nil
	}

	initialLSlot := lastSlot
	readr := bytes.NewReader(nil)
	dec := json.NewDecoder(readr)
	for {
		data, err := s.TTLStorage.Get(ctx, HeaderKey(initialLSlot))
		if err != nil {
			return el, err
		}
		readr.Reset(data)
		hnt := []structs.HeaderAndTrace{}
		if err := dec.Decode(&hnt); err != nil {
			return nil, err
		}

		el = append(el, hnt...)
		initialLSlot--

		// introduce limit?
	}
}

func (s *Datastore) deprecatedGetHeaders(ctx context.Context, key ds.Key) ([]structs.HeaderAndTrace, error) {
	data, err := s.TTLStorage.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return s.deprecatedUnsmarshalHeaders(data)
}

func (s *Datastore) deprecatedUnsmarshalHeaders(data []byte) ([]structs.HeaderAndTrace, error) {
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

// SaveHeaders is meant to persist the all the keys under one key
// As optimization in future this function can operate only on database, so instead from memory it may just reorganize keys
func (s *Datastore) SaveHeaders(ctx context.Context, slot uint64, ttl time.Duration) error {
	el, rev, err := s.hc.GetSingleSlot(slot)
	if err != nil {
		return err
	}

	if err = saveHeaders(ctx, s, slot, el, ttl); err != nil {
		return err
	}

	if s.hc.UnlinkElement(slot, rev) {
		return nil // success
	}

	// revert the saveHeaders operation as revision changed
	_ = s.Badger.Update(func(txn *badger.Txn) error {
		return txn.Delete(HeaderKey(slot).Bytes())
	})

	return errors.New("revision changed")
}

func saveHeaders(ctx context.Context, s *Datastore, slot uint64, cont []structs.HeaderAndTrace, ttl time.Duration) error {
	buff := bytes.NewBuffer(nil)
	buff.WriteString("[")

	enc := json.NewEncoder(buff)

	for i, c := range cont {
		if i > 0 {
			buff.WriteString(",")
		}
		if err := enc.Encode(c); err != nil {
			return err
		}
	}
	buff.WriteString("]")
	if err := s.TTLStorage.PutWithTTL(ctx, HeaderKey(slot), buff.Bytes(), ttl); err != nil {
		return err
	}

	buff.Truncate(0) // immediately remove
	return nil
}

type LoadItem struct {
	Time    uint64
	Content []byte
}

// FixOrphanHeaders is reading all the orphan headers from
func (s *Datastore) FixOrphanHeaders(ctx context.Context, ttl time.Duration) error {
	exists := make(map[uint64][]LoadItem)

	// Get all headers, rebuild
	err := s.Badger.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false

		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("/" + HeaderContentPrefix)
		re := regexp.MustCompile(`\/hc\/([^\/]+)\/([^\/]+)`)

		//for it.Rewind(); it.Valid(); it.Next() {
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			subM := re.FindSubmatch(item.Key())
			if len(subM) != 3 {
				continue
			}
			slot, err := strconv.ParseUint(string(subM[2]), 10, 64)
			if err != nil {
				continue
			}
			is, ok := exists[slot]
			if !ok {
				_, err := txn.Get(append(HeaderPrefixBytes, subM[1]...))
				if err != nil {
					if !errors.Is(err, badger.ErrEmptyKey) {
						return err
					}
					exists[slot] = nil
				}
				is = []LoadItem{}
				exists[slot] = is
			}

			if ok && is == nil {
				continue
			}

			li := LoadItem{
				Time:    item.Version(),
				Content: make([]byte, item.ValueSize())}
			item.ValueCopy(li.Content)
			exists[slot] = append(is, li)
		}
		return nil
	})
	if err != nil {
		return err
	}

	buff := new(bytes.Buffer)
	for slot, v := range exists {
		if v != nil {
			buff.Reset()
			sort.Slice(v, func(i, j int) bool {
				return v[i].Time > v[j].Time
			})

			buff.WriteString("[")
			for i, payload := range v {
				if i > 0 {
					buff.WriteString(",")
				}
				io.Copy(buff, bytes.NewReader(payload.Content))

			}
			buff.WriteString("]")

			if err := s.TTLStorage.PutWithTTL(ctx, HeaderKey(slot), buff.Bytes(), ttl); err != nil {
				return err
			}
		}
	}
	return err
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
