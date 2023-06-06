package structs

import (
	"time"
)

const (
	SlotsPerEpoch        Slot = 32
	DurationPerSlot           = time.Second * 12
	DurationPerEpoch          = DurationPerSlot * time.Duration(SlotsPerEpoch)
	NumberOfSlotsInState      = 2
)
