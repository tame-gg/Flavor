package session

import (
	"net/netip"
	"slices"
	"strings"
	"time"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
)

func projectStatus(st *ipnstate.Status) EngineStatus {
	if st == nil {
		return EngineStatus{}
	}
	out := EngineStatus{
		BackendState: st.BackendState,
		AuthURL:      st.AuthURL,
		TailscaleIPs: append([]netip.Addr(nil), st.TailscaleIPs...),
		Health:       append([]string(nil), st.Health...),
	}
	if st.Self != nil {
		self := projectPeer(st.Self)
		out.Self = &self
		out.ApprovedRoutes = approvedRoutes(st.Self)
	}
	for _, p := range st.Peer {
		if p == nil {
			continue
		}
		out.Peers = append(out.Peers, projectPeer(p))
	}
	if st.ExitNodeStatus != nil {
		out.ExitNode = domain.NodeID(strings.TrimSpace(string(st.ExitNodeStatus.ID)))
	}
	return out
}

func projectPeer(p *ipnstate.PeerStatus) EnginePeer {
	id := domain.NodeID(strings.TrimSpace(string(p.ID)))
	return EnginePeer{
		NodeID:         id,
		Hostname:       p.HostName,
		DNSName:        strings.TrimSuffix(p.DNSName, "."),
		Addresses:      append([]netip.Addr(nil), p.TailscaleIPs...),
		Online:         p.Online,
		LastSeen:       p.LastSeen.UTC(),
		OS:             p.OS,
		Tags:           tags(p),
		Routes:         routes(p),
		ExitNodeOption: p.ExitNodeOption,
	}
}

func projectDNSRecords(cfg *tailcfg.DNSConfig) []domain.DNSRecord {
	if cfg == nil {
		return nil
	}
	var records []domain.DNSRecord
	for _, r := range cfg.ExtraRecords {
		if r.Type != "" && r.Type != "A" && r.Type != "AAAA" {
			continue
		}
		addr, err := netip.ParseAddr(strings.TrimSpace(r.Value))
		if err != nil {
			continue
		}
		records = append(records, domain.DNSRecord{Name: r.Name, Addresses: []netip.Addr{addr}})
	}
	return domain.MergeDNSRecords(records)
}

func routes(p *ipnstate.PeerStatus) []netip.Prefix {
	if p.PrimaryRoutes == nil {
		return nil
	}
	var out []netip.Prefix
	for _, r := range p.PrimaryRoutes.All() {
		if r.IsValid() && r.Bits() > 0 {
			out = append(out, r.Masked())
		}
	}
	return out
}

func approvedRoutes(self *ipnstate.PeerStatus) []netip.Prefix {
	if self.AllowedIPs == nil {
		return nil
	}
	var out []netip.Prefix
	for _, r := range self.AllowedIPs.All() {
		if !r.IsValid() || r.Bits() == 0 || (r.IsSingleIP() && slices.Contains(self.TailscaleIPs, r.Addr())) {
			continue
		}
		out = append(out, r.Masked())
	}
	return out
}

func projectAdvertised(snap EngineStatus) ([]domain.AdvertisedRoute, domain.AdvertisedExitNode) {
	var routes []domain.AdvertisedRoute
	for _, r := range snap.AdvertisedRoutes {
		if !r.IsValid() || r.Bits() == 0 {
			continue
		}
		r = r.Masked()
		if slices.ContainsFunc(routes, func(a domain.AdvertisedRoute) bool { return a.Prefix == r }) {
			continue
		}
		routes = append(routes, domain.AdvertisedRoute{Prefix: r, Approved: slices.Contains(snap.ApprovedRoutes, r)})
	}
	slices.SortFunc(routes, func(a, b domain.AdvertisedRoute) int { return a.Prefix.Compare(b.Prefix) })
	exit := domain.AdvertisedExitNode{Offered: containsAll(snap.AdvertisedRoutes, domain.ExitRoutes)}
	exit.Approved = exit.Offered && snap.Self != nil && snap.Self.ExitNodeOption
	return routes, exit
}

func containsAll(have, want []netip.Prefix) bool {
	for _, w := range want {
		if !slices.Contains(have, w) {
			return false
		}
	}
	return true
}

func tags(p *ipnstate.PeerStatus) []string {
	if p.Tags == nil || p.Tags.Len() == 0 {
		return nil
	}
	return p.Tags.AsSlice()
}

func connectedInvariant(snap EngineStatus) bool {
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

func mapConnectionState(snap EngineStatus, hadAuthPrompt bool) domain.NetworkConnectionState {
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
	if a.ID != b.ID || a.Hostname != b.Hostname || a.DNSName != b.DNSName || a.Online != b.Online || a.Local != b.Local || a.OS != b.OS || !slices.Equal(a.Tags, b.Tags) || !slices.Equal(a.Routes, b.Routes) || a.ExitNodeOption != b.ExitNodeOption || a.ExitNode != b.ExitNode {
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
	out.Tags = slices.Clone(d.Tags)
	out.Routes = slices.Clone(d.Routes)
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
	out.Routes = slices.Clone(n.Routes)
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
