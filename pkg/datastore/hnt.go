package datastore

import (
	"bytes"
	"encoding/gob"
	"sort"
	"sync"
)

type HNTs struct {
	S   StoredIndex
	buf *bytes.Buffer

	blockHashToContent map[[32]byte][]byte
	contentLock        sync.RWMutex
}

func NewHNTs() (h *HNTs) {
	return &HNTs{
		buf:                new(bytes.Buffer),
		blockHashToContent: make(map[[32]byte][]byte),
	}
}

func (h *HNTs) AddContent(hash [32]byte, content []byte) {
	h.contentLock.Lock()
	defer h.contentLock.Unlock()

}

func (h *HNTs) Add(ihr IndexEl, content []byte) {

	h.S = append(h.S, &ihr)

	// h.blockHashToIndex[ihr.Trace.BlockHash] = struct{}{}
	// var exists bool
	// skip max profit
	// TODO(l): Make sure we shouldn't check the profit here
	for i, mp := range h.MaxProfit {
		if mp.Trace.BuilderPubkey == ihr.Trace.BuilderPubkey {
			exists = true
			h.MaxProfit[i] = mp
			break
		}
	}
	if !exists {
		h.MaxProfit = append(h.MaxProfit, &ihr)
	}

	sort.Slice(h.MaxProfit, func(i, j int) bool {
		return h.MaxProfit[i].Trace.Value.Cmp(&h.MaxProfit[j].Trace.Value) > 0
	})
}

func (h *HNTs) LoadMaxProfit() {
	for i := range h.S.Index {
		h.MaxProfit = append(h.MaxProfit, h.S[i])
	}
	// sort by bid value DESC
	sort.Slice(h.MaxProfit, func(i, j int) bool {
		return h.MaxProfit[i].Trace.Value.Cmp(&h.MaxProfit[j].Trace.Value) > 0
	})
}

func (h *HNTs) SerializeIndex() (b []byte, err error) {
	enc := gob.NewEncoder(h.buf)
	if err = enc.Encode(h.S); err != nil {
		return nil, err
	}
	defer h.buf.Reset()
	return h.buf.Bytes(), nil
}

func (h *HNTs) Serialize() (b []byte) {
	b = append(b, []byte("[")...)

	/*for i, c := range h.S {
		if i != 0 {
			b = append(b, []byte(",")...)
		}
		b = append(b, c.Marshaled...)
	}*/

	for i, c := range h.S.Index {
		if i != 0 {
			b = append(b, []byte(",")...)
		}

		b = append(b, c.Marshaled...)
	}
	b = append(b, []byte("]")...)
	return b
}

/*
type HNTs struct {
	S         []*structs.HR
	MaxProfit []*structs.HR

	blockHashToIndex map[types.Hash]struct{}
}

func NewHNTs() (h *HNTs) {
	return &HNTs{
		blockHashToIndex: make(map[types.Hash]struct{}),
	}
}

func (h *HNTs) Add(ihr structs.HR) {
	if _, ok := h.blockHashToIndex[ihr.Trace.BlockHash]; ok {
		return // already exists
	}

	h.S = append(h.S, &ihr)
	h.blockHashToIndex[ihr.Trace.BlockHash] = struct{}{}
	var exists bool
	// skip max profit
	// TODO(l): Make sure we shouldn't check the profit here
	for i, mp := range h.MaxProfit {
		if mp.Trace.BuilderPubkey == ihr.Trace.BuilderPubkey {
			exists = true
			h.MaxProfit[i] = mp
			break
		}
	}
	if !exists {
		h.MaxProfit = append(h.MaxProfit, &ihr)
	}

	sort.Slice(h.MaxProfit, func(i, j int) bool {
		return h.MaxProfit[i].Trace.Value.Cmp(&h.MaxProfit[j].Trace.Value) > 0
	})
}

func (h *HNTs) LoadMaxProfit() {
	for i := range h.S {
		h.MaxProfit = append(h.MaxProfit, h.S[i])
	}
	// sort by bid value DESC
	sort.Slice(h.MaxProfit, func(i, j int) bool {
		return h.MaxProfit[i].Trace.Value.Cmp(&h.MaxProfit[j].Trace.Value) > 0
	})
}

func (h *HNTs) Serialize() (b []byte) {
	b = append(b, []byte("[")...)
	for i, c := range h.S {
		if i != 0 {
			b = append(b, []byte(",")...)
		}
		b = append(b, c.Marshaled...)

	}
	b = append(b, []byte("]")...)
	return b
}

func (h *HNTs) SerializeMaxProfit() (b []byte) {
	b = append(b, []byte("[")...)
	for i, c := range h.MaxProfit {
		if i != 0 {
			b = append(b, []byte(",")...)
		}
		b = append(b, c.Marshaled...)

	}
	b = append(b, []byte("]")...)
	return b
}
*/
