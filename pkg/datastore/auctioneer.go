package datastore

import (
	"sync"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
)

type Auctioneer struct {
	mu             sync.RWMutex
	maxProfit      *structs.CompleteBlockstruct
	blockByBuilder map[types.PublicKey]*structs.CompleteBlockstruct
}

func NewAuctioneer() *Auctioneer {
	return &Auctioneer{
		blockByBuilder: make(map[types.PublicKey]*structs.CompleteBlockstruct),
	}
}

func (a *Auctioneer) UpdateMaxProfit(block *structs.CompleteBlockstruct) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.blockByBuilder[block.Payload.Trace.Message.BuilderPubkey] = block

	// we should allow resubmission
	if a.maxProfit != nil && a.maxProfit.Header.Trace.BuilderPubkey == block.Header.Trace.BuilderPubkey {
		for _, block := range a.blockByBuilder {
			if a.maxProfit.Header.Trace.Value.Cmp(&block.Header.Trace.Value) <= 0 {
				a.maxProfit = block
			}
		}
	} else {
		if a.maxProfit == nil ||
			a.maxProfit.Header.Trace.Value.Cmp(&block.Header.Trace.Value) < 0 {
			a.maxProfit = block
		}
	}
}

func (a *Auctioneer) GetMaxProfitBlock(slot structs.Slot) (*structs.CompleteBlockstruct, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.maxProfit != nil && structs.Slot(a.maxProfit.Header.Trace.Slot) == slot {
		return a.maxProfit, true
	}

	return nil, false
}
