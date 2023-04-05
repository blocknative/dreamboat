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
	GetPayloadRequest DataType = iota
	SubmitBlockRequest
)

func toString(data DataType) string {
	switch data {
	case GetPayloadRequest:
		return "GetPayloadRequest"
	case SubmitBlockRequest:
		return "SubmitBlockRequest"
	default:
		return "unknown"
	}
}

type ExportRequest struct {
	DataType DataType
	Data     []byte
	Slot     uint64
	Id       string

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
