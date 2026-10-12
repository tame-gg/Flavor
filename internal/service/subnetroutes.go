package service

import (
	"cmp"
	"context"
	"net/netip"
	"slices"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/inspect"
)

type SubnetRouteState int

const (
	RouteNotUsed SubnetRouteState = iota + 1
	RouteInstalled
	RoutePending
	RouteRefused
	RouteNotAdvertised
)

type SubnetRoute struct {
	Network  domain.Network
	State    domain.NetworkConnectionState
	Prefix   netip.Prefix
	Routers  []domain.Device
	Use      SubnetRouteState
	Refused  string
	Overlaps []domain.Network
}

type SystemRoute struct {
	Prefix  netip.Prefix
	Network domain.NetworkID
}

type installedRoutes struct {
	active    bool
	installed []netip.Prefix
	refused   map[netip.Prefix]string
}

func (s *Service) SubnetRoutes(ctx context.Context, rawNetwork string) ([]SubnetRoute, error) {
	var id domain.NetworkID
	if rawNetwork != "" {
		var err error
		if id, err = s.resolveNetworkRef(ctx, rawNetwork); err != nil {
			return nil, err
		}
	}
	nets, _, err := s.liveNetworks(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	inst := s.routes
	s.mu.Unlock()
	var out []SubnetRoute
	for _, n := range nets {
		if !n.Live || (id != "" && n.Network.ID != id) {
			continue
		}
		routers := advertisedRoutes(n)
		prefixes := make([]netip.Prefix, 0, len(routers))
		for p := range routers {
			prefixes = append(prefixes, p)
		}
		for _, p := range n.Network.UsedRoutes {
			if _, ok := routers[p]; !ok {
				prefixes = append(prefixes, p)
			}
		}
		slices.SortFunc(prefixes, netip.Prefix.Compare)
		for _, p := range prefixes {
			r := SubnetRoute{Network: n.Network, State: n.State, Prefix: p, Routers: routers[p], Use: RouteNotUsed, Overlaps: overlapping(nets, n.Network.ID, p)}
			if slices.Contains(n.Network.UsedRoutes, p) {
				switch {
				case len(r.Routers) == 0:
					r.Use = RouteNotAdvertised
				case slices.Contains(inst.installed, p):
					r.Use = RouteInstalled
				case inst.refused[p] != "":
					r.Use, r.Refused = RouteRefused, inst.refused[p]
				case !inst.active:
					r.Use, r.Refused = RoutePending, "system-wide networking through flavor-netd is not active"
				default:
					r.Use = RoutePending
				}
			}
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Service) UseSubnetRoutes(ctx context.Context, rawNetwork string, add, remove []string, all, none bool) ([]SubnetRoute, error) {
	id, err := s.resolveNetworkRef(ctx, rawNetwork)
	if err != nil {
		return nil, err
	}
	toAdd, err := parseRoutes(add, true)
	if err != nil {
		return nil, err
	}
	toRemove, err := parseRoutes(remove, false)
	if err != nil {
		return nil, err
	}
	release, err := s.acquire(id, opRoutes)
	if err != nil {
		return nil, err
	}
	defer release()
	s.routesMu.Lock()
	defer s.routesMu.Unlock()
	nets, _, err := s.liveNetworks(ctx)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(nets, func(n inspect.Network) bool { return n.Network.ID == id })
	if i < 0 {
		return nil, fail(CodeNetworkNotFound, "network not found", false)
	}
	if !nets[i].Live && (len(toAdd) > 0 || all) {
		return nil, fail(CodeDestinationUnreachable, "connect the network to choose its routes", false)
	}
	n := nets[i]
	routers := advertisedRoutes(n)
	if all {
		for p := range routers {
			toAdd = append(toAdd, p)
		}
	}
	if none {
		toRemove = slices.Clone(n.Network.UsedRoutes)
	}
	for _, p := range toAdd {
		if _, ok := routers[p]; !ok {
			return nil, fail(CodeInvalidArgument, "no device on "+n.Network.DisplayName+" advertises "+p.String(), false)
		}
		for _, other := range nets {
			if other.Network.ID != id && slices.Contains(other.Network.UsedRoutes, p) {
				return nil, fail(CodeInvalidArgument, p.String()+" is already used from "+other.Network.DisplayName+"; stop using it there first", false)
			}
		}
	}
	next := mergeRoutes(n.Network.UsedRoutes, toAdd, toRemove)
	if len(next) > maxAdvertisedRoutes {
		return nil, fail(CodeInvalidArgument, "too many routes", false)
	}
	if !slices.Equal(n.Network.UsedRoutes, next) {
		if err := s.cfg.Store.Networks().SetUsedRoutes(ctx, id, next); err != nil {
			return nil, s.storeErr(err)
		}
	}
	s.nudgeRoutes()
	return s.SubnetRoutes(ctx, string(id))
}

func (s *Service) SystemRoutes(ctx context.Context) []SystemRoute {
	nets, _, err := s.liveNetworks(ctx)
	if err != nil {
		return nil
	}
	var out []SystemRoute
	for _, n := range nets {
		if !n.Live {
			continue
		}
		routers := advertisedRoutes(n)
		for _, p := range n.Network.UsedRoutes {
			if _, ok := routers[p]; ok && !slices.ContainsFunc(out, func(r SystemRoute) bool { return r.Prefix == p }) {
				out = append(out, SystemRoute{Prefix: p, Network: n.Network.ID})
			}
		}
	}
	slices.SortFunc(out, func(a, b SystemRoute) int { return a.Prefix.Compare(b.Prefix) })
	return out
}

func (s *Service) SetInstalledRoutes(active bool, installed []netip.Prefix, refused map[netip.Prefix]string) {
	s.mu.Lock()
	s.routes = installedRoutes{active: active, installed: slices.Clone(installed), refused: refused}
	s.mu.Unlock()
}

func (s *Service) RoutesChanged() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.routesNudge == nil {
		s.routesNudge = make(chan struct{}, 1)
	}
	return s.routesNudge
}

func (s *Service) nudgeRoutes() {
	select {
	case s.RoutesChanged() <- struct{}{}:
	default:
	}
}

func (s *Service) SystemRouteFor(a netip.Addr) (netip.Prefix, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var best netip.Prefix
	for _, p := range s.routes.installed {
		if p.Contains(a) && (!best.IsValid() || p.Bits() > best.Bits()) {
			best = p
		}
	}
	return best, best.IsValid()
}

func advertisedRoutes(n inspect.Network) map[netip.Prefix][]domain.Device {
	out := map[netip.Prefix][]domain.Device{}
	for _, d := range n.Devices {
		if d.Local {
			continue
		}
		for _, p := range d.Routes {
			if p.Bits() > 0 && !slices.ContainsFunc(domain.TailnetPrefixes, p.Overlaps) {
				out[p] = append(out[p], d)
			}
		}
	}
	for p := range out {
		slices.SortFunc(out[p], func(a, b domain.Device) int { return cmp.Compare(a.Hostname, b.Hostname) })
	}
	return out
}

func overlapping(nets []inspect.Network, self domain.NetworkID, p netip.Prefix) []domain.Network {
	var out []domain.Network
	for _, n := range nets {
		if !n.Live || n.Network.ID == self {
			continue
		}
		hit := slices.ContainsFunc(n.Network.UsedRoutes, p.Overlaps)
		for q := range advertisedRoutes(n) {
			hit = hit || q.Overlaps(p)
		}
		if hit {
			out = append(out, n.Network)
		}
	}
	return out
}
