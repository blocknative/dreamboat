package auction

import (
	"sync"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
)

type Auctioneer struct {
	mu                   sync.RWMutex
	maxProfit            *structs.CompleteBlockstruct
	latestBlockByBuilder map[types.PublicKey]*structs.CompleteBlockstruct
}

func NewAuctioneer() *Auctioneer {
	return &Auctioneer{
		latestBlockByBuilder: make(map[types.PublicKey]*structs.CompleteBlockstruct),
	}
}

func (a *Auctioneer) AddBlock(block *structs.CompleteBlockstruct)  {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.latestBlockByBuilder[block.Payload.Trace.Message.BuilderPubkey] = block

	// always set new value and bigger slot
	if a.maxProfit == nil || a.maxProfit.Header.Trace.Slot < block.Header.Trace.Slot {
		a.maxProfit = block
		return
	}

	// always discard submissions lower than latest slot
	if a.maxProfit.Header.Trace.Slot > block.Header.Trace.Slot {
		return
	}

	// accept bigger bid from different builder
	if a.maxProfit.Header.Trace.BuilderPubkey != block.Header.Trace.BuilderPubkey &&
		a.maxProfit.Header.Trace.Value.Cmp(&block.Header.Trace.Value) <= 0 {
		a.maxProfit = block
		return
	}

	// reassign biggest for resubmission from the same builder
	for _, b := range a.latestBlockByBuilder {
		if a.maxProfit.Header.Trace.Slot == b.Header.Trace.Slot && // Only check the current slot
			a.maxProfit.Header.Trace.Value.Cmp(&b.Header.Trace.Value) <= 0 {
			a.maxProfit = b
		}
	}
}

func (a *Auctioneer) MaxProfitBlock(slot structs.Slot) (*structs.CompleteBlockstruct, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.maxProfit != nil && structs.Slot(a.maxProfit.Header.Trace.Slot) == slot {
		return a.maxProfit, true
	}

	return nil, false
}
