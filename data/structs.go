package data

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
)

var (
	ErrUnknownType = errors.New("unknown data type")
)

type DataType uint8

const (
	BlockBidAndTraceData DataType = iota
)

func toString(data DataType) string {
	switch data {
	case BlockBidAndTraceData:
		return "BlockBidAndTraceData"
	default:
		return "unknown"
	}
}

type ExportMeta struct {
	Slot      uint64
	Timestamp time.Time
}

type exportRequest struct {
	dt   DataType
	data []byte
	id   string
	meta ExportMeta
	err  chan error
}

func (req exportRequest) Loggable() map[string]any {
	return map[string]any{
		"id":        req.id,
		"slot":      req.meta.Slot,
		"timestamp": req.meta.Timestamp.String(),
	}
}

type exportFiles struct {
	BlockBidAndTrace *os.File
}

type exportEncoders struct {
	BlockBidAndTrace *json.Encoder
}

type exportBuffers struct {
	BlockBidAndTrace *bufio.Writer
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

type writer struct {
	file       *os.File
	encoder    io.WriteCloser
	compressor *gzip.Writer
}
