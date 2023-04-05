package data

import (
	"errors"
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

type ExportRequest struct {
	Dt   DataType
	Data []byte
	Id   string

	err chan error
}

func (req ExportRequest) Loggable() map[string]any {
	return map[string]any{
		"id": req.Id,
	}
}

type fileWithTimestamp struct {
	*os.File
	ts time.Time
}
