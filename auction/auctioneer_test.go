package auction

import (
	"testing"

	"github.com/blocknative/dreamboat/structs"
	"github.com/blocknative/dreamboat/structs/forks/capella"
	"github.com/flashbots/go-boost-utils/types"
)

func TestAuctioneer_AddBlock(t *testing.T) {
	auctioneer := NewAuctioneer()

	// Create a sample bid
	bid := capella.BuilderBidExtended{
		CapellaSlot: 10,
		CapellaBuilderBid: capella.BuilderBid{
			CapellaHeader: &capella.ExecutionPayloadHeader{
				ExecutionPayloadHeader: types.ExecutionPayloadHeader{
					ParentHash: types.Hash{},
				},
			},
			CapellaValue: types.IntToU256(10),
		},
	}

	// Test adding a block
	added := auctioneer.AddBlock(bid)
	if !added {
		t.Error("Expected block to be added")
	}

	// Test adding a block with a lower slot
	bidLowerSlot := capella.BuilderBidExtended{
		CapellaSlot: bid.CapellaSlot - structs.NumberOfSlotsInState,
		CapellaBuilderBid: capella.BuilderBid{
			CapellaHeader: &capella.ExecutionPayloadHeader{
				ExecutionPayloadHeader: types.ExecutionPayloadHeader{
					ParentHash: types.Hash{},
				},
			},
			CapellaValue: types.IntToU256(9),
		},
	}
	added = auctioneer.AddBlock(bidLowerSlot)
	if added {
		t.Error("Expected block with lower slot to be discarded")
	}

	// Test adding a block with a higher slot
	bidHigherSlot := capella.BuilderBidExtended{
		CapellaSlot: bid.CapellaSlot + structs.NumberOfSlotsInState,
		CapellaBuilderBid: capella.BuilderBid{
			CapellaHeader: &capella.ExecutionPayloadHeader{
				ExecutionPayloadHeader: types.ExecutionPayloadHeader{
					ParentHash: types.Hash{},
				},
			},
			CapellaValue: bid.CapellaBuilderBid.Value(),
		},
	}
	added = auctioneer.AddBlock(bidHigherSlot)
	if !added {
		t.Error("Expected block with higher slot to be added")
	}

	// Add a new block with the same parent hash and higher value
	bidHigherValue := capella.BuilderBidExtended{
		CapellaSlot: bidHigherSlot.CapellaSlot,
		CapellaBuilderBid: capella.BuilderBid{
			CapellaHeader: &capella.ExecutionPayloadHeader{
				ExecutionPayloadHeader: types.ExecutionPayloadHeader{
					ParentHash: types.Hash{},
				},
			},
			CapellaValue: types.IntToU256(100),
		},
	}
	added = auctioneer.AddBlock(bidHigherValue)
	if !added {
		t.Error("Expected block with higher value to be added")
	}

	// Add a new block with the same parent hash and lower value
	bidLowerValue := capella.BuilderBidExtended{
		CapellaSlot: bidHigherSlot.CapellaSlot,
		CapellaBuilderBid: capella.BuilderBid{
			CapellaHeader: &capella.ExecutionPayloadHeader{
				ExecutionPayloadHeader: types.ExecutionPayloadHeader{
					ParentHash: types.Hash{},
				},
			},
			CapellaValue: types.IntToU256(1),
		},
	}
	added = auctioneer.AddBlock(bidLowerValue)
	if added {
		t.Error("Expected block with lower value to be discarded")
	}
}

func TestAuctioneer_MaxProfitBlock(t *testing.T) {
	auctioneer := NewAuctioneer()

	// Add a block
	bid := capella.BuilderBidExtended{
		CapellaSlot: 0,
		CapellaBuilderBid: capella.BuilderBid{
			CapellaHeader: &capella.ExecutionPayloadHeader{
				ExecutionPayloadHeader: types.ExecutionPayloadHeader{
					ParentHash: types.Hash{},
				},
			},
			CapellaValue: types.IntToU256(10),
		},
	}
	auctioneer.AddBlock(bid)

	// Test retrieving the max profit block
	maxProfit, found := auctioneer.MaxProfitBlock(0, types.Hash{})
	if !found {
		t.Error("Expected max profit block to be found")
	}
	if maxProfit != bid {
		t.Error("Max profit block does not match the added block")
	}

	// Test retrieving max profit block with a different slot
	_, found = auctioneer.MaxProfitBlock(1, types.Hash{})
	if found {
		t.Error("Expected max profit block with different slot to not be found")
	}
}
