package datastore

import (
	"sync"

	"github.com/blocknative/dreamboat/pkg/structs"
)

type HNTs struct {
	S StoredIndex

	content                    []structs.HeaderAndTrace
	blockHashToContentPosition map[[32]byte]int
	contentLock                sync.RWMutex
}

func NewHNTs() (h *HNTs) {
	return &HNTs{
		blockHashToContentPosition: make(map[[32]byte]int),
	}
}

func (h *HNTs) GetContent() []structs.HeaderAndTrace {
	h.contentLock.RLock()
	defer h.contentLock.RUnlock()

	return h.content
}

func (h *HNTs) GetMaxProfit() (hnt structs.HeaderAndTrace, ok bool) {
	h.contentLock.RLock()
	defer h.contentLock.RUnlock()

	n, ok := h.blockHashToContentPosition[h.S.MaxProfit.Hash]
	if !ok {
		return hnt, ok
	}
	return h.content[n], true
}

func (h *HNTs) AddContent(hnt structs.HeaderAndTrace) error {
	newEl := IndexEl{
		Hash:          hnt.Trace.BlockHash,
		Value:         hnt.Trace.Value.BigInt(),
		BuilderPubkey: hnt.Trace.BuilderPubkey,
	}

	h.contentLock.Lock()
	defer h.contentLock.Unlock()
	_, ok := h.blockHashToContentPosition[hnt.Trace.BlockHash]
	if !ok {
		h.content = append(h.content, hnt)
		h.blockHashToContentPosition[hnt.Trace.BlockHash] = len(h.content)
	}

	h.S.Index = append(h.S.Index, newEl)

	// we should allow resubmission
	if h.S.MaxProfit.BuilderPubkey == newEl.BuilderPubkey {
		// if new value is smaller we should pick new max that is not
		if h.S.MaxProfit.Value.Cmp(newEl.Value) < 0 {
			for _, el := range h.S.Index {
				if (h.S.MaxProfit.Value == nil ||
					h.S.MaxProfit.Value.Cmp(el.Value) < 0) &&
					h.S.MaxProfit.BuilderPubkey == newEl.BuilderPubkey {
					h.S.MaxProfit = el
				}
			}
		}
	} else {
		if h.S.MaxProfit.Value == nil ||
			h.S.MaxProfit.Value.Cmp(newEl.Value) < 0 {
			h.S.MaxProfit = newEl
		}
	}

	return nil
}

/*
	func (h *HNTs) Add(ihr IndexEl, content []byte) {
		//h.S = append(h.S, &ihr)
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
*/
/*
func (h *HNTs) SerializeIndex() (b []byte, err error) {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	if err = enc.Encode(h.S); err != nil {
		return nil, err
	}
	defer buf.Reset()
	return buf.Bytes(), nil
}

func (h *HNTs) Serialize() (b []byte) {
	b = append(b, []byte("[")...)


	for i, c := range h.S.Index {
		if i != 0 {
			b = append(b, []byte(",")...)
		}

		b = append(b, c.Marshaled...)
	}
	b = append(b, []byte("]")...)
	return b
}
*/
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
