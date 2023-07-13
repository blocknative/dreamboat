package structs

import (
	"time"
)

const (
	SlotsPerEpochSlot    Slot = 32
	SlotsPerEpoch             = 32
	DurationPerSlot           = time.Second * 12
	DurationPerEpoch          = DurationPerSlot * time.Duration(SlotsPerEpoch)
	NumberOfSlotsInState      = 2
)
