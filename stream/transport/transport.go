package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

const (
	Unknown = iota
	AltairJson
	BellatrixJson
	CapellaJson
	CapellaSSZ
)

var (
	ErrDecodeVarint = errors.New("error decoding varint value")
)

type Subscription interface {
	Next(context.Context) (Message, error)
	Close() error
}

type Encoding uint16

func (f Encoding) String() string {
	switch f {
	case Unknown:
		return "none"
	case AltairJson:
		return "altair/json"
	case BellatrixJson:
		return "bellatrix/json"
	case CapellaJson:
		return "capella/json"
	case CapellaSSZ:
		return "capella/ssz"
	}

	return fmt.Sprintf("invalid: %d", f)
}

type Message struct {
	Source   uuid.UUID
	Encoding Encoding
	Payload  []byte
}

func (m Message) Loggable() map[string]any {
	return map[string]any{
		"source":        m.Source,
		"fork_encoding": m.Encoding,
		"n_bytes":       len(m.Payload),
	}
}

func Encode(m Message) ([]byte, error) {
	return json.Marshal(m)
}

func Decode(b []byte) (m Message, err error) {
	err = json.Unmarshal(b, &m)
	return
}
