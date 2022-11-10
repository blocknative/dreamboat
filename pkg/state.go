package relay

import (
	"sync/atomic"

	"github.com/blocknative/dreamboat/pkg/structs"
)

type AtomicState struct {
	duties     atomic.Value
	validators atomic.Value
}

func (as *AtomicState) Beacon() *structs.BeaconState {
	duties := as.duties.Load().(structs.DutiesState)
	validators := as.validators.Load().(structs.ValidatorsState)
	return &structs.BeaconState{DutiesState: duties, ValidatorsState: validators}
}
