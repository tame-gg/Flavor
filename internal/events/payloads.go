package events

import (
	"git.lunarlabs.dev/lattice/lattice/internal/domain"
)

type NetworkStateChanged struct {
	NetworkID   domain.NetworkID
	State       domain.NetworkConnectionState
	SafeMessage string
}

func (NetworkStateChanged) eventPayload() {}

type AuthenticationRequired struct {
	NetworkID domain.NetworkID
	Provider  domain.ProviderType
	AuthURL   string
	FlowID    string
}

func (AuthenticationRequired) eventPayload() {}

type AuthenticationCompleted struct {
	NetworkID domain.NetworkID
}

func (AuthenticationCompleted) eventPayload() {}

type ApprovalRequired struct {
	NetworkID   domain.NetworkID
	Provider    domain.ProviderType
	SafeMessage string
}

func (ApprovalRequired) eventPayload() {}

type PeerAdded struct {
	Device domain.Device
}

func (PeerAdded) eventPayload() {}

type PeerUpdated struct {
	Device domain.Device
}

func (PeerUpdated) eventPayload() {}

type PeerRemoved struct {
	ID domain.DeviceIdentity
}

func (PeerRemoved) eventPayload() {}

type NetworkAdded struct {
	Network domain.Network
	State   domain.NetworkConnectionState
}

func (NetworkAdded) eventPayload() {}

type NetworkUpdated struct {
	Network domain.Network
}

func (NetworkUpdated) eventPayload() {}

type NetworkRemoved struct {
	NetworkID domain.NetworkID
}

func (NetworkRemoved) eventPayload() {}
