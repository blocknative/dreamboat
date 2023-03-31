package beacon

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/blocknative/dreamboat/structs"
)

const (
	NumberOfSlotsInState = 2
)

type MultiSlotState struct {
	mu    sync.Mutex
	slots [NumberOfSlotsInState]AtomicState

	duties               atomic.Value
	validatorsUpdateTime atomic.Value
	headSlot             atomic.Value
	fork                 atomic.Value
	knownValidators      atomic.Value
	genesis              atomic.Value
}

func (as *MultiSlotState) Duties() structs.DutiesState {
	if val := as.duties.Load(); val != nil {
		return val.(structs.DutiesState)
	}

	return structs.DutiesState{}
}

func (as *MultiSlotState) SetDuties(duties structs.DutiesState) {
	as.duties.Store(duties)
}

func (as *MultiSlotState) Genesis() structs.GenesisInfo {
	if val := as.genesis.Load(); val != nil {
		return val.(structs.GenesisInfo)
	}

	return structs.GenesisInfo{}
}

func (as *MultiSlotState) SetGenesis(genesis structs.GenesisInfo) {
	as.genesis.Store(genesis)
}

func (as *MultiSlotState) KnownValidators() structs.ValidatorsState {
	if val := as.knownValidators.Load(); val != nil {
		return val.(structs.ValidatorsState)
	}

	return structs.ValidatorsState{}
}

func (as *MultiSlotState) SetKnownValidators(validators structs.ValidatorsState) {
	as.knownValidators.Store(validators)
	as.validatorsUpdateTime.Store(time.Now())
}

func (as *MultiSlotState) KnownValidatorsUpdateTime() time.Time {
	updateTime, ok := as.validatorsUpdateTime.Load().(time.Time)
	if !ok {
		return time.Time{}
	}
	return updateTime
}

func (as *MultiSlotState) HeadSlot() structs.Slot {
	if val := as.headSlot.Load(); val != nil {
		return val.(structs.Slot)
	}

	return 0
}

func (as *MultiSlotState) SetHeadSlot(headSlot structs.Slot) {
	as.headSlot.Store(headSlot)
}

func (as *MultiSlotState) Withdrawals(slot uint64) structs.WithdrawalsState {
	as.mu.Lock()
	defer as.mu.Unlock()

	if ws := as.slots[slot%NumberOfSlotsInState].Withdrawals(); uint64(ws.Slot) == slot {
		return ws
	}

	return structs.WithdrawalsState{}
}

func (as *MultiSlotState) SetWithdrawals(withdrawals structs.WithdrawalsState) {
	as.mu.Lock()
	defer as.mu.Unlock()

	as.slots[withdrawals.Slot%NumberOfSlotsInState].SetWithdrawals(withdrawals)
}

func (as *MultiSlotState) Randao(slot uint64) structs.RandaoState {
	as.mu.Lock()
	defer as.mu.Unlock()

	if randao := as.slots[slot%NumberOfSlotsInState].Randao(); randao.Slot == slot {
		return randao
	}
	return structs.RandaoState{}
}

func (as *MultiSlotState) SetRandao(randao structs.RandaoState) {
	as.mu.Lock()
	defer as.mu.Unlock()

	as.slots[randao.Slot%NumberOfSlotsInState].SetRandao(randao)
}

func (as *MultiSlotState) ForkVersion(slot structs.Slot) structs.ForkVersion {
	return as.Fork().Version(slot)
}

func (as *MultiSlotState) Fork() structs.ForkState {
	if val := as.fork.Load(); val != nil {
		return val.(structs.ForkState)
	}

	return structs.ForkState{}
}

func (as *MultiSlotState) SetFork(fork structs.ForkState) {
	as.fork.Store(fork)
}

type AtomicState struct {
	withdrawals atomic.Value
	randao      atomic.Value
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

func (as *AtomicState) Randao() structs.RandaoState {
	if val := as.randao.Load(); val != nil {
		return val.(structs.RandaoState)
	}

	return structs.RandaoState{}
}

func (as *AtomicState) SetRandao(randao structs.RandaoState) {
	as.randao.Store(randao)
}
