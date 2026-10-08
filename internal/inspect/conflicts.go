package inspect

import (
	"net/netip"
	"sort"
	"strings"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
)

type ConflictType int

const (
	ConflictAddress ConflictType = iota + 1
	ConflictDNSName
	ConflictHostname
	ConflictSubnet
)

type Severity int

const (
	SeverityAmbiguous Severity = iota + 1
	SeverityExpected
)

type Scope int

const (
	ScopeCrossNetwork Scope = iota + 1
	ScopeWithinNetwork
)

type Member struct {
	Network    domain.Network
	State      domain.NetworkConnectionState
	Device     domain.Device
	UniqueName string
	Route      netip.Prefix
}

type Conflict struct {
	ID               string
	Type             ConflictType
	Severity         Severity
	Scope            Scope
	Value            string
	Members          []Member
	ContextResolves  bool
	PreferredNetwork domain.NetworkID
	SampleAddress    netip.Addr
}

type ConflictReport struct {
	Conflicts    []Conflict
	NotInspected []Network
}

var conflictSlugs = map[ConflictType]string{
	ConflictAddress:  "address",
	ConflictDNSName:  "dns",
	ConflictHostname: "name",
	ConflictSubnet:   "subnet",
}

func Conflicts(networks []Network, prefs []domain.DestinationPreference) ConflictReport {
	var rep ConflictReport
	var members []Member
	seen := make(map[string]bool)
	for _, n := range networks {
		if !n.Live {
			rep.NotInspected = append(rep.NotInspected, Network{Network: n.Network, State: n.State})
			continue
		}
		for _, d := range n.Devices {
			if d.ID.NetworkID != n.Network.ID || seen[d.ID.Key()] {
				continue
			}
			seen[d.ID.Key()] = true
			members = append(members, Member{Network: n.Network, State: n.State, Device: d})
		}
	}

	dnsCount := make(map[string]int)
	for _, m := range members {
		if dns := dnsName(m.Device); dns != "" {
			dnsCount[dns]++
		}
	}
	for i := range members {
		if dns := dnsName(members[i].Device); dns != "" && dnsCount[dns] == 1 {
			members[i].UniqueName = dns
		}
	}

	groups := map[ConflictType]map[string][]Member{
		ConflictAddress:  {},
		ConflictDNSName:  {},
		ConflictHostname: {},
	}
	for _, m := range members {
		for _, a := range m.Device.Addresses {
			key := a.Unmap().String()
			groups[ConflictAddress][key] = append(groups[ConflictAddress][key], m)
		}
		if dns := dnsName(m.Device); dns != "" {
			groups[ConflictDNSName][dns] = append(groups[ConflictDNSName][dns], m)
		}
		for _, name := range bareNames(m.Device) {
			groups[ConflictHostname][name] = append(groups[ConflictHostname][name], m)
		}
	}

	for typ, byValue := range groups {
		for value, ms := range byValue {
			if len(ms) < 2 || allLocal(ms) {
				continue
			}
			c := classify(typ, value, ms)
			c.PreferredNetwork = preferredFor(c, prefs)
			rep.Conflicts = append(rep.Conflicts, c)
		}
	}
	rep.Conflicts = append(rep.Conflicts, subnetConflicts(members)...)
	sort.Slice(rep.Conflicts, func(i, j int) bool {
		a, b := rep.Conflicts[i], rep.Conflicts[j]
		if a.Severity != b.Severity {
			return a.Severity < b.Severity
		}
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return a.Value < b.Value
	})
	sort.SliceStable(rep.NotInspected, func(i, j int) bool {
		return rep.NotInspected[i].Network.DisplayName < rep.NotInspected[j].Network.DisplayName
	})
	return rep
}

func classify(typ ConflictType, value string, ms []Member) Conflict {
	sort.Slice(ms, func(i, j int) bool {
		a, b := ms[i], ms[j]
		if a.Network.DisplayName != b.Network.DisplayName {
			return a.Network.DisplayName < b.Network.DisplayName
		}
		if a.Network.ID != b.Network.ID {
			return a.Network.ID < b.Network.ID
		}
		return a.Device.ID.NodeID < b.Device.ID.NodeID
	})
	nets := make(map[domain.NetworkID]bool)
	resolves := true
	for _, m := range ms {
		nets[m.Network.ID] = true
		resolves = resolves && m.UniqueName != ""
	}
	c := Conflict{
		ID:              conflictSlugs[typ] + ":" + value,
		Type:            typ,
		Scope:           ScopeCrossNetwork,
		Value:           value,
		Members:         ms,
		ContextResolves: resolves && typ != ConflictDNSName,
		Severity:        SeverityExpected,
	}
	if len(nets) == 1 {
		c.Scope = ScopeWithinNetwork
	}
	if c.Scope == ScopeWithinNetwork || !c.ContextResolves {
		c.Severity = SeverityAmbiguous
	}
	return c
}

func allLocal(ms []Member) bool {
	for _, m := range ms {
		if !m.Device.Local {
			return false
		}
	}
	return true
}

func dnsName(d domain.Device) string {
	return strings.ToLower(strings.TrimSuffix(d.DNSName, "."))
}

func bareNames(d domain.Device) []string {
	var out []string
	if h := strings.ToLower(d.Hostname); h != "" && !strings.Contains(h, ".") {
		out = append(out, h)
	}
	if short, _, _ := strings.Cut(dnsName(d), "."); short != "" && (len(out) == 0 || out[0] != short) {
		out = append(out, short)
	}
	return out
}

func preferredFor(c Conflict, prefs []domain.DestinationPreference) domain.NetworkID {
	kind := domain.DestinationName
	if c.Type == ConflictAddress {
		kind = domain.DestinationAddress
	}
	for _, p := range prefs {
		if p.Destination != c.Value || p.Kind != kind {
			continue
		}
		for _, m := range c.Members {
			if m.Network.ID == p.NetworkID {
				return p.NetworkID
			}
		}
	}
	return ""
}

func subnetConflicts(members []Member) []Conflict {
	type entry struct {
		m Member
		r netip.Prefix
	}
	var entries []entry
	for _, m := range members {
		for _, r := range m.Device.Routes {
			m.Route = r
			entries = append(entries, entry{m, r})
		}
	}
	var out []Conflict
	byPrefix := make(map[netip.Prefix][]Member)
	for _, e := range entries {
		byPrefix[e.r] = append(byPrefix[e.r], e.m)
	}
	for r, ms := range byPrefix {
		if distinctNetworks(ms) > 1 {
			c := classify(ConflictSubnet, r.String(), ms)
			c.ID = "subnet:" + r.String()
			c.Severity, c.ContextResolves, c.SampleAddress = SeverityAmbiguous, false, sample(r)
			out = append(out, c)
		}
	}
	for narrow, inner := range byPrefix {
		var ms []Member
		for _, e := range entries {
			if e.r.Bits() < narrow.Bits() && e.r.Contains(narrow.Addr()) && !sameNetwork(e.m, inner) {
				ms = append(ms, e.m)
			}
		}
		if len(ms) == 0 {
			continue
		}
		c := classify(ConflictSubnet, narrow.String(), append(ms, inner...))
		c.ID = "overlap:" + narrow.String()
		c.Severity, c.ContextResolves, c.SampleAddress = SeverityExpected, true, sample(narrow)
		out = append(out, c)
	}
	return out
}

func distinctNetworks(ms []Member) int {
	seen := make(map[domain.NetworkID]bool)
	for _, m := range ms {
		seen[m.Network.ID] = true
	}
	return len(seen)
}

func sameNetwork(m Member, others []Member) bool {
	for _, o := range others {
		if o.Network.ID == m.Network.ID {
			return true
		}
	}
	return false
}

func sample(p netip.Prefix) netip.Addr {
	if a := p.Addr().Next(); a.IsValid() && p.Contains(a) {
		return a
	}
	return p.Addr()
}
