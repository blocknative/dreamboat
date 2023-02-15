package types

import (
	"github.com/flashbots/go-boost-utils/types"
)

type BuilderBlockValidationRequest struct {
	types.BuilderSubmitBlockRequest
	RegisteredGasLimit uint64 `json:"registered_gas_limit,string"`
}
