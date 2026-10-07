package domain

import (
	"fmt"
	"net/netip"
	"time"
)

type DeviceIdentity struct {
	NetworkID NetworkID
	NodeID    NodeID
}

func (d DeviceIdentity) Key() string {
	return fmt.Sprintf("%s:%s", d.NetworkID, d.NodeID)
}

type Device struct {
	ID        DeviceIdentity
	Hostname  string
	DNSName   string
	Addresses []netip.Addr
	Online    bool
	LastSeen  time.Time
	Local     bool
	OS        string
	Tags      []string
}
