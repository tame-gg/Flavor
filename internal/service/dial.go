package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/inspect"
	"git.lunarlabs.dev/flavor/flavor/internal/store"
)

const (
	CodeDestinationAmbiguous   Code = "DESTINATION_AMBIGUOUS"
	CodeDestinationNotFound    Code = "DESTINATION_NOT_FOUND"
	CodeDestinationUnreachable Code = "DESTINATION_UNREACHABLE"
)

type Route struct {
	Result  inspect.Result
	Network domain.Network
	Target  netip.AddrPort
}

func (s *Service) Route(ctx context.Context, destination, rawNetwork string, defaultPort uint16) (Route, error) {
	q, err := parseDestination(destination)
	if err != nil {
		return Route{}, err
	}
	if q.Port == 0 {
		q.Port = defaultPort
	}
	if q.Port == 0 {
		return Route{}, fail(CodeInvalidArgument, "include a port, for example postgres.home.flavor.internal:5432", false)
	}
	if rawNetwork != "" {
		id, err := s.resolveNetworkRef(ctx, rawNetwork)
		if err != nil {
			return Route{}, err
		}
		q.Context = id
	}
	nets, _, err := s.liveNetworks(ctx)
	if err != nil {
		return Route{}, err
	}
	var pref *domain.DestinationPreference
	if q.Context == "" {
		if p, err := s.cfg.Store.Preferences().Get(ctx, q.Normalized()); err == nil {
			pref = &p
		} else if !errors.Is(err, store.ErrNotFound) {
			return Route{}, s.storeErr(err)
		}
	}
	res := inspect.Resolve(q, nets, pref)
	r := Route{Result: res}
	switch res.Decision {
	case inspect.DecisionAmbiguous:
		n := map[domain.NetworkID]bool{}
		for _, c := range res.Candidates {
			if c.Status == inspect.StatusTied {
				n[c.Network.ID] = true
			}
		}
		return r, fail(CodeDestinationAmbiguous, fmt.Sprintf("%s matches %d candidates on %d network(s); use a Flavor name, a preference or an explicit network", q.Normalized(), countTied(res), len(n)), false)
	case inspect.DecisionNoMatch:
		return r, fail(CodeDestinationNotFound, "no connected network has a device or route for "+q.Normalized(), false)
	}
	for _, c := range res.Candidates {
		if c.Status != inspect.StatusSelected {
			continue
		}
		r.Network = c.Network
		addr, ok := targetAddress(q, c)
		if !ok {
			return r, fail(CodeDestinationUnreachable, "the selected device has no address", false)
		}
		r.Target = netip.AddrPortFrom(addr, q.Port)
	}
	return r, nil
}

func (s *Service) Dial(ctx context.Context, r Route) (net.Conn, error) {
	sess, ok := s.cfg.Sessions.Get(r.Network.ID)
	if !ok {
		return nil, fail(CodeDestinationUnreachable, "network "+r.Network.DisplayName+" is not connected", true)
	}
	conn, err := sess.Dial(ctx, "tcp", r.Target.String())
	if err != nil {
		return nil, fail(CodeDestinationUnreachable, "could not connect to "+r.Target.String()+" on "+r.Network.DisplayName, true)
	}
	return conn, nil
}

func (s *Service) resolveNetworkRef(ctx context.Context, raw string) (domain.NetworkID, error) {
	all, err := s.cfg.Store.Networks().List(ctx)
	if err != nil {
		return "", s.storeErr(err)
	}
	var match []domain.NetworkID
	for _, n := range all {
		if string(n.ID) == raw {
			return n.ID, nil
		}
		if n.DisplayName == raw || domain.NetworkLabel(n.DisplayName) == raw {
			match = append(match, n.ID)
		}
	}
	switch len(match) {
	case 1:
		return match[0], nil
	case 0:
		return "", fail(CodeNetworkNotFound, "no network named "+raw, false)
	}
	return "", fail(CodeInvalidArgument, "several networks are named "+raw+"; use the network id", false)
}

func targetAddress(q inspect.Query, c inspect.Candidate) (netip.Addr, bool) {
	if c.Match == inspect.MatchDeviceAddress || c.Match == inspect.MatchSubnetRoute {
		return q.Address, true
	}
	for _, a := range c.Device.Addresses {
		if a.Is4() {
			return a, true
		}
	}
	if len(c.Device.Addresses) > 0 {
		return c.Device.Addresses[0], true
	}
	return netip.Addr{}, false
}

func countTied(res inspect.Result) int {
	n := 0
	for _, c := range res.Candidates {
		if c.Status == inspect.StatusTied {
			n++
		}
	}
	return n
}
