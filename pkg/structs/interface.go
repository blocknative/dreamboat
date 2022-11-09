package structs

import "github.com/flashbots/go-boost-utils/types"

type State interface {
	Beacon() BeaconState
}

type BeaconState interface {
	KnownValidatorByIndex(uint64) (types.PubkeyHex, error)
	IsKnownValidator(types.PubkeyHex) (bool, error)
	HeadSlot() Slot
	ValidatorsMap() BuilderGetValidatorsResponseEntrySlice
}
