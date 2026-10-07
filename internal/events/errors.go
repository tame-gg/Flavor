package events

import "errors"

var (
	ErrResyncRequired  = errors.New("event resync required")
	ErrClosed          = errors.New("event bus closed")
	ErrInvalidSequence = errors.New("invalid event sequence")
)
