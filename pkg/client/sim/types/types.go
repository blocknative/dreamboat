package types

import (
	"encoding/json"

	"github.com/flashbots/go-boost-utils/types"
)

type RpcRawResponse struct {
	VersionTag string          `json:"jsonrpc"`
	Result     json.RawMessage `json:"result"`
	Error      *RpcError       `json:"error"`
	ID         uint64          `json:"id"`
}

type RpcRequest struct {
	VersionTag string        `json:"jsonrpc"`
	ID         uint64        `json:"id"`
	Method     string        `json:"method"`
	Params     []interface{} `json:"params"`
}

type RpcError struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
}

type ResponseAttached struct {
	Method   string
	Response RpcRawResponse
	Attached []byte
}

type Payload struct {
	Method   string
	Payload  []byte
	Attached []byte
}

type BuilderBlockValidationRequest struct {
	types.BuilderSubmitBlockRequest
	RegisteredGasLimit uint64 `json:"registered_gas_limit,string"`
}
