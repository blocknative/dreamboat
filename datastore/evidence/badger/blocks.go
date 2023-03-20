package badger

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/blocknative/dreamboat/structs"
	"github.com/dgraph-io/badger/v2"
	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
)

const (
	HeaderContentPrefix = "hc/"
	maxSlotLag          = 50
)

func HeaderKeyContent(slot uint64, blockHash string) ds.Key {
	return ds.NewKey(fmt.Sprintf("%s/%d/%s", HeaderContentPrefix, slot, blockHash))
}

func HeaderHashKey(bh types.Hash) ds.Key {
	return ds.NewKey(fmt.Sprintf("header-hash-%s", bh.String()))
}

func HeaderNumKey(bn uint64) ds.Key {
	return ds.NewKey(fmt.Sprintf("header-num-%d", bn))
}

func (s *Datastore) PutBuilderBlockSubmission(ctx context.Context, bid structs.BidTraceWithTimestamp, isMostProfitable bool) (err error) {
	data, err := json.Marshal(bid)
	if err != nil {
		return err
	}

	txn := s.DBInter.NewTransaction(true)
	defer txn.Discard()
	slot := make([]byte, 8)
	binary.LittleEndian.PutUint64(slot, uint64(bid.Slot))

	// another write of the same data.
	if err := txn.SetEntry(badger.NewEntry(HeaderKeyContent(uint64(bid.Slot), bid.BlockHash.String()).Bytes(), data).WithTTL(s.TTL)); err != nil {
		return err
	}

	if err := txn.SetEntry(badger.NewEntry(HeaderHashKey(bid.BlockHash).Bytes(), slot).WithTTL(s.TTL)); err != nil {
		return err
	}

	if err := txn.SetEntry(badger.NewEntry(HeaderNumKey(bid.BlockNumber).Bytes(), slot).WithTTL(s.TTL)); err != nil {
		return err
	}

	return txn.Commit()
}

func (s *Datastore) GetBuilderBlockSubmissions(ctx context.Context, headSlot uint64, query structs.SubmissionTraceQuery) (events []structs.BidTraceWithTimestamp, err error) {

	if query.HasSlot() {
		events, err = s.getHeadersBySlot(ctx, uint64(query.Slot))
	} else if query.HasBlockHash() {
		events, err = s.getHeadersByBlockHash(ctx, query.BlockHash)
	} else if query.HasBlockNum() {
		events, err = s.getHeadersByBlockNum(ctx, query.BlockNum)
	} else {
		events, err = s.getLatestHeaders(ctx, headSlot, int(query.Limit))
	}

	if err != nil {
		if errors.Is(err, ds.ErrNotFound) {
			return []structs.BidTraceWithTimestamp{}, nil
		}
		return []structs.BidTraceWithTimestamp{}, err
	}
	return events, err
}

func (s *Datastore) getHeadersBySlot(ctx context.Context, slot uint64) (el []structs.BidTraceWithTimestamp, err error) {
	err = s.DBInter.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false

		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		readr := bytes.NewReader(nil)
		dec := json.NewDecoder(readr)
		prefix := []byte("/" + HeaderContentPrefix + strconv.FormatUint(slot, 10) + "/")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			c, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			readr.Reset(c)

			b := structs.BidTraceWithTimestamp{}
			if err := dec.Decode(&b); err != nil {
				return err
			}
			el = append(el, b)
		}
		return nil
	})

	return el, err
}

func (s *Datastore) getHeadersByBlockNum(ctx context.Context, blockNumber uint64) ([]structs.BidTraceWithTimestamp, error) {
	slot, err := s.DB.Get(ctx, HeaderNumKey(blockNumber))
	if err != nil {
		return nil, err
	}
	return s.getHeadersBySlot(ctx, binary.LittleEndian.Uint64(slot))
}

func (s *Datastore) getHeadersByBlockHash(ctx context.Context, hash types.Hash) ([]structs.BidTraceWithTimestamp, error) {
	slot, err := s.DB.Get(ctx, HeaderHashKey(hash))
	if err != nil {
		return nil, err
	}

	content, err := s.DB.Get(ctx, HeaderKeyContent(binary.LittleEndian.Uint64(slot), hash.String()))
	if err != nil {
		if !errors.Is(err, badger.ErrKeyNotFound) { // do not fail on not found try others
			return nil, err
		}
	}

	el := structs.BidTraceWithTimestamp{}
	if err = json.Unmarshal(content, &el); err != nil {
		return nil, err
	}
	return []structs.BidTraceWithTimestamp{el}, nil
}

func (s *Datastore) getLatestHeaders(ctx context.Context, headSlot uint64, limit int) (el []structs.BidTraceWithTimestamp, err error) {
	initialSlot := headSlot - 1

	for {
		events, err := s.getHeadersBySlot(ctx, initialSlot)
		if err != nil {
			if errors.Is(err, ds.ErrNotFound) {
				return el, nil
			}
			return el, err
		}

		el = append(el, events...)
		if len(el) == limit {
			return el, nil
		} else if len(el) > limit {
			return el[0:limit], nil
		}

		if headSlot-initialSlot >= maxSlotLag {
			return el, nil
		}
		initialSlot--
	}

}

/*
func (s *Datastore) getLatestHeaders(ctx context.Context, headSlot uint64, limit int, stopLag uint64) (el []structs.BidTraceWithTimestamp, err error) {

	initialSlot := headSlot - 1
	readr := bytes.NewReader(nil)
	dec := json.NewDecoder(readr)
	for {
		data, err := s.DB.Get(ctx, HeaderKey(initialSlot))
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
		sum := len(el) + len(hnt)
		if sum >= limit {
			numEl := limit - len(el)
			if numEl != 0 {
				if len(hnt) <= numEl {
					el = append(el, hnt[0:numEl-1]...)
				} else {
					el = append(el, hnt[0:len(hnt)-1]...)
				}
			}
			return el, err
		}
		el = append(el, hnt...)

		initialSlot--
	}
}
*/
