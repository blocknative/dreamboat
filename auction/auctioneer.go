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
	Slot                 uint64
	maxProfit            map[types.Hash]structs.BuilderBidExtended
	latestBlockByBuilder map[LatestKey]structs.BuilderBidExtended
}

type LatestKey struct {
	Pk         types.PublicKey
	ParentHash types.Hash
}

func NewAuctioneer() *Auctioneer {
	a := &Auctioneer{}
	for i := 0; i < structs.NumberOfSlotsInState; i++ {
		a.auctions[i] = &Auction{
			Slot:                 uint64(i),
			latestBlockByBuilder: make(map[LatestKey]structs.BuilderBidExtended),
			maxProfit:            make(map[types.Hash]structs.BuilderBidExtended),
		}
	}

	return a

}

func (a *Auctioneer) AddBlock(bid structs.BuilderBidExtended) bool {
	auction := a.auctions[bid.Slot()%structs.NumberOfSlotsInState]
	parent := bid.BuilderBid().Header().GetParentHash()
	bbid := bid.BuilderBid()

	auction.mu.Lock()
	defer auction.mu.Unlock()

	// always discard submissions lower than latest slot
	if auction.Slot > bid.Slot() {
		return false
	}

	// always set new value and bigger slot
	if auction.Slot < bid.Slot() {
		a.auctions[bid.Slot()%structs.NumberOfSlotsInState] = &Auction{
			Slot:                 bid.Slot(),
			latestBlockByBuilder: make(map[LatestKey]structs.BuilderBidExtended),
			maxProfit:            make(map[types.Hash]structs.BuilderBidExtended),
		}
		auction.maxProfit[parent] = bid
		auction.latestBlockByBuilder[LatestKey{ParentHash: parent, Pk: bbid.Pubkey()}] = bid
		return true
	}

	auction.latestBlockByBuilder[LatestKey{ParentHash: parent, Pk: bbid.Pubkey()}] = bid
	mp, ok := auction.maxProfit[parent]
	if !ok {
		auction.maxProfit[parent] = bid
		return true
	}

	// accept bigger bid
	maxBidValue := mp.BuilderBid().Value()
	bidValue := bid.BuilderBid().Value()
	if maxBidValue.Cmp(&bidValue) <= 0 {
		auction.maxProfit[parent] = bid
		return true
	}

	// reassign biggest for resubmission from the same builder with lower bid
	if mp.BuilderBid().Pubkey() == bbid.Pubkey() &&
		maxBidValue.Cmp(&bidValue) > 0 {
		auction.maxProfit[parent] = bid
		for _, b := range auction.latestBlockByBuilder {
			if b.BuilderBid().Header().GetParentHash() == parent && mp.Slot() == b.Slot() { // Only check the current slot
				mp, ok := auction.maxProfit[parent]
				if !ok {
					continue
				}
				bidValue := b.BuilderBid().Value()
				maxBidValue = mp.BuilderBid().Value()
				if maxBidValue.Cmp(&bidValue) <= 0 {
					auction.maxProfit[parent] = b
				}
			}
		}
	}
	return bid == mp
}

func (a *Auctioneer) MaxProfitBlock(slot structs.Slot, parentHash types.Hash) (structs.BuilderBidExtended, bool) {
	auction := a.auctions[slot%structs.NumberOfSlotsInState]

	auction.mu.RLock()
	defer auction.mu.RUnlock()

	if auction.maxProfit != nil {
		if mp, ok := auction.maxProfit[parentHash]; ok && structs.Slot(mp.Slot()) == slot {
			return mp, true
		}
	}

	return nil, false
}
