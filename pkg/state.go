package relay

import (
	"sync/atomic"

	"github.com/blocknative/dreamboat/pkg/structs"
)

type AtomicState struct {
	duties     atomic.Value
	validators atomic.Value
	genesis    atomic.Value
}

func (as *AtomicState) Beacon() *structs.BeaconState {
	return &structs.BeaconState{
		DutiesState:     as.duties.Load().(structs.DutiesState),
		ValidatorsState: as.validators.Load().(structs.ValidatorsState),
		GenesisInfo:     as.genesis.Load().(structs.GenesisInfo),
	}
}
