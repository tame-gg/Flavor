package inspect

import (
	"sort"
	"strings"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
)

type ConflictType int

const (
	ConflictAddress ConflictType = iota + 1
	ConflictDNSName
	ConflictHostname
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
}

type Conflict struct {
	ID              string
	Type            ConflictType
	Severity        Severity
	Scope           Scope
	Value           string
	Members         []Member
	ContextResolves bool
}

type ConflictReport struct {
	Conflicts    []Conflict
	NotInspected []Network
}

var conflictSlugs = map[ConflictType]string{
	ConflictAddress:  "address",
	ConflictDNSName:  "dns",
	ConflictHostname: "name",
}

func Conflicts(networks []Network) ConflictReport {
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
			rep.Conflicts = append(rep.Conflicts, classify(typ, value, ms))
		}
	}
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
