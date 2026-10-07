package session

import (
	"net/netip"
	"strings"
	"time"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"tailscale.com/ipn/ipnstate"
)

func projectStatus(st *ipnstate.Status) statusSnap {
	if st == nil {
		return statusSnap{}
	}
	out := statusSnap{
		BackendState: st.BackendState,
		AuthURL:      st.AuthURL,
		TailscaleIPs: append([]netip.Addr(nil), st.TailscaleIPs...),
		Health:       append([]string(nil), st.Health...),
	}
	if st.Self != nil {
		self := projectPeer(st.Self)
		out.Self = &self
	}
	for _, p := range st.Peer {
		if p == nil {
			continue
		}
		out.Peers = append(out.Peers, projectPeer(p))
	}
	return out
}

func projectPeer(p *ipnstate.PeerStatus) peerSnap {
	id := domain.NodeID(strings.TrimSpace(string(p.ID)))
	return peerSnap{
		NodeID:    id,
		Hostname:  p.HostName,
		DNSName:   strings.TrimSuffix(p.DNSName, "."),
		Addresses: append([]netip.Addr(nil), p.TailscaleIPs...),
		Online:    p.Online,
		LastSeen:  p.LastSeen.UTC(),
	}
}

func connectedInvariant(snap statusSnap) bool {
	if snap.BackendState != "Running" {
		return false
	}
	if len(snap.TailscaleIPs) == 0 {
		return false
	}
	if snap.Self == nil || snap.Self.NodeID == "" {
		return false
	}
	return true
}

func mapConnectionState(snap statusSnap, hadAuthPrompt bool) domain.NetworkConnectionState {
	switch snap.BackendState {
	case "NeedsLogin":
		if hadAuthPrompt || snap.AuthURL != "" {
			return domain.StateAuthenticating
		}
		return domain.StateConnecting
	case "NeedsMachineAuth":
		return domain.StateAwaitingApproval
	case "Starting":
		return domain.StateConnecting
	case "Running":
		if connectedInvariant(snap) {
			if len(snap.Health) > 0 {
				return domain.StateDegraded
			}
			return domain.StateConnected
		}
		return domain.StateConnecting
	case "Stopped":
		return domain.StateDisconnected
	case "NoState":
		return domain.StateConnecting
	default:
		return domain.StateConnecting
	}
}

func deviceEqual(a, b domain.Device) bool {
	if a.ID != b.ID || a.Hostname != b.Hostname || a.DNSName != b.DNSName || a.Online != b.Online {
		return false
	}
	if !a.LastSeen.Equal(b.LastSeen) {
		return false
	}
	if len(a.Addresses) != len(b.Addresses) {
		return false
	}
	for i := range a.Addresses {
		if a.Addresses[i] != b.Addresses[i] {
			return false
		}
	}
	return true
}

func cloneDevice(d domain.Device) domain.Device {
	out := d
	if d.Addresses != nil {
		out.Addresses = append([]netip.Addr(nil), d.Addresses...)
	}
	if !d.LastSeen.IsZero() {
		out.LastSeen = d.LastSeen.UTC()
	}
	return out
}

func cloneLocalNode(n domain.LocalNode) domain.LocalNode {
	out := n
	if n.Addresses != nil {
		out.Addresses = append([]netip.Addr(nil), n.Addresses...)
	}
	return out
}

func cloneAuthPrompt(p *domain.AuthPrompt) *domain.AuthPrompt {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}

func nowUTC() time.Time { return time.Now().UTC() }
