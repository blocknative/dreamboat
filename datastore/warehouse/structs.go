package warehouse

import (
	"errors"
	"os"
	"time"
)

var (
	ErrUnknownType = errors.New("unknown data type")
)

type StoreRequest struct {
	DataType  string
	Data      []byte
	Slot      uint64
	Id        string
	Timestamp time.Time

	header []byte
}

func (req StoreRequest) Loggable() map[string]any {
	return map[string]any{
		"id":        req.Id,
		"timestamp": req.Timestamp,
		"dataType":  req.DataType,
		"slot":      req.Slot,
	}
}

type fileWithTimestamp struct {
	*os.File
	ts time.Time
}
