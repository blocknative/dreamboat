package auction

import (
	"sync"

	"github.com/blocknative/dreamboat/structs"
	"github.com/flashbots/go-boost-utils/types"
)

type Auctioneer struct {
	auctions [structs.NumberOfSlotsInState]*Auction
}

type Auction struct {
	mu                   sync.RWMutex
	maxProfit            structs.BuilderBidExtended
	latestBlockByBuilder map[types.PublicKey]structs.BuilderBidExtended
}

func NewAuctioneer() *Auctioneer {
	a := &Auctioneer{}
	for i := 0; i < structs.NumberOfSlotsInState; i++ {
		a.auctions[i] = &Auction{latestBlockByBuilder: make(map[types.PublicKey]structs.BuilderBidExtended)}
	}

	return a
}

func (a *Auctioneer) AddBlock(bid structs.BuilderBidExtended) bool {
	auction := a.auctions[bid.Slot%3]

	auction.mu.Lock()
	defer auction.mu.Unlock()

	//auction.latestBlockByBuilder[block.Payload.Trace.Message.BuilderPubkey] = block
	auction.latestBlockByBuilder[bid.BuilderBid.Pubkey()] = bid

	// always set new value and bigger slot
	if auction.maxProfit.BuilderBid == nil || auction.maxProfit.Slot < bid.Slot {
		auction.maxProfit = bid
		return true
	}

	// always discard submissions lower than latest slot
	if auction.maxProfit.Slot > bid.Slot {
		return false
	}

	// accept bigger bid
	bidValue := bid.BuilderBid.Value()
	maxBidValue := auction.maxProfit.BuilderBid.Value()
	if maxBidValue.Cmp(&bidValue) <= 0 {
		auction.maxProfit = bid
		return true
	}

	// reassign biggest for resubmission from the same builder with lower bid
	if auction.maxProfit.BuilderBid.Pubkey() == bid.BuilderBid.Pubkey() &&
		maxBidValue.Cmp(&bidValue) > 0 {
		auction.maxProfit = bid
		for _, b := range auction.latestBlockByBuilder {
			maxBidValue := auction.maxProfit.BuilderBid.Value()
			bidValue := b.BuilderBid.Value()
			if auction.maxProfit.Slot == b.Slot && // Only check the current slot
				maxBidValue.Cmp(&bidValue) <= 0 {
				auction.maxProfit = b
			}
		}
	}

	return bid == auction.maxProfit
}

func (a *Auctioneer) MaxProfitBlock(slot structs.Slot) (structs.BuilderBidExtended, bool) {
	auction := a.auctions[slot%3]

	auction.mu.RLock()
	defer auction.mu.RUnlock()

	if auction.maxProfit.BuilderBid != nil && structs.Slot(auction.maxProfit.Slot) == slot {
		return auction.maxProfit, true
	}

	return structs.BuilderBidExtended{}, false
}
