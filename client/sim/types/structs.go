package types

import "github.com/blocknative/dreamboat/structs"

type BuilderBlockValidationRequest struct {
	structs.SubmitBlockRequest
	RegisteredGasLimit uint64 `json:"registered_gas_limit,string"`
}
