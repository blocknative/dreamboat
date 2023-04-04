package data

import (
	"encoding/json"
	"errors"
)

var (
	ErrUnknownType = errors.New("unknown data type")
)

type DataType uint8

const (
	BlockBidAndTraceData DataType = iota
)

type exportRequest struct {
	dt     DataType
	data   any
	caller string
	err    chan error
}

type exportEncoders struct {
	BlockBidAndTrace *json.Encoder
}

func selectEncoder(req exportRequest, encoders exportEncoders) (*json.Encoder, error) {
	switch req.dt {
	case BlockBidAndTraceData:
		return encoders.BlockBidAndTrace, nil
	}

	return nil, ErrUnknownType
}

type dataWithCaller struct {
	Data   any    `json:"data"`
	Caller string `json:"caller"`
}
