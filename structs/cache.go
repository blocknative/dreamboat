package structs

import (
	"fmt"

	lru "github.com/hashicorp/golang-lru/v2"
)

type PayloadCache interface {
	ContainsOrAdd(PayloadKey, BlockBidAndTrace) (ok, evicted bool)
	Add(PayloadKey, BlockBidAndTrace) (evicted bool)
	Get(PayloadKey) (BlockBidAndTrace, bool)
}

type MultiSlotPayloadCache [NumberOfSlotsInState]PayloadCache

func NewMultiSlotPayloadCache(cacheSize int) (c MultiSlotPayloadCache, err error) {
	for i := 0; i < NumberOfSlotsInState; i++ {
		payloadCache, err := lru.New[PayloadKey, BlockBidAndTrace](cacheSize)
		if err != nil {
			return MultiSlotPayloadCache{}, fmt.Errorf("failed to initialize cache: %w", err)
		}
		c[i] = payloadCache
	}

	return
}

func (c MultiSlotPayloadCache) ContainsOrAdd(key PayloadKey, bbt BlockBidAndTrace) (ok, evicted bool) {
	return c[key.Slot%NumberOfSlotsInState].ContainsOrAdd(key, bbt)
}
func (c MultiSlotPayloadCache) Add(key PayloadKey, bbt BlockBidAndTrace) (evicted bool) {
	return c[key.Slot%NumberOfSlotsInState].Add(key, bbt)
}
func (c MultiSlotPayloadCache) Get(key PayloadKey) (BlockBidAndTrace, bool) {
	return c[key.Slot%NumberOfSlotsInState].Get(key)
}
