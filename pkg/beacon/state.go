package beacon

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
)

type AtomicState struct {
	duties               atomic.Value
	knownValidators      atomic.Value
	validatorsUpdateTime atomic.Value
	genesis              atomic.Value
	headSlot             atomic.Value
	withdrawals          atomic.Value

	// is the state initialized?
	once  sync.Once
	ready chan struct{}
}

func (as *AtomicState) Genesis() structs.GenesisInfo {
	if val := as.genesis.Load(); val != nil {
		return val.(structs.GenesisInfo)
	}

	return structs.GenesisInfo{}
}

func (as *AtomicState) SetGenesis(genesis structs.GenesisInfo) {
	as.genesis.Store(genesis)
}

func (as *AtomicState) Duties() structs.DutiesState {
	if val := as.duties.Load(); val != nil {
		return val.(structs.DutiesState)
	}

	return structs.DutiesState{}
}

func (as *AtomicState) SetDuties(duties structs.DutiesState) {
	as.duties.Store(duties)
}

func (as *AtomicState) KnownValidators() structs.ValidatorsState {
	if val := as.knownValidators.Load(); val != nil {
		return val.(structs.ValidatorsState)
	}

	return structs.ValidatorsState{}
}

func (as *AtomicState) SetKnownValidators(validators structs.ValidatorsState) {
	as.knownValidators.Store(validators)
	as.validatorsUpdateTime.Store(time.Now())
}

func (as *AtomicState) KnownValidatorsUpdateTime() time.Time {
	updateTime, ok := as.validatorsUpdateTime.Load().(time.Time)
	if !ok {
		return time.Time{}
	}
	return updateTime
}

func (as *AtomicState) HeadSlot() structs.Slot {
	if val := as.headSlot.Load(); val != nil {
		return val.(structs.Slot)
	}

	return 0
}

func (as *AtomicState) SetHeadSlot(headSlot structs.Slot) {
	as.headSlot.Store(headSlot)
}

func (as *AtomicState) Withdrawals() structs.WithdrawalsState {
	if val := as.withdrawals.Load(); val != nil {
		return val.(structs.WithdrawalsState)
	}

	return structs.WithdrawalsState{}
}

func (as *AtomicState) SetWithdrawals(withdrawals structs.WithdrawalsState) {
	as.withdrawals.Store(withdrawals)
}

func (as *AtomicState) Ready() <-chan struct{} {
	as.once.Do(func() {
		as.ready = make(chan struct{})
	})
	return as.ready
}

func (as *AtomicState) SetReady() {
	select {
	case <-as.Ready():
	default:
		close(as.ready)
	}
}
