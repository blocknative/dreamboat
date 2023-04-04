package data

import (
	"errors"
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

type exportRequest struct {
	dt        DataType
	data      []byte
	id        string
	slot      uint64
	timestamp time.Time

	err chan error
}

func (req exportRequest) Loggable() map[string]any {
	return map[string]any{
		"id":        req.id,
		"slot":      req.slot,
		"timestamp": req.timestamp.String(),
	}
}
