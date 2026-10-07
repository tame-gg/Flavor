package domain

import "time"

type DestinationKind string

const (
	DestinationAddress DestinationKind = "address"
	DestinationName    DestinationKind = "name"
)

type DestinationPreference struct {
	Destination string
	Kind        DestinationKind
	NetworkID   NetworkID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
