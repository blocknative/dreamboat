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
	"sort"
	"strconv"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/dgraph-io/badger/v2"
	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
)

const (
	HeaderPrefix        = "header-"
	HeaderContentPrefix = "hc/"

	FixSlotLag = 5
)

var (
	HeaderPrefixBytes = []byte("header-")
)

func HeaderKeyContent(slot uint64, blockHash string) ds.Key {
	return ds.NewKey(fmt.Sprintf("hc/%d/%s", slot, blockHash))
}

func HeaderMaxNewKey(slot uint64) ds.Key {
	return ds.NewKey(fmt.Sprintf("hm/%d", slot))
}

func HeaderKey(slot uint64) ds.Key {
	return ds.NewKey(fmt.Sprintf("%s%d", HeaderPrefix, slot))
}

// OLD Entries
func HeaderMaxProfitKey(slot uint64) ds.Key {
	return ds.NewKey(fmt.Sprintf("header/max-profit/%d", slot))
}

type StoredIndex struct {
	Index                []IndexMeta
	MaxProfit            IndexMeta
	SubmissionsByPubKeys map[[48]byte]IndexMeta
}

func NewStoreIndex() StoredIndex {
	return StoredIndex{
		SubmissionsByPubKeys: make(map[[48]byte]IndexMeta),
	}
}

type IndexMeta struct {
	Hash          [32]byte
	Value         *big.Int
	BuilderPubkey [48]byte
}

type SlotInfo struct {
	Slot  uint64
	Added time.Time
}

func (s *Datastore) GetMaxProfitHeader(ctx context.Context, slot uint64) (structs.HeaderAndTrace, error) {
	p, ok := s.hc.GetMaxProfit(uint64(slot))
	if ok {
		return p, nil
	}

	p, err := s.getMaxHeader(ctx, slot)
	return p, err
}

var ErrNotFound = errors.New("not found")

func (s *Datastore) getMaxHeader(ctx context.Context, slot uint64) (h structs.HeaderAndTrace, err error) {
	txn := s.Badger.NewTransaction(false)
	defer txn.Discard()

	item, err := txn.Get(HeaderMaxNewKey(slot).Bytes())
	if err != nil {
		return h, err
	}

	v, err := item.ValueCopy(nil)
	if err != nil {
		return h, err
	}
	item, err = txn.Get(HeaderKeyContent(slot, string(v)).Bytes())
	if err != nil {
		return h, err
	}

	h = structs.HeaderAndTrace{}
	err = item.Value(func(val []byte) error {
		return json.Unmarshal(val, &h)
	})

	return h, err
}

func (s *Datastore) PutHeader(ctx context.Context, hd structs.HeaderData, ttl time.Duration) (err error) {
	if err := storeHeader(s.Badger, hd, ttl); err != nil {
		return err
	}

	newlyCreated, err := s.hc.Add(uint64(hd.Slot), hd.HeaderAndTrace)
	if err != nil {
		return err
	}

	if !newlyCreated {
		return // success
	}

	// check and load keys if exists
	return s.loadKeysAndCleanup(ctx, uint64(hd.Slot))
}

func (s *Datastore) loadKeysAndCleanup(ctx context.Context, slot uint64) error {
	s.l.Lock()
	defer s.l.Unlock()

	// need to load keys to memory
	data, err := s.TTLStorage.Get(ctx, HeaderKey(slot))
	if err != nil {
		if errors.Is(err, ds.ErrNotFound) {
			return nil // success - key is empty, nothing to load
		}
		return err
	}
	var h []structs.HeaderAndTrace
	if err := json.Unmarshal(data, &h); err != nil {
		return err
	}

	return s.hc.PrependMultiple(slot, h)
}

func storeHeader(s Badger, h structs.HeaderData, ttl time.Duration) error {
	txn := s.NewTransaction(true)
	defer txn.Discard()

	// we don't need to lock here, as the value would be always different from different block
	if err := txn.SetEntry(badger.NewEntry(HeaderKeyContent(uint64(h.Slot), h.Header.BlockHash.String()).Bytes(), h.Marshaled).WithTTL(ttl)); err != nil {
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
	el, _ := s.hc.GetHeaders(slot, slot, 1)
	if el != nil {
		return el, nil
	}

	data, err := s.TTLStorage.Get(ctx, HeaderKey(slot))
	if err != nil && errors.Is(err, ds.ErrNotFound) {
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

	newContent, err := s.TTLStorage.Get(ctx, HeaderKeyContent(binary.LittleEndian.Uint64(slot), hash.String()))
	if err != nil {
		if !errors.Is(err, badger.ErrKeyNotFound) { // do not fail on not found try others
			return nil, err
		}
		// old code fallback - to be removed
		if true {
			newContent, err = s.TTLStorage.Get(ctx, HeaderKey(binary.LittleEndian.Uint64(slot)))
			if err != nil {
				return nil, err
			}

			el := []structs.HeaderAndTrace{}
			if err = json.Unmarshal(newContent, &el); err != nil {
				return el, err
			}

			newEl := []structs.HeaderAndTrace{}
			for _, v := range el {
				if v.Header.BlockHash == hash {
					elem := v
					newEl = append(newEl, elem)
				}
			}
			return newEl, nil
		}

	}

	el := structs.HeaderAndTrace{}
	if err = json.Unmarshal(newContent, &el); err != nil {
		return nil, err
	}
	return []structs.HeaderAndTrace{el}, nil
}

func (s *Datastore) GetLatestHeaders(ctx context.Context, limit uint64, stopLag uint64) ([]structs.HeaderAndTrace, error) {
	ls := s.hc.GetLatestSlot()
	stop := ls - stopLag
	el, lastSlot := s.hc.GetHeaders(ls, stop, int(limit))

	if el == nil {
		el = []structs.HeaderAndTrace{}
	}

	// all from memory
	if len(el) >= int(limit) {
		return el[:limit], nil
	}

	initialSlot := lastSlot
	readr := bytes.NewReader(nil)
	dec := json.NewDecoder(readr)
	for {
		data, err := s.TTLStorage.Get(ctx, HeaderKey(initialSlot))
		if err != nil {
			if errors.Is(err, ds.ErrNotFound) {
				return el, nil
			}
			return el, err
		}
		readr.Reset(data)
		hnt := []structs.HeaderAndTrace{}
		if err := dec.Decode(&hnt); err != nil {
			return nil, err
		}

		el = append(el, hnt...)
		initialSlot--
		// introduce limit?
	}
}

// SaveHeaders is meant to persist the all the keys under one key
// As optimization in future this function can operate only on database, so instead from memory it may just reorganize keys
func (s *Datastore) SaveHeaders(ctx context.Context, slots []uint64, ttl time.Duration) error {
	for _, slot := range slots {
		if err := s.saveHeader(ctx, slot, ttl); err != nil {
			return err
		}
	}
	return nil
}

func (s *Datastore) saveHeader(ctx context.Context, slot uint64, ttl time.Duration) error {
	el, maxP, rev, err := s.hc.GetSingleSlot(slot)
	if err != nil {
		return err
	}

	if err = putHeaders(ctx, s, slot, el, ttl); err != nil {
		return err
	}

	if err := s.TTLStorage.PutWithTTL(ctx, HeaderMaxNewKey(slot), []byte(types.Hash(maxP).String()), ttl); err != nil {
		return err
	}

	if s.hc.RemoveSlot(slot, rev) {
		return nil // success
	}

	// revert the saveHeaders operation as revision changed
	_ = s.Badger.Update(func(txn *badger.Txn) error {
		return txn.Delete(HeaderKey(slot).Bytes())
	})

	return errors.New("revision changed")
}

func putHeaders(ctx context.Context, s *Datastore, slot uint64, cont []structs.HeaderAndTrace, ttl time.Duration) error {
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

func (s *Datastore) getOrphanedHeaders(ctx context.Context, slot uint64) (items []LoadItem, err error) {

	err = s.Badger.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false

		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("/" + HeaderContentPrefix + "/" + strconv.FormatUint(slot, 10) + "/")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			li := LoadItem{Time: item.Version()}
			li.Content, err = item.ValueCopy(nil)
			if err != nil {
				return err
			}
			items = append(items, li)
		}
		return nil
	})

	return items, err
}

// FixOrphanHeaders is reading all the orphan headers from
func (s *Datastore) FixOrphanHeaders(ctx context.Context, latestSlot uint64, ttl time.Duration) error {

	buff := new(bytes.Buffer)
	for slot := latestSlot - FixSlotLag; slot < latestSlot+1; slot++ {

		items, err := s.getOrphanedHeaders(ctx, latestSlot)
		if err != nil {
			return err
		}

		if items != nil && len(items) > 0 {
			continue
		}
		tempHC := NewHeaderController(100, time.Hour) // params doesn't matter here

		buff.Reset()
		sort.Slice(items, func(i, j int) bool {
			return items[i].Time > items[j].Time
		})

		buff.WriteString("[")
		for i, payload := range items {
			if i > 0 {
				buff.WriteString(",")
			}
			io.Copy(buff, bytes.NewReader(payload.Content))
			hnt := structs.HeaderAndTrace{}
			if err := json.Unmarshal(payload.Content, &hnt); err != nil {
				return err
			}

			if _, err := tempHC.Add(slot, hnt); err != nil {
				return err
			}
		}
		buff.WriteString("]")
		if err := s.TTLStorage.PutWithTTL(ctx, HeaderKey(slot), buff.Bytes(), ttl); err != nil {
			return err
		}

		maxProfit, ok := tempHC.GetMaxProfit(slot)
		if !ok {
			return errors.New("max profit from records not calculated")
		}
		if err := s.TTLStorage.PutWithTTL(ctx, HeaderMaxNewKey(slot), []byte(maxProfit.Trace.BlockHash.String()), ttl); err != nil {
			return err
		}
	}
	return nil
}

func (s *Datastore) MemoryCleanup(ctx context.Context, slotPurgeDuration time.Duration, ttl time.Duration) error {
	for {
		time.Sleep(slotPurgeDuration)
		slots, ok := s.hc.CheckForRemoval()
		if !ok {
			continue
		}
		s.SaveHeaders(ctx, slots, ttl)
	}
}
