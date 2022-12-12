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

func (a *Auctioneer) AddBlock(block *structs.CompleteBlockstruct) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.latestBlockByBuilder[block.Payload.Trace.Message.BuilderPubkey] = block

	if a.maxProfit == nil || a.maxProfit.Header.Trace.Slot < block.Header.Trace.Slot {
		a.maxProfit = block
	} else if a.maxProfit != nil && a.maxProfit.Header.Trace.BuilderPubkey == block.Header.Trace.BuilderPubkey {
		for _, block := range a.latestBlockByBuilder {
			if a.maxProfit.Header.Trace.Value.Cmp(&block.Header.Trace.Value) <= 0 {
				a.maxProfit = block
			}
		}
	} else if a.maxProfit.Header.Trace.Value.Cmp(&block.Header.Trace.Value) <= 0 {
		a.maxProfit = block
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
