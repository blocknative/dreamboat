package types

import "github.com/blocknative/dreamboat/pkg/structs"

type BuilderBlockValidationRequest struct {
	structs.SubmitBlockRequest
	RegisteredGasLimit uint64 `json:"registered_gas_limit,string"`
}
