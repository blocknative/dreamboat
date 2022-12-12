package auction

import (
	"sync"

	"github.com/lthibault/log"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
)

type Auctioneer struct {
	l log.Logger

	mu                   [2]sync.RWMutex
	maxProfit            [2]*structs.CompleteBlockstruct
	latestBlockByBuilder [2]map[types.PublicKey]*structs.CompleteBlockstruct
}

func NewAuctioneer(l log.Logger) *Auctioneer {
	return &Auctioneer{
		l:                    l.WithField("relay-service", "auctioneer"),
		mu:                   [2]sync.RWMutex{},
		maxProfit:            [2]*structs.CompleteBlockstruct{},
		latestBlockByBuilder: [2]map[types.PublicKey]*structs.CompleteBlockstruct{make(map[types.PublicKey]*structs.CompleteBlockstruct), make(map[types.PublicKey]*structs.CompleteBlockstruct)},
	}
}

func (a *Auctioneer) AddBlock(block *structs.CompleteBlockstruct) {
	idx := block.Header.Trace.Slot % 2

	a.mu[idx].Lock()
	defer a.mu[idx].Unlock()

	a.latestBlockByBuilder[idx][block.Payload.Trace.Message.BuilderPubkey] = block

	// always set new value and bigger slot
	if a.maxProfit[idx] == nil || a.maxProfit[idx].Header.Trace.Slot < block.Header.Trace.Slot {
		a.maxProfit[idx] = block
		a.l.WithField("slot", block.Header.Trace.Slot).WithField("value", block.Header.Trace.Value.String()).Debug("new max bid")
		return
	}

	// always discard submissions lower than latest slot
	if a.maxProfit[idx].Header.Trace.Slot > block.Header.Trace.Slot {
		return
	}

	// accept bigger bid from different builder
	if a.maxProfit[idx].Header.Trace.BuilderPubkey != block.Header.Trace.BuilderPubkey &&
		a.maxProfit[idx].Header.Trace.Value.Cmp(&block.Header.Trace.Value) <= 0 {
		a.maxProfit[idx] = block
		a.l.WithField("slot", block.Header.Trace.Slot).WithField("value", block.Header.Trace.Value.String()).Debug("new max bid")
		return
	}

	// reassign biggest for resubmission from the same builder
	for _, b := range a.latestBlockByBuilder[idx] {
		if a.maxProfit[idx].Header.Trace.Slot == b.Header.Trace.Slot && // Only check the current slot
			a.maxProfit[idx].Header.Trace.Value.Cmp(&b.Header.Trace.Value) <= 0 {
			a.maxProfit[idx] = b
		}
	}

	block = a.maxProfit[idx]
	a.l.WithField("slot", block.Header.Trace.Slot).WithField("value", block.Header.Trace.Value.String()).Debug("new max bid")
}

func (a *Auctioneer) MaxProfitBlock(slot structs.Slot) (*structs.CompleteBlockstruct, bool) {
	idx := slot % 2

	a.mu[idx].RLock()
	defer a.mu[idx].RUnlock()

	if a.maxProfit[idx] != nil && structs.Slot(a.maxProfit[idx].Header.Trace.Slot) == slot {
		return a.maxProfit[idx], true
	}

	a.l.WithField("slot", slot).Debug("max profit not found")
	return nil, false
}
