package data

import (
	"encoding/json"
	"errors"
	"os"
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

type exportFiles struct {
	BlockBidAndTrace *os.File
}

func (f exportFiles) Close() {
	f.BlockBidAndTrace.Close()
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
