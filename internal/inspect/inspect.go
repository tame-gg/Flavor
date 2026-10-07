package inspect

import (
	"errors"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
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
}

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
)

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
	Live    bool
}

type Candidate struct {
	Network      domain.Network
	State        domain.NetworkConnectionState
	Device       domain.Device
	Match        MatchKind
	MatchedValue string
	Status       CandidateStatus
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
	seen := make(map[string]bool)
	for _, n := range networks {
		if !n.Live {
			res.NotInspected = append(res.NotInspected, Network{Network: n.Network, State: n.State})
			continue
		}
		for _, d := range n.Devices {
			if d.ID.NetworkID != n.Network.ID || seen[d.ID.Key()] {
				continue
			}
			kind, value, ok := match(q, d)
			if !ok {
				continue
			}
			seen[d.ID.Key()] = true
			res.Candidates = append(res.Candidates, Candidate{Network: n.Network, State: n.State, Device: d, Match: kind, MatchedValue: value})
		}
	}
	sort.SliceStable(res.Candidates, func(i, j int) bool {
		a, b := res.Candidates[i], res.Candidates[j]
		if a.Match != b.Match {
			return a.Match < b.Match
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
		res.Decision, res.Reason = DecisionNoMatch, ReasonNoMatch
		return res
	}
	best := res.Candidates[0].Match
	tied := 0
	for i := range res.Candidates {
		switch {
		case res.Candidates[i].Match == best:
			tied++
		default:
			res.Candidates[i].Status = StatusOutranked
		}
	}
	res.DecidedBy = best
	if tied == 1 {
		res.Decision, res.Reason = DecisionUnique, reasonFor(best)
		res.Candidates[0].Status = StatusSelected
		return res
	}
	res.Decision, res.Reason = DecisionAmbiguous, ReasonMultipleMatches
	for i := 0; i < tied; i++ {
		res.Candidates[i].Status = StatusTied
	}
	return res
}

func reasonFor(m MatchKind) Reason {
	switch m {
	case MatchDeviceAddress:
		return ReasonExactDeviceAddress
	case MatchDeviceDNSName:
		return ReasonDeviceDNSName
	default:
		return ReasonDeviceHostname
	}
}

func match(q Query, d domain.Device) (MatchKind, string, bool) {
	if q.Kind == KindAddress {
		for _, a := range d.Addresses {
			if a.Unmap() == q.Address {
				return MatchDeviceAddress, a.String(), true
			}
		}
		return 0, "", false
	}
	dns := strings.ToLower(strings.TrimSuffix(d.DNSName, "."))
	if dns != "" && dns == q.Name {
		return MatchDeviceDNSName, dns, true
	}
	if strings.Contains(q.Name, ".") {
		return 0, "", false
	}
	if h := strings.ToLower(d.Hostname); h != "" && h == q.Name {
		return MatchDeviceHostname, d.Hostname, true
	}
	if short, _, _ := strings.Cut(dns, "."); short != "" && short == q.Name {
		return MatchDeviceHostname, short, true
	}
	return 0, "", false
}
