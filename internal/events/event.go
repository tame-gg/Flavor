package events

import "time"

type Payload interface {
	eventPayload()
}

type Event struct {
	Sequence   uint64
	OccurredAt time.Time
	Payload    Payload
}

type Marker struct {
	Label string
}

func (Marker) eventPayload() {}
