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
			latestBlockByBuilder: make(map[LatestKey]structs.BuilderBidExtended),
			maxProfit:            make(map[types.Hash]structs.BuilderBidExtended),
		}
	}

	return a

}

func (a *Auctioneer) AddBlock(bid structs.BuilderBidExtended) bool {
	auction := a.auctions[bid.Slot()%structs.NumberOfSlotsInState]

	auction.mu.Lock()
	defer auction.mu.Unlock()

	bbid := bid.BuilderBid()
	parent := bbid.Header().GetParentHash()

	//auction.latestBlockByBuilder[block.Payload.Trace.Message.BuilderPubkey] = block
	//auction.latestBlockByBuilder[bid.BuilderBid().Pubkey()] = bid
	auction.latestBlockByBuilder[LatestKey{ParentHash: parent, Pk: bbid.Pubkey()}] = bid
	mp, ok := auction.maxProfit[parent]

	// always set new value and bigger slot
	if !ok || mp.Slot() < bid.Slot() {
		auction.maxProfit[parent] = bid
		return true
	}

	// always discard submissions lower than latest slot
	if mp.Slot() > bid.Slot() {
		return false
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
			if mp.Slot() == b.Slot() { // Only check the current slot
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
