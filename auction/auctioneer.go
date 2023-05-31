package auction

import (
	"sync"

	"github.com/blocknative/dreamboat/structs"
	"github.com/flashbots/go-boost-utils/types"
)

type Auctioneer struct {
	auctions [3]*Auction
}

type Auction struct {
	mu                   sync.RWMutex
	maxProfit            map[types.Hash]*structs.CompleteBlockstruct
	latestBlockByBuilder map[LatestKey]*structs.CompleteBlockstruct
}

type LatestKey struct {
	Pk         types.PublicKey
	ParentHash types.Hash
}

func NewAuctioneer() *Auctioneer {
	return &Auctioneer{
		auctions: [3]*Auction{
			{ // slot - 1
				latestBlockByBuilder: make(map[LatestKey]*structs.CompleteBlockstruct),
				maxProfit:            make(map[types.Hash]*structs.CompleteBlockstruct),
			},
			{ // slot
				latestBlockByBuilder: make(map[LatestKey]*structs.CompleteBlockstruct),
				maxProfit:            make(map[types.Hash]*structs.CompleteBlockstruct),
			},
			{ // slot + 1
				latestBlockByBuilder: make(map[LatestKey]*structs.CompleteBlockstruct),
				maxProfit:            make(map[types.Hash]*structs.CompleteBlockstruct),
			},
		},
	}
}

func (a *Auctioneer) AddBlock(block *structs.CompleteBlockstruct) bool {
	auction := a.auctions[block.Header.Trace.Slot%3]
	parent := block.Header.Header.GetParentHash()
	auction.mu.Lock()
	defer auction.mu.Unlock()

	a.auctions[(block.Header.Trace.Slot-2)%3] = &Auction{
		latestBlockByBuilder: make(map[LatestKey]*structs.CompleteBlockstruct),
		maxProfit:            make(map[types.Hash]*structs.CompleteBlockstruct),
	}

	//auction.latestBlockByBuilder[block.Payload.Trace.Message.BuilderPubkey] = block
	auction.latestBlockByBuilder[LatestKey{ParentHash: parent, Pk: block.Header.Trace.BuilderPubkey}] = block

	mp, ok := auction.maxProfit[parent]

	// always set new value and bigger slot
	if !ok || mp.Header.Trace.Slot < block.Header.Trace.Slot {
		auction.maxProfit[parent] = block
		return true
	}

	// always discard submissions lower than latest slot
	if mp.Header.Trace.Slot > block.Header.Trace.Slot {
		return false
	}

	// accept bigger bid
	if mp.Header.Trace.Value.Cmp(&block.Header.Trace.Value) <= 0 {
		auction.maxProfit[parent] = block
		return true
	}

	// reassign biggest for resubmission from the same builder with lower bid
	if mp.Header.Trace.BuilderPubkey == block.Header.Trace.BuilderPubkey &&
		mp.Header.Trace.Value.Cmp(&block.Header.Trace.Value) > 0 {
		auction.maxProfit[parent] = block
		for _, b := range auction.latestBlockByBuilder {
			if mp.Header.Trace.Slot == b.Header.Trace.Slot && // Only check the current slot
				mp.Header.Trace.Value.Cmp(&b.Header.Trace.Value) <= 0 {
				auction.maxProfit[parent] = b
			}
		}
	}

	return block == mp
}

func (a *Auctioneer) MaxProfitBlock(slot structs.Slot, parentHash types.Hash) (*structs.CompleteBlockstruct, bool) {
	auction := a.auctions[slot%3]

	auction.mu.RLock()
	defer auction.mu.RUnlock()
	if auction.maxProfit != nil {
		if mp, ok := auction.maxProfit[parentHash]; ok && structs.Slot(mp.Header.Trace.Slot) == slot {
			return mp, true
		}
	}

	return nil, false
}
