package structs

import "errors"

var (
	ErrUnknownValue            = errors.New("value is unknown")
	ErrPayloadAlreadyDelivered = errors.New("the slot payload was already delivered")
)
