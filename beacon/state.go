package beacon

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/blocknative/dreamboat/structs"
)



type MultiSlotState struct {
	mu    sync.Mutex
	slots [structs.NumberOfSlotsInState]AtomicState

	headSlotPayloadAttributes atomic.Uint64
	duties                    atomic.Value
	validatorsUpdateTime      atomic.Value
	headSlot                  atomic.Value
	fork                      atomic.Value
	knownValidators           atomic.Value
	genesis                   atomic.Value
	parentBlockHash           atomic.Value
}

func (as *MultiSlotState) HeadSlotPayloadAttributes() uint64 {
	return as.headSlotPayloadAttributes.Load()
}
func (as *MultiSlotState) SetHeadSlotPayloadAttributesIfHigher(slot uint64) (uint64, bool) {
	as.mu.Lock()
	defer as.mu.Unlock()

	headSlot := as.headSlotPayloadAttributes.Load()
	if slot > headSlot {
		as.headSlotPayloadAttributes.Store(slot)
		return slot, true
	}

	return headSlot, false
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

	if ws := as.slots[slot%structs.NumberOfSlotsInState].Withdrawals(); uint64(ws.Slot) == slot {
		return ws
	}

	return structs.WithdrawalsState{}
}

func (as *MultiSlotState) SetWithdrawals(withdrawals structs.WithdrawalsState) {
	as.mu.Lock()
	defer as.mu.Unlock()

	as.slots[withdrawals.Slot%structs.NumberOfSlotsInState].SetWithdrawals(withdrawals)
}

func (as *MultiSlotState) Randao(slot uint64) structs.RandaoState {
	as.mu.Lock()
	defer as.mu.Unlock()

	if randao := as.slots[slot%structs.NumberOfSlotsInState].Randao(); randao.Slot == slot {
		return randao
	}
	return structs.RandaoState{}
}

func (as *MultiSlotState) SetRandao(randao structs.RandaoState) {
	as.mu.Lock()
	defer as.mu.Unlock()

	as.slots[randao.Slot%structs.NumberOfSlotsInState].SetRandao(randao)
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

func (as *MultiSlotState) ParentBlockHash() string {
	if val := as.parentBlockHash.Load(); val != nil {
		return val.(string)
	}

	return ""
}

func (as *MultiSlotState) SetParentBlockHash(blockHash string) {
	as.parentBlockHash.Store(blockHash)
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
