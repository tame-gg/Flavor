package inspect

import (
	"errors"
	"net"
	"net/netip"
	"slices"
	"sort"
	"strconv"
	"strings"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/naming"
)

var ErrInvalidDestination = errors.New("invalid destination")

type QueryKind int

const (
	KindAddress QueryKind = iota + 1
	KindName
)

type Query struct {
	Raw     string
	Kind    QueryKind
	Address netip.Addr
	Name    string
	Port    uint16
	Device  string
	Network string
	Context domain.NetworkID
}

func (q Query) Qualified() bool { return q.Network != "" }

func (q Query) DestinationKind() domain.DestinationKind {
	if q.Kind == KindAddress {
		return domain.DestinationAddress
	}
	return domain.DestinationName
}

func (q Query) Normalized() string {
	if q.Kind == KindAddress {
		return q.Address.String()
	}
	return q.Name
}

type MatchKind int

const (
	MatchDeviceAddress MatchKind = iota + 1
	MatchDeviceDNSName
	MatchDeviceHostname
	MatchSubnetRoute
	MatchQualifiedName
	MatchDNSRecord
	MatchExitNode
)

func (m MatchKind) rank() MatchKind {
	if m == MatchDNSRecord {
		return MatchDeviceDNSName
	}
	return m
}

type Decision int

const (
	DecisionUnique Decision = iota + 1
	DecisionAmbiguous
	DecisionNoMatch
)

type Reason int

const (
	ReasonExactDeviceAddress Reason = iota + 1
	ReasonDeviceDNSName
	ReasonDeviceHostname
	ReasonMultipleMatches
	ReasonNoMatch
	ReasonDestinationPreference
	ReasonSubnetRoute
	ReasonLongestPrefix
	ReasonNetworkQualifiedName
	ReasonExplicitNetwork
	ReasonAmbiguousNetworkLabel
	ReasonDNSRecord
	ReasonExitNode
)

type PreferenceState int

const (
	PreferenceApplied PreferenceState = iota + 1
	PreferenceNetworkNotConnected
	PreferenceNoMatchOnNetwork
)

type PreferenceUse struct {
	Preference domain.DestinationPreference
	Network    domain.Network
	State      PreferenceState
}

type CandidateStatus int

const (
	StatusSelected CandidateStatus = iota + 1
	StatusTied
	StatusOutranked
)

type Network struct {
	Network domain.Network
	State   domain.NetworkConnectionState
	Devices []domain.Device
	Records []domain.DNSRecord
	Live    bool

	ExitNode domain.Device
}

type Candidate struct {
	Network      domain.Network
	State        domain.NetworkConnectionState
	Device       domain.Device
	Match        MatchKind
	MatchedValue string
	Prefix       netip.Prefix
	Addresses    []netip.Addr
	Status       CandidateStatus
	Name         string
	StableName   string
}

type Result struct {
	Query        Query
	Decision     Decision
	Reason       Reason
	DecidedBy    MatchKind
	Candidates   []Candidate
	NotInspected []Network
	Preference   *PreferenceUse
}

func ParseQuery(raw string) (Query, error) {
	s := strings.TrimSpace(raw)
	q := Query{Raw: s}
	if s == "" || len(s) > 260 || strings.ContainsAny(s, " \t\r\n/\\@?#") {
		return q, ErrInvalidDestination
	}
	host := s
	if h, p, err := net.SplitHostPort(s); err == nil {
		port, perr := strconv.ParseUint(p, 10, 16)
		if perr != nil || port == 0 {
			return q, ErrInvalidDestination
		}
		host, q.Port = h, uint16(port)
	}
	host = strings.TrimSuffix(strings.Trim(host, "[]"), ".")
	if addr, err := netip.ParseAddr(host); err == nil {
		if addr.Zone() != "" {
			return q, ErrInvalidDestination
		}
		q.Kind, q.Address = KindAddress, addr.Unmap()
		return q, nil
	}
	name := strings.ToLower(host)
	if !validName(name) {
		return q, ErrInvalidDestination
	}
	q.Kind, q.Name = KindName, name
	if strings.HasSuffix(name, "."+naming.Suffix) {
		device, network, ok := naming.Split(name)
		if !ok {
			return q, ErrInvalidDestination
		}
		q.Device, q.Network = device, network
	}
	return q, nil
}

func validName(name string) bool {
	if name == "" || len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
				return false
			}
		}
	}
	return true
}

func Resolve(q Query, networks []Network, pref *domain.DestinationPreference) Result {
	res := resolveMatches(q, networks)
	if q.Context != "" {
		if res.Decision == DecisionUnique {
			res.Reason = ReasonExplicitNetwork
		}
		return res
	}
	if pref != nil && pref.Destination == q.Normalized() && pref.Kind == q.DestinationKind() {
		applyPreference(&res, networks, *pref)
	}
	return res
}

func applyPreference(res *Result, networks []Network, pref domain.DestinationPreference) {
	use := &PreferenceUse{Preference: pref, State: PreferenceNoMatchOnNetwork}
	res.Preference = use
	for _, n := range networks {
		if n.Network.ID == pref.NetworkID {
			use.Network = n.Network
			if !n.Live {
				use.State = PreferenceNetworkNotConnected
				return
			}
		}
	}
	var preferred []int
	for i, c := range res.Candidates {
		if c.Network.ID == pref.NetworkID {
			preferred = append(preferred, i)
		}
	}
	if len(preferred) == 0 {
		return
	}
	use.State = PreferenceApplied
	for i := range res.Candidates {
		res.Candidates[i].Status = StatusOutranked
	}
	if len(preferred) == 1 {
		c := &res.Candidates[preferred[0]]
		c.Status = StatusSelected
		res.Decision, res.Reason, res.DecidedBy = DecisionUnique, ReasonDestinationPreference, c.Match
		return
	}
	for _, i := range preferred {
		res.Candidates[i].Status = StatusTied
	}
	res.Decision, res.Reason = DecisionAmbiguous, ReasonMultipleMatches
}

func resolveMatches(q Query, networks []Network) Result {
	res := Result{Query: q}
	all := make([]domain.Network, 0, len(networks))
	for _, n := range networks {
		all = append(all, n.Network)
	}
	netLabels := naming.NetworkLabels(all)
	stableNet := ""
	sharedLabel := 0
	if q.Qualified() {
		for _, l := range netLabels {
			if l.Stable == q.Network {
				stableNet = q.Network
			}
		}
		for _, n := range all {
			if naming.FriendlyNetworkCandidate(n) == q.Network {
				sharedLabel++
			}
		}
	}
	seen := make(map[string]bool)
	for _, n := range networks {
		if q.Context != "" && n.Network.ID != q.Context {
			continue
		}
		if !n.Live {
			res.NotInspected = append(res.NotInspected, Network{Network: n.Network, State: n.State})
			continue
		}
		devices := ownDevices(n, seen)
		devLabels := naming.DeviceLabels(devices)
		if q.Qualified() && !networkMatches(q, n.Network, netLabels[n.Network.ID], stableNet) {
			continue
		}
		stableDev := ""
		for _, l := range devLabels {
			if q.Qualified() && l.Stable == q.Device {
				stableDev = q.Device
			}
		}
		for _, d := range devices {
			var c Candidate
			var ok bool
			if q.Qualified() {
				c, ok = qualifiedMatch(q, d, devLabels[d.ID.NodeID], stableDev)
			} else {
				c, ok = match(q, d)
			}
			if !ok {
				continue
			}
			dl, nl := devLabels[d.ID.NodeID], netLabels[n.Network.ID]
			c.Network, c.State, c.Device = n.Network, n.State, d
			c.Name, c.StableName = naming.Name(dl.Published(), nl.Published()), naming.Name(dl.Stable, nl.Stable)
			res.Candidates = append(res.Candidates, c)
		}
		if !q.Qualified() {
			res.Candidates = append(res.Candidates, recordMatches(q, n, devices, devLabels, netLabels[n.Network.ID])...)
		}
	}
	sort.SliceStable(res.Candidates, func(i, j int) bool {
		a, b := res.Candidates[i], res.Candidates[j]
		if a.Match.rank() != b.Match.rank() {
			return a.Match.rank() < b.Match.rank()
		}
		if a.Prefix.Bits() != b.Prefix.Bits() {
			return a.Prefix.Bits() > b.Prefix.Bits()
		}
		if a.Network.DisplayName != b.Network.DisplayName {
			return a.Network.DisplayName < b.Network.DisplayName
		}
		if a.Network.ID != b.Network.ID {
			return a.Network.ID < b.Network.ID
		}
		return a.Device.ID.NodeID < b.Device.ID.NodeID
	})
	sort.SliceStable(res.NotInspected, func(i, j int) bool {
		return res.NotInspected[i].Network.DisplayName < res.NotInspected[j].Network.DisplayName
	})

	if len(res.Candidates) == 0 {
		res.Candidates = exitNodeCandidates(q, networks, netLabels)
		switch len(res.Candidates) {
		case 0:
			res.Decision, res.Reason = DecisionNoMatch, ReasonNoMatch
		case 1:
			res.Candidates[0].Status = StatusSelected
			res.Decision, res.Reason, res.DecidedBy = DecisionUnique, ReasonExitNode, MatchExitNode
		default:
			for i := range res.Candidates {
				res.Candidates[i].Status = StatusTied
			}
			res.Decision, res.Reason, res.DecidedBy = DecisionAmbiguous, ReasonMultipleMatches, MatchExitNode
		}
		return res
	}
	if stableNet == "" && sharedLabel > 1 {
		for i := range res.Candidates {
			res.Candidates[i].Status = StatusTied
		}
		res.Decision, res.Reason, res.DecidedBy = DecisionAmbiguous, ReasonAmbiguousNetworkLabel, MatchQualifiedName
		return res
	}
	best := res.Candidates[0]
	tied := 0
	for i := range res.Candidates {
		c := res.Candidates[i]
		if c.Match.rank() == best.Match.rank() && c.Prefix.Bits() == best.Prefix.Bits() {
			tied++
		} else {
			res.Candidates[i].Status = StatusOutranked
		}
	}
	res.DecidedBy = best.Match
	if tied == 1 {
		res.Decision, res.Reason = DecisionUnique, reasonFor(best.Match)
		if best.Match == MatchSubnetRoute && len(res.Candidates) > 1 {
			res.Reason = ReasonLongestPrefix
		}
		res.Candidates[0].Status = StatusSelected
		return res
	}
	res.Decision, res.Reason = DecisionAmbiguous, ReasonMultipleMatches
	for i := 0; i < tied; i++ {
		res.Candidates[i].Status = StatusTied
	}
	return res
}

func exitNodeCandidates(q Query, networks []Network, netLabels map[domain.NetworkID]naming.Labels) []Candidate {
	if q.Qualified() || tailnetDestination(q) {
		return nil
	}
	var out []Candidate
	for _, n := range networks {
		if !n.Live || n.ExitNode.ID.NodeID == "" || (q.Context != "" && n.Network.ID != q.Context) {
			continue
		}
		devLabels := naming.DeviceLabels(ownDevices(n, make(map[string]bool)))
		dl, nl := devLabels[n.ExitNode.ID.NodeID], netLabels[n.Network.ID]
		out = append(out, Candidate{
			Network:      n.Network,
			State:        n.State,
			Device:       n.ExitNode,
			Match:        MatchExitNode,
			MatchedValue: q.Normalized(),
			Name:         naming.Name(dl.Published(), nl.Published()),
			StableName:   naming.Name(dl.Stable, nl.Stable),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Network.DisplayName != b.Network.DisplayName {
			return a.Network.DisplayName < b.Network.DisplayName
		}
		return a.Network.ID < b.Network.ID
	})
	return out
}

func tailnetDestination(q Query) bool {
	if q.Kind == KindAddress {
		return slices.ContainsFunc(domain.TailnetPrefixes, func(p netip.Prefix) bool { return p.Contains(q.Address) })
	}
	return q.Name == naming.Suffix || strings.HasSuffix(q.Name, "."+naming.Suffix)
}

func reasonFor(m MatchKind) Reason {
	switch m {
	case MatchDeviceAddress:
		return ReasonExactDeviceAddress
	case MatchDeviceDNSName:
		return ReasonDeviceDNSName
	case MatchSubnetRoute:
		return ReasonSubnetRoute
	case MatchQualifiedName:
		return ReasonNetworkQualifiedName
	case MatchDNSRecord:
		return ReasonDNSRecord
	default:
		return ReasonDeviceHostname
	}
}

func recordMatches(q Query, n Network, devices []domain.Device, devLabels map[domain.NodeID]naming.Labels, nl naming.Labels) []Candidate {
	if q.Kind != KindName {
		return nil
	}
	for _, d := range devices {
		if dnsName(d) == q.Name {
			return nil
		}
	}
	var out []Candidate
	for _, r := range n.Records {
		if r.Name != q.Name {
			continue
		}
		target, ok := r.Target()
		if !ok {
			continue
		}
		c := Candidate{
			Match:        MatchDNSRecord,
			MatchedValue: target.String(),
			Addresses:    slices.Clone(r.Addresses),
			Network:      n.Network,
			State:        n.State,
		}
		if owner, ok := ownerOf(devices, target); ok {
			dl := devLabels[owner.ID.NodeID]
			c.Device = owner
			c.Name, c.StableName = naming.Name(dl.Published(), nl.Published()), naming.Name(dl.Stable, nl.Stable)
		}
		out = append(out, c)
	}
	return out
}

func ownerOf(devices []domain.Device, addr netip.Addr) (domain.Device, bool) {
	for _, d := range devices {
		for _, a := range d.Addresses {
			if a.Unmap() == addr {
				return d, true
			}
		}
	}
	return domain.Device{}, false
}

func ownDevices(n Network, seen map[string]bool) []domain.Device {
	var out []domain.Device
	for _, d := range n.Devices {
		if d.ID.NetworkID != n.Network.ID || seen[d.ID.Key()] {
			continue
		}
		seen[d.ID.Key()] = true
		out = append(out, d)
	}
	return out
}

func networkMatches(q Query, n domain.Network, l naming.Labels, stable string) bool {
	if stable != "" {
		return l.Stable == stable
	}
	return naming.FriendlyNetworkCandidate(n) == q.Network
}

func qualifiedMatch(q Query, d domain.Device, l naming.Labels, stable string) (Candidate, bool) {
	if stable != "" {
		if l.Stable != stable {
			return Candidate{}, false
		}
	} else if naming.FriendlyDeviceCandidate(d) != q.Device {
		return Candidate{}, false
	}
	return Candidate{Match: MatchQualifiedName, MatchedValue: q.Name}, true
}

func match(q Query, d domain.Device) (Candidate, bool) {
	if q.Kind == KindAddress {
		for _, a := range d.Addresses {
			if a.Unmap() == q.Address {
				return Candidate{Match: MatchDeviceAddress, MatchedValue: a.String()}, true
			}
		}
		var best netip.Prefix
		for _, r := range d.Routes {
			if r.Contains(q.Address) && r.Bits() > best.Bits() {
				best = r
			}
		}
		if best.IsValid() {
			return Candidate{Match: MatchSubnetRoute, MatchedValue: best.String(), Prefix: best}, true
		}
		return Candidate{}, false
	}
	dns := strings.ToLower(strings.TrimSuffix(d.DNSName, "."))
	if dns != "" && dns == q.Name {
		return Candidate{Match: MatchDeviceDNSName, MatchedValue: dns}, true
	}
	if strings.Contains(q.Name, ".") {
		return Candidate{}, false
	}
	if h := strings.ToLower(d.Hostname); h != "" && h == q.Name {
		return Candidate{Match: MatchDeviceHostname, MatchedValue: d.Hostname}, true
	}
	if short, _, _ := strings.Cut(dns, "."); short != "" && short == q.Name {
		return Candidate{Match: MatchDeviceHostname, MatchedValue: short}, true
	}
	return Candidate{}, false
}
