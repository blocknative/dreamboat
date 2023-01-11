package structs

import "errors"

var (
	ErrUnknownValue            = errors.New("value is unknown")
	ErrPayloadAlreadyDelivered = errors.New("slot payload already delivered")
	ErrNoPayloadFound          = errors.New("no payload found")
	ErrMissingRequest          = errors.New("req is nil")
	ErrMissingSecretKey        = errors.New("secret key is nil")
	ErrNoBuilderBid            = errors.New("no builder bid")
	ErrOldSlot                 = errors.New("requested slot is old")
	ErrBadHeader               = errors.New("invalid block header from datastore")
	ErrInvalidSignature        = errors.New("invalid signature")
	ErrStore                   = errors.New("failed to store")
	ErrMarshal                 = errors.New("failed to marshal")
	ErrInternal                = errors.New("internal server error")
	ErrUnknownValidator        = errors.New("unknown validator")
	ErrVerification            = errors.New("failed to verify")
	ErrInvalidTimestamp        = errors.New("invalid timestamp")
	ErrInvalidSlot             = errors.New("invalid slot")
	ErrEmptyBlock              = errors.New("block is empty")
)
