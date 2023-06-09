package client

import "errors"

var (
	ErrNotFound          = errors.New("network not found")
	ErrConnectionFailure = errors.New("connection failure")
)
