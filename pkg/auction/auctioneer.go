package auction

import (
	"sync"

	"github.com/lthibault/log"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
)

type Auctioneer struct {
	l log.Logger

	auctions [2]*Auction
}

type Auction struct {
	mu                   sync.RWMutex
	maxProfit            *structs.CompleteBlockstruct
	latestBlockByBuilder map[types.PublicKey]*structs.CompleteBlockstruct
}

func NewAuctioneer(l log.Logger) *Auctioneer {
	return &Auctioneer{
		l: l.WithField("relay-service", "auctioneer"),
		auctions: [2]*Auction{
			{latestBlockByBuilder: make(map[types.PublicKey]*structs.CompleteBlockstruct)}, // slot
			{latestBlockByBuilder: make(map[types.PublicKey]*structs.CompleteBlockstruct)}, // slot + 1
		},
	}
}

func (a *Auctioneer) AddBlock(block *structs.CompleteBlockstruct) {
	auction := a.auctions[block.Header.Trace.Slot%2]

	auction.mu.Lock()
	defer auction.mu.Unlock()

	auction.latestBlockByBuilder[block.Payload.Trace.Message.BuilderPubkey] = block

	// always set new value and bigger slot
	if auction.maxProfit == nil || auction.maxProfit.Header.Trace.Slot < block.Header.Trace.Slot {
		auction.maxProfit = block
		a.l.WithField("slot", block.Header.Trace.Slot).WithField("value", block.Header.Trace.Value.String()).Trace("new max bid")
		return
	}

	// always discard submissions lower than latest slot
	if auction.maxProfit.Header.Trace.Slot > block.Header.Trace.Slot {
		return
	}

	// accept bigger bid
	if auction.maxProfit.Header.Trace.Value.Cmp(&block.Header.Trace.Value) <= 0 {
		auction.maxProfit = block
		a.l.WithField("slot", block.Header.Trace.Slot).WithField("value", block.Header.Trace.Value.String()).Trace("new max bid")
		return
	}

	// reassign biggest for resubmission from the same builder with lower bid
	if auction.maxProfit.Header.Trace.BuilderPubkey == block.Header.Trace.BuilderPubkey &&
	auction.maxProfit.Header.Trace.Value.Cmp(&block.Header.Trace.Value) > 0{
		auction.maxProfit = block
		for _, b := range auction.latestBlockByBuilder {
		if auction.maxProfit.Header.Trace.Slot == b.Header.Trace.Slot && // Only check the current slot
			auction.maxProfit.Header.Trace.Value.Cmp(&b.Header.Trace.Value) <= 0 {
			auction.maxProfit = b
		}
	}

	block = auction.maxProfit
	a.l.WithField("slot", block.Header.Trace.Slot).WithField("value", block.Header.Trace.Value.String()).Trace("new max bid")
}

func (a *Auctioneer) MaxProfitBlock(slot structs.Slot) (*structs.CompleteBlockstruct, bool) {
	auction := a.auctions[slot%2]

	auction.mu.RLock()
	defer auction.mu.RUnlock()

	if auction.maxProfit != nil && structs.Slot(auction.maxProfit.Header.Trace.Slot) == slot {
		return auction.maxProfit, true
	}

	return nil, false
}
