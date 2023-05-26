package beacon

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/blocknative/dreamboat/structs"
	"github.com/flashbots/go-boost-utils/types"
)

<<<<<<< Updated upstream
const (
	NumberOfSlotsInState = 2
)

=======
>>>>>>> Stashed changes
type MultiSlotState struct {
	mu    sync.Mutex
	slots [NumberOfSlotsInState]AtomicState

	duties               atomic.Value
	validatorsUpdateTime atomic.Value
	headSlot             atomic.Value
	fork                 atomic.Value
	knownValidators      atomic.Value
	genesis              atomic.Value
	parentBlockHash      atomic.Value
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

func (as *MultiSlotState) SetHeadSlotIfHigher(slot structs.Slot) (structs.Slot, bool) {
	as.mu.Lock()
	defer as.mu.Unlock()

<<<<<<< Updated upstream
	if ws := as.slots[slot%NumberOfSlotsInState].Withdrawals(); uint64(ws.Slot) == slot {
=======
	headSlot := as.headSlot.Load()
	if headSlot == nil || slot > headSlot.(structs.Slot) {
		as.headSlot.Store(slot)
		return slot, true
	}

	return headSlot.(structs.Slot), false
}

func (as *MultiSlotState) Withdrawals(slot uint64, parentHash types.Hash) structs.WithdrawalsState {
	as.mu.Lock()
	defer as.mu.Unlock()

	if ws := as.slots[slot%structs.NumberOfSlotsInState].Withdrawals(parentHash); uint64(ws.Slot) == slot {
>>>>>>> Stashed changes
		return ws
	}

	return structs.WithdrawalsState{}
}

func (as *MultiSlotState) SetWithdrawals(withdrawals structs.WithdrawalsState) {
	as.mu.Lock()
	defer as.mu.Unlock()

	as.slots[withdrawals.Slot%NumberOfSlotsInState].SetWithdrawals(withdrawals)
}

func (as *MultiSlotState) Randao(slot uint64, parentHash types.Hash) structs.RandaoState {
	as.mu.Lock()
	defer as.mu.Unlock()

<<<<<<< Updated upstream
	if randao := as.slots[slot%NumberOfSlotsInState].Randao(); randao.Slot == slot {
=======
	if randao := as.slots[slot%structs.NumberOfSlotsInState].Randao(parentHash); randao.Slot == slot {
>>>>>>> Stashed changes
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

func (as *MultiSlotState) ParentBlockHash() types.Hash {
	if val := as.parentBlockHash.Load(); val != nil {
		return val.(types.Hash)
	}

	return types.Hash{}
}

func (as *MultiSlotState) SetParentBlockHash(blockHash types.Hash) {
	as.parentBlockHash.Store(blockHash)
}

type AtomicState struct {
	sync.Mutex
	withdrawals map[types.Hash]structs.WithdrawalsState
	randao      map[types.Hash]structs.RandaoState
}

func (as *AtomicState) Withdrawals(parentHash types.Hash) structs.WithdrawalsState {
	as.Lock()
	defer as.Unlock()

	if w, ok := as.withdrawals[parentHash]; ok {
		return w
	}

	return structs.WithdrawalsState{}
}

func (as *AtomicState) SetWithdrawals(withdrawals structs.WithdrawalsState) {
	as.Lock()
	defer as.Unlock()

	as.withdrawals[withdrawals.ParentHash] = withdrawals
}

func (as *AtomicState) Randao(parentHash types.Hash) structs.RandaoState {
	as.Lock()
	defer as.Unlock()

	if r, ok := as.randao[parentHash]; ok {
		return r
	}

	return structs.RandaoState{}
}

func (as *AtomicState) SetRandao(randao structs.RandaoState) {
	as.Lock()
	defer as.Unlock()

	as.randao[randao.ParentHash] = randao
}
