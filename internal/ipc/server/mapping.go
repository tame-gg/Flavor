package server

import (
	"time"

	v1 "git.lunarlabs.dev/lattice/lattice/gen/go/lattice/v1"
	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/events"
	"git.lunarlabs.dev/lattice/lattice/internal/service"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func timestamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

var capabilities = map[string]v1.Capability{
	"headscale":               v1.Capability_CAPABILITY_HEADSCALE,
	"device_snapshots":        v1.Capability_CAPABILITY_DEVICE_SNAPSHOTS,
	"connection_inspector":    v1.Capability_CAPABILITY_CONNECTION_INSPECTOR,
	"conflict_center":         v1.Capability_CAPABILITY_CONFLICT_CENTER,
	"workspaces":              v1.Capability_CAPABILITY_WORKSPACES,
	"destination_preferences": v1.Capability_CAPABILITY_DESTINATION_PREFERENCES,
	"forwarding":              v1.Capability_CAPABILITY_FORWARDING,
	"socks_proxy":             v1.Capability_CAPABILITY_SOCKS_PROXY,
}

func daemonInfo(info service.DaemonInfo) *v1.GetDaemonInfoResponse {
	out := &v1.GetDaemonInfoResponse{
		DaemonVersion:    info.DaemonVersion,
		ProtocolMajor:    uint32(info.ProtocolMajor),
		ProtocolMinor:    uint32(info.ProtocolMinor),
		DaemonInstanceId: info.InstanceID,
		BuildCommit:      info.BuildCommit,
	}
	for _, c := range info.Capabilities {
		if v, ok := capabilities[c]; ok {
			out.Capabilities = append(out.Capabilities, v)
		}
	}
	return out
}

func providerToProto(p domain.ProviderType) v1.ProviderType {
	switch p {
	case domain.ProviderTailscale:
		return v1.ProviderType_PROVIDER_TYPE_TAILSCALE
	case domain.ProviderHeadscale:
		return v1.ProviderType_PROVIDER_TYPE_HEADSCALE
	}
	return v1.ProviderType_PROVIDER_TYPE_UNSPECIFIED
}

func providerFromProto(p v1.ProviderType) (domain.ProviderType, bool) {
	switch p {
	case v1.ProviderType_PROVIDER_TYPE_TAILSCALE:
		return domain.ProviderTailscale, true
	case v1.ProviderType_PROVIDER_TYPE_HEADSCALE:
		return domain.ProviderHeadscale, true
	}
	return "", false
}

var states = map[domain.NetworkConnectionState]v1.NetworkConnectionState{
	domain.StateDisabled:         v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_DISABLED,
	domain.StateDisconnected:     v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_DISCONNECTED,
	domain.StateConnecting:       v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_CONNECTING,
	domain.StateAuthenticating:   v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_AUTHENTICATING,
	domain.StateAwaitingApproval: v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_AWAITING_APPROVAL,
	domain.StateConnected:        v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_CONNECTED,
	domain.StateDegraded:         v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_DEGRADED,
	domain.StateReconnecting:     v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_RECONNECTING,
	domain.StateRemoving:         v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_REMOVING,
	domain.StateError:            v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_ERROR,
}

func networkConfig(n domain.Network) *v1.Network {
	return &v1.Network{
		Id:           string(n.ID),
		DisplayName:  n.DisplayName,
		Provider:     providerToProto(n.Provider),
		ControlUrl:   n.ControlURL,
		AutoConnect:  n.AutoConnect,
		NodeHostname: n.NodeHostname,
		Label:        domain.NetworkLabel(n.DisplayName),
		CreatedAt:    timestamp(n.CreatedAt),
		UpdatedAt:    timestamp(n.UpdatedAt),
	}
}

func network(v service.NetworkView) *v1.Network {
	out := networkConfig(v.Network)
	out.State = states[v.State]
	switch {
	case v.Prompt != nil:
		out.Authentication = &v1.AuthenticationPrompt{
			FlowId:    v.Prompt.FlowID,
			Kind:      "authentication",
			AuthUrl:   v.Prompt.URL,
			Provider:  providerToProto(v.Prompt.Provider),
			CreatedAt: timestamp(v.Prompt.CreatedAt),
		}
	case v.State == domain.StateAwaitingApproval:
		out.Authentication = &v1.AuthenticationPrompt{Kind: "approval", Provider: providerToProto(v.Network.Provider)}
	}
	return out
}

func device(d domain.Device) *v1.Device {
	out := &v1.Device{
		Id:       &v1.DeviceIdentity{NetworkId: string(d.ID.NetworkID), NodeId: string(d.ID.NodeID)},
		Hostname: d.Hostname,
		DnsName:  d.DNSName,
		Online:   d.Online,
		LastSeen: timestamp(d.LastSeen),
		Local:    d.Local,
		Os:       d.OS,
		Tags:     append([]string(nil), d.Tags...),
	}
	for _, r := range d.Routes {
		out.Routes = append(out.Routes, r.String())
	}
	for _, a := range d.Addresses {
		out.Addresses = append(out.Addresses, a.String())
	}
	return out
}

func event(ev events.Event, instance string) *v1.DaemonEvent {
	out := &v1.DaemonEvent{
		DaemonInstanceId: instance,
		SequenceId:       ev.Sequence,
		Timestamp:        timestamp(ev.OccurredAt),
	}
	switch p := ev.Payload.(type) {
	case events.NetworkAdded:
		n := networkConfig(p.Network)
		n.State = states[p.State]
		out.Payload = &v1.DaemonEvent_NetworkAdded{NetworkAdded: &v1.NetworkAdded{Network: n}}
	case events.NetworkUpdated:
		out.Payload = &v1.DaemonEvent_NetworkUpdated{NetworkUpdated: &v1.NetworkUpdated{Network: networkConfig(p.Network)}}
	case events.NetworkRemoved:
		out.Payload = &v1.DaemonEvent_NetworkRemoved{NetworkRemoved: &v1.NetworkRemoved{NetworkId: string(p.NetworkID)}}
	case events.NetworkStateChanged:
		out.Payload = &v1.DaemonEvent_NetworkStateChanged{NetworkStateChanged: &v1.NetworkStateChanged{
			NetworkId: string(p.NetworkID), State: states[p.State], SafeMessage: p.SafeMessage,
		}}
	case events.AuthenticationRequired:
		out.Payload = &v1.DaemonEvent_AuthenticationRequired{AuthenticationRequired: &v1.AuthenticationRequired{
			NetworkId: string(p.NetworkID), Provider: providerToProto(p.Provider), AuthUrl: p.AuthURL, FlowId: p.FlowID,
		}}
	case events.AuthenticationCompleted:
		out.Payload = &v1.DaemonEvent_AuthenticationCompleted{AuthenticationCompleted: &v1.AuthenticationCompleted{NetworkId: string(p.NetworkID)}}
	case events.ApprovalRequired:
		out.Payload = &v1.DaemonEvent_ApprovalRequired{ApprovalRequired: &v1.ApprovalRequired{
			NetworkId: string(p.NetworkID), Provider: providerToProto(p.Provider), SafeMessage: p.SafeMessage,
		}}
	case events.PeerAdded:
		out.Payload = &v1.DaemonEvent_PeerAdded{PeerAdded: &v1.PeerAdded{Device: device(p.Device)}}
	case events.PeerUpdated:
		out.Payload = &v1.DaemonEvent_PeerUpdated{PeerUpdated: &v1.PeerUpdated{Device: device(p.Device)}}
	case events.PeerRemoved:
		out.Payload = &v1.DaemonEvent_PeerRemoved{PeerRemoved: &v1.PeerRemoved{
			Id: &v1.DeviceIdentity{NetworkId: string(p.ID.NetworkID), NodeId: string(p.ID.NodeID)},
		}}
	case events.WorkspaceChanged:
		out.Payload = &v1.DaemonEvent_WorkspaceChanged{WorkspaceChanged: &v1.WorkspaceChanged{Workspace: workspace(p.Workspace)}}
	case events.WorkspaceRemoved:
		out.Payload = &v1.DaemonEvent_WorkspaceRemoved{WorkspaceRemoved: &v1.WorkspaceRemoved{WorkspaceId: string(p.WorkspaceID)}}
	case events.ActiveWorkspaceChanged:
		out.Payload = &v1.DaemonEvent_ActiveWorkspaceChanged{ActiveWorkspaceChanged: &v1.ActiveWorkspaceChanged{WorkspaceId: string(p.WorkspaceID)}}
	case events.DestinationPreferenceChanged:
		out.Payload = &v1.DaemonEvent_DestinationPreferenceChanged{DestinationPreferenceChanged: &v1.DestinationPreferenceChanged{Preference: preference(p.Preference)}}
	case events.DestinationPreferenceRemoved:
		out.Payload = &v1.DaemonEvent_DestinationPreferenceRemoved{DestinationPreferenceRemoved: &v1.DestinationPreferenceRemoved{Destination: p.Destination}}
	}
	return out
}
