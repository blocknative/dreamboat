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
	maxProfit            *structs.CompleteBlockstruct
	latestBlockByBuilder map[types.PublicKey]*structs.CompleteBlockstruct
}

func NewAuctioneer() *Auctioneer {
	return &Auctioneer{
		auctions: [3]*Auction{
			{latestBlockByBuilder: make(map[types.PublicKey]*structs.CompleteBlockstruct)}, // slot - 1
			{latestBlockByBuilder: make(map[types.PublicKey]*structs.CompleteBlockstruct)}, // slot
			{latestBlockByBuilder: make(map[types.PublicKey]*structs.CompleteBlockstruct)}, // slot + 1
		},
	}
}

func (a *Auctioneer) AddBlock(block *structs.CompleteBlockstruct) bool {
	auction := a.auctions[block.Header.BidTrace().Slot%3]

	auction.mu.Lock()
	defer auction.mu.Unlock()

	//auction.latestBlockByBuilder[block.Payload.Trace.Message.BuilderPubkey] = block
	auction.latestBlockByBuilder[block.Header.BidTrace().BuilderPubkey] = block

	// always set new value and bigger slot
	if auction.maxProfit == nil || auction.maxProfit.Header.BidTrace().Slot < block.Header.BidTrace().Slot {
		auction.maxProfit = block
		return true
	}

	// always discard submissions lower than latest slot
	if auction.maxProfit.Header.BidTrace().Slot > block.Header.BidTrace().Slot {
		return false
	}

	// accept bigger bid
	value1 := auction.maxProfit.Header.BidTrace().Value
	value2 := block.Header.BidTrace().Value
	if value1.Cmp(&value2) <= 0 {
		auction.maxProfit = block
		return true
	}

	// reassign biggest for resubmission from the same builder with lower bid
	if auction.maxProfit.Header.BidTrace().BuilderPubkey == block.Header.BidTrace().BuilderPubkey &&
		value1.Cmp(&value2) > 0 {
		auction.maxProfit = block
		for _, b := range auction.latestBlockByBuilder {
			if auction.maxProfit.Header.BidTrace().Slot == b.Header.BidTrace().Slot && // Only check the current slot
				value1.Cmp(&value2) <= 0 {
				auction.maxProfit = b
			}
		}
	}

	return block == auction.maxProfit
}

func (a *Auctioneer) MaxProfitBlock(slot structs.Slot) (*structs.CompleteBlockstruct, bool) {
	auction := a.auctions[slot%3]

	auction.mu.RLock()
	defer auction.mu.RUnlock()

	if auction.maxProfit != nil && structs.Slot(auction.maxProfit.Header.BidTrace().Slot) == slot {
		return auction.maxProfit, true
	}

	return nil, false
}
