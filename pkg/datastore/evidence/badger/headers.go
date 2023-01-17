package dbadger

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/dgraph-io/badger/v2"
	"github.com/flashbots/go-boost-utils/types"
)

func (s *Datastore) GetHeadersBySlot(ctx context.Context, slot uint64) ([]structs.HeaderAndTrace, error) {

	el, _ := s.hc.GetHeaders(slot, slot, 1)
	if el != nil {
		return el, nil
	}

	data, err := s.DB.Get(ctx, HeaderKey(slot))
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
	slot, err := s.DB.Get(ctx, HeaderNumKey(blockNumber))
	if err != nil {
		return nil, err
	}
	return s.GetHeadersBySlot(ctx, binary.LittleEndian.Uint64(slot))
}

func (s *Datastore) GetHeadersByBlockHash(ctx context.Context, hash types.Hash) ([]structs.HeaderAndTrace, error) {
	slot, err := s.DB.Get(ctx, HeaderHashKey(hash))
	if err != nil {
		return nil, err
	}

	newContent, err := s.DB.Get(ctx, HeaderKeyContent(binary.LittleEndian.Uint64(slot), hash.String()))
	if err != nil {
		if !errors.Is(err, badger.ErrKeyNotFound) { // do not fail on not found try others
			return nil, err
		}
		// old code fallback - to be removed
		if true {
			newContent, err = s.DB.Get(ctx, HeaderKey(binary.LittleEndian.Uint64(slot)))
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

		el = append(el, hnt...)
		initialSlot--
		// introduce limit?
	}
}
