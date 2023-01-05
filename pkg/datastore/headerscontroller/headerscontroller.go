package headerscontroller

import (
	"math/big"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
)

type SlotInfo struct {
	Slot  uint64
	Added time.Time
}

type HeaderController struct {
	headers map[uint64]*IndexedHeaders
	ordered []SlotInfo

	slotLag     uint64
	slotTimeLag time.Duration

	latestSlot *uint64
	cl         sync.RWMutex

	m HeaderControllerMetrics
}

func NewHeaderController(slotLag uint64, slotTimeLag time.Duration) *HeaderController {
	latestSlot := uint64(0)
	hc := &HeaderController{
		slotLag:     slotLag,
		slotTimeLag: slotTimeLag,
		latestSlot:  &latestSlot,
		headers:     make(map[uint64]*IndexedHeaders),
	}
	hc.initMetrics()
	return hc
}

func (hc *HeaderController) CheckForRemoval() (toBeRemoved []uint64, ok bool) {
	hc.cl.RLock()
	defer hc.cl.RUnlock()
	if len(hc.ordered) == 0 {
		return nil, false
	}

	l := hc.ordered[len(hc.ordered)-1]
	for _, v := range hc.ordered {
		if !(l.Slot-v.Slot >= hc.slotLag && time.Since(v.Added) > hc.slotTimeLag) {
			hc.m.RemovalChecks.Inc()
			return toBeRemoved, ok
		}
		toBeRemoved = append(toBeRemoved, v.Slot)
		ok = true
	}
	hc.m.RemovalChecks.Inc()
	return toBeRemoved, ok

}

func (hc *HeaderController) getOrderedDesc() (info []uint64) {
	hc.cl.RLock()
	defer hc.cl.RUnlock()
	info = make([]uint64, len(hc.ordered))
	for i := len(hc.ordered) - 1; i >= 0; i-- {
		info = append(info, hc.ordered[i].Slot)
	}

	return info
}

// addToOrdered to be run in lock
func (hc *HeaderController) addToOrdered(i SlotInfo) {
	hc.ordered = append(hc.ordered, i)
	l := len(hc.ordered)
	if l > 1 {
		o := hc.ordered[l-1]
		if o.Slot >= i.Slot {
			sort.Slice(hc.ordered, func(i, j int) bool {
				return hc.ordered[i].Slot < hc.ordered[j].Slot
			})
		}
	}
}

func (hc *HeaderController) GetLatestSlot() (slot uint64) {
	return atomic.LoadUint64(hc.latestSlot)
}

func (hc *HeaderController) PrependMultiple(slot uint64, hnt []structs.HeaderAndTrace) (err error) {
	hc.cl.Lock()
	defer hc.cl.Unlock()

	h, ok := hc.headers[slot]
	if !ok {
		h = NewIndexedHeaders()
		hc.headers[slot] = h
		hc.addToOrdered(SlotInfo{
			Slot:  slot,
			Added: time.Now(),
		})

		ls := atomic.LoadUint64(hc.latestSlot)
		if ls == 0 || slot > ls {
			atomic.StoreUint64(hc.latestSlot, slot)
			hc.m.LatestSlot.Set(float64(slot))
		}
	}

	if err := h.PrependContent(hnt); err != nil {
		return err
	}
	hc.m.HeadersAdded.Add(float64(len(hnt)))

	return nil
}

func (hc *HeaderController) Add(slot uint64, hnt structs.HeaderAndTrace) (newCreated bool, err error) {
	hc.cl.Lock()
	defer hc.cl.Unlock()

	h, ok := hc.headers[slot]
	if !ok {
		h = NewIndexedHeaders()
		hc.headers[slot] = h
		hc.addToOrdered(SlotInfo{
			Slot:  slot,
			Added: time.Now(),
		})
		newCreated = true

		ls := atomic.LoadUint64(hc.latestSlot)
		if ls == 0 || slot > ls {
			atomic.StoreUint64(hc.latestSlot, slot)
			hc.m.LatestSlot.Set(float64(slot))
		}
	}
	hc.m.HeadersAdded.Inc()
	hc.m.HeadersSize.Set(float64(len(hc.headers)))
	return newCreated, h.AddContent(hnt)
}

func (hc *HeaderController) GetHeaders(startingSlot, stopSlot uint64, limit int) (elements []structs.HeaderAndTrace, lastSlot uint64) {
	for _, o := range hc.getOrderedDesc() {
		if o > startingSlot || o < stopSlot {
			continue
		}
		hc.cl.RLock()
		h, ok := hc.headers[o]
		hc.cl.RUnlock()
		if !ok || h == nil {
			continue
		}
		lastSlot = o
		c, _, _ := h.GetContent()
		elements = append(elements, c...)
		if len(elements) >= limit {
			return elements, lastSlot
		}
	}

	return elements, lastSlot
}

func (hc *HeaderController) GetSingleSlot(slot uint64) (elements []structs.HeaderAndTrace, maxProfitHash [32]byte, revision uint64, err error) {
	hc.cl.RLock()
	h, ok := hc.headers[slot]
	hc.cl.RUnlock()

	if !ok {
		return nil, maxProfitHash, 0, nil
	}
	elements, maxProfitHash, revision = h.GetContent()
	return elements, maxProfitHash, revision, err
}

func (hc *HeaderController) RemoveSlot(slot, expectedRevision uint64) (success bool) {
	hc.cl.Lock()
	defer hc.cl.Unlock()

	s, ok := hc.headers[slot]
	if !ok {
		return true
	}

	if expectedRevision != s.GetRevision() {
		return false
	}

	// we're only unlinking from map here
	// it should be later on eligible for GC
	delete(hc.headers, slot)
	hc.m.HeadersSize.Set(float64(len(hc.headers)))

	// removed element should be first on the left
	if len(hc.ordered) > 0 {
		for i, v := range hc.ordered {
			if v.Slot == slot {
				hc.ordered = append(hc.ordered[:i], hc.ordered[i+1:]...)
				break
			}
		}
	}

	return true
}

func (hc *HeaderController) GetMaxProfit(slot uint64) (hnt structs.HeaderAndTrace, ok bool) {
	hc.cl.RLock()
	defer hc.cl.RUnlock()

	s, ok := hc.headers[slot]
	if !ok {
		return hnt, false
	}
	return s.GetMaxProfit()

}

type IndexMeta struct {
	Hash          [32]byte
	Value         *big.Int
	BuilderPubkey [48]byte
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

type IndexedHeaders struct {
	S StoredIndex

	rev                        uint64
	content                    []structs.HeaderAndTrace
	blockHashToContentPosition map[[32]byte]int
	contentLock                sync.RWMutex
}

func NewIndexedHeaders() (h *IndexedHeaders) {
	return &IndexedHeaders{
		S:                          NewStoreIndex(),
		blockHashToContentPosition: make(map[[32]byte]int),
	}
}

func (h *IndexedHeaders) GetRevision() (revision uint64) {
	h.contentLock.RLock()
	defer h.contentLock.RUnlock()
	return h.rev
}

func (h *IndexedHeaders) GetContent() (content []structs.HeaderAndTrace, maxProfitHash [32]byte, revision uint64) {
	h.contentLock.RLock()
	defer h.contentLock.RUnlock()

	return h.content, h.S.MaxProfit.Hash, h.rev
}

func (h *IndexedHeaders) GetMaxProfit() (hnt structs.HeaderAndTrace, ok bool) {
	h.contentLock.RLock()
	defer h.contentLock.RUnlock()

	n, ok := h.blockHashToContentPosition[h.S.MaxProfit.Hash]
	if !ok {
		return hnt, ok
	}
	if len(h.content) < n {
		return hnt, false
	}
	return h.content[n], true
}

func (h *IndexedHeaders) linkHash(hnt structs.HeaderAndTrace) {
	_, ok := h.blockHashToContentPosition[hnt.Trace.BlockHash]
	if !ok {
		h.content = append(h.content, hnt)
		h.blockHashToContentPosition[hnt.Trace.BlockHash] = len(h.content) - 1
	}
}

func (h *IndexedHeaders) addContent(newEl IndexMeta) error {
	h.S.Index = append(h.S.Index, newEl)
	h.S.SubmissionsByPubKeys[newEl.BuilderPubkey] = newEl

	// we should allow resubmission
	if h.S.MaxProfit.BuilderPubkey == newEl.BuilderPubkey {
		h.S.MaxProfit = IndexMeta{}
		for _, submission := range h.S.SubmissionsByPubKeys {
			if h.S.MaxProfit.Value == nil || h.S.MaxProfit.Value.Cmp(submission.Value) <= 0 {
				h.S.MaxProfit = submission
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

func (h *IndexedHeaders) AddContent(hnt structs.HeaderAndTrace) error {
	h.contentLock.Lock()
	defer h.contentLock.Unlock()

	newEl := IndexMeta{
		Hash:          hnt.Trace.BlockHash,
		Value:         hnt.Trace.Value.BigInt(),
		BuilderPubkey: hnt.Trace.BuilderPubkey,
	}

	h.linkHash(hnt)
	h.rev++
	return h.addContent(newEl)
}

func (h *IndexedHeaders) PrependContent(hnts []structs.HeaderAndTrace) error {
	h.contentLock.Lock()
	defer h.contentLock.Unlock()

	newIndex := h.S.Index[:]
	for _, hnt := range hnts {
		newEl := IndexMeta{
			Hash:          hnt.Trace.BlockHash,
			Value:         hnt.Trace.Value.BigInt(),
			BuilderPubkey: hnt.Trace.BuilderPubkey,
		}

		h.linkHash(hnt)
		if err := h.addContent(newEl); err != nil {
			h.S.Index = newIndex
			return err
		}
	}
	for _, ni := range newIndex {
		if err := h.addContent(ni); err != nil {
			h.S.Index = newIndex
			return err
		}
	}
	h.rev++
	return nil
}
