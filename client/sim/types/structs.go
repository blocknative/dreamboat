package types

import (
	"github.com/blocknative/dreamboat/structs/forks/bellatrix"
	"github.com/blocknative/dreamboat/structs/forks/capella"
	"github.com/ethereum/go-ethereum/common"
)

type BuilderBlockValidationRequest struct {
	*bellatrix.SubmitBlockRequest
	RegisteredGasLimit uint64 `json:"registered_gas_limit,string"`
}

type BuilderBlockValidationRequestV2 struct {
	*capella.SubmitBlockRequest
	WithdrawalsRoot    common.Hash `json:"withdrawals_root"`
	RegisteredGasLimit uint64      `json:"registered_gas_limit,string"`
}
