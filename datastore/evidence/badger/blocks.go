package badger

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/blocknative/dreamboat/structs"
	"github.com/dgraph-io/badger/v2"
	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
)

const (
	HeaderContentPrefix = "hc"
	maxSlotLag          = 50
)

func HeaderKeyContent(slot uint64, blockHash string, builderHash string) ds.Key {
	return ds.NewKey(fmt.Sprintf("%s/%d/%s/%s", HeaderContentPrefix, slot, blockHash, builderHash))
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
	if err := txn.SetEntry(badger.NewEntry(HeaderKeyContent(uint64(bid.Slot), bid.BlockHash.String(), bid.BuilderPubkey.String()).Bytes(), data).WithTTL(s.TTL)); err != nil {
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
		events, err = s.getHeadersRange(ctx, strconv.FormatUint(uint64(query.Slot), 10), "")
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

func (s *Datastore) getHeadersRange(ctx context.Context, slot, blockhash string) (el []structs.BidTraceWithTimestamp, err error) {
	err = s.DBInter.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		readr := bytes.NewReader(nil)
		dec := json.NewDecoder(readr)
		var prefix []byte
		if blockhash != "" {
			prefix = []byte(strings.Join([]string{"/" + HeaderContentPrefix, slot, blockhash}, "/"))
		} else {
			prefix = []byte(strings.Join([]string{"/" + HeaderContentPrefix, slot}, "/"))
		}

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			c, err := it.Item().ValueCopy(nil)
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

	return s.getHeadersRange(ctx, strconv.FormatUint(binary.LittleEndian.Uint64(slot), 10), "")
}

func (s *Datastore) getHeadersByBlockHash(ctx context.Context, hash types.Hash) ([]structs.BidTraceWithTimestamp, error) {
	slot, err := s.DB.Get(ctx, HeaderHashKey(hash))
	if err != nil {
		return nil, err
	}

	return s.getHeadersRange(ctx, strconv.FormatUint(binary.LittleEndian.Uint64(slot), 10), hash.String())

}

func (s *Datastore) getLatestHeaders(ctx context.Context, headSlot uint64, limit int) (el []structs.BidTraceWithTimestamp, err error) {
	initialSlot := headSlot - 1

	for {
		events, err := s.getHeadersRange(ctx, strconv.FormatUint(initialSlot, 10), "")
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
