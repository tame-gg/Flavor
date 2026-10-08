package events

import (
	"git.lunarlabs.dev/flavor/flavor/internal/domain"
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

type WorkspaceChanged struct {
	Workspace domain.Workspace
}

func (WorkspaceChanged) eventPayload() {}

type WorkspaceRemoved struct {
	WorkspaceID domain.WorkspaceID
}

func (WorkspaceRemoved) eventPayload() {}

type ActiveWorkspaceChanged struct {
	WorkspaceID domain.WorkspaceID
}

func (ActiveWorkspaceChanged) eventPayload() {}

type DestinationPreferenceChanged struct {
	Preference domain.DestinationPreference
}

func (DestinationPreferenceChanged) eventPayload() {}

type DestinationPreferenceRemoved struct {
	Destination string
}

func (DestinationPreferenceRemoved) eventPayload() {}

type DaemonWarning struct {
	Code        string
	SafeMessage string
}

func (DaemonWarning) eventPayload() {}
