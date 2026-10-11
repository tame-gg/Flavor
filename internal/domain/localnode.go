package domain

import (
	"net/netip"
	"time"
)

type LocalNode struct {
	NetworkID NetworkID
	NodeID    NodeID
	Hostname  string
	DNSName   string
	Addresses []netip.Addr
	Routes    []AdvertisedRoute
	ExitNode  AdvertisedExitNode
}

type AuthPrompt struct {
	FlowID    string
	URL       string
	Provider  ProviderType
	CreatedAt time.Time
}

type SessionDiagnostics struct {
	NetworkID        NetworkID
	Provider         ProviderType
	ConnectionState  NetworkConnectionState
	BackendLifecycle string
	ControlHost      string
	LocalAddrCount   int
	DeviceCount      int
	HasAuthPrompt    bool
}
