package transport

import (
	"context"
	"encoding/binary"
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

type ForkVersionFormat uint64

type Message struct {
	Source       uuid.UUID
	ForkEncoding ForkVersionFormat
	Payload      []byte
}

func Encode(m Message) ([]byte, error) {
	rawItem, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	// encode the varint with a variable size
	varintBytes := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(varintBytes, CapellaJson)
	varintBytes = varintBytes[:n]

	// append the varint
	return append(varintBytes, rawItem...), nil
}

func Decode(b []byte) (Message, error) {
	varint, n := binary.Uvarint(b)
	if n <= 0 {
		return Message{}, ErrDecodeVarint
	}

	b = b[n:]

	switch ForkVersionFormat(varint) {
	case BellatrixJson, CapellaJson:
		var msg Message
		if err := json.Unmarshal(b, &msg); err != nil {
			return Message{}, err
		}
		return msg, nil

	default:
		return Message{}, fmt.Errorf("invalid fork version: %d", varint)
	}
}
