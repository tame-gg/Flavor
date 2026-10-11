package service

import (
	"context"
	"net/netip"
	"slices"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
)

const (
	opRoutes = "routes"

	maxAdvertisedRoutes = 64
)

func (s *Service) GetRoutes(ctx context.Context, rawNetwork string) (domain.LocalNode, error) {
	id, err := parseID(rawNetwork)
	if err != nil {
		return domain.LocalNode{}, err
	}
	sess, err := s.connectedSession(ctx, id)
	if err != nil {
		return domain.LocalNode{}, err
	}
	return sess.LocalNode(), nil
}

func (s *Service) UpdateRoutes(ctx context.Context, rawNetwork string, add, remove []string, exitNode *bool) (domain.LocalNode, error) {
	id, err := parseID(rawNetwork)
	if err != nil {
		return domain.LocalNode{}, err
	}
	toAdd, err := parseRoutes(add, true)
	if err != nil {
		return domain.LocalNode{}, err
	}
	toRemove, err := parseRoutes(remove, false)
	if err != nil {
		return domain.LocalNode{}, err
	}
	release, err := s.acquire(id, opRoutes)
	if err != nil {
		return domain.LocalNode{}, err
	}
	defer release()
	sess, err := s.connectedSession(ctx, id)
	if err != nil {
		return domain.LocalNode{}, err
	}
	local := sess.LocalNode()
	current := advertisedPrefixes(local)
	offered := local.ExitNode.Offered
	if exitNode != nil {
		offered = *exitNode
	}
	if offered && !local.ExitNode.Offered && slices.ContainsFunc(sess.Devices(), func(d domain.Device) bool { return d.ExitNode }) {
		return domain.LocalNode{}, fail(CodeInvalidArgument, "stop using an exit node before offering this machine as one", false)
	}
	next := mergeRoutes(routePrefixes(local), toAdd, toRemove)
	if len(next) > maxAdvertisedRoutes {
		return domain.LocalNode{}, fail(CodeInvalidArgument, "too many advertised routes", false)
	}
	if offered {
		next = append(next, domain.ExitRoutes...)
	}
	if !slices.Equal(current, next) {
		err := sess.SetAdvertisedRoutes(ctx, next)
		if err = s.sessionError(sess, err, "setting advertised routes failed", "could not change the advertised routes"); err != nil {
			return domain.LocalNode{}, err
		}
	}
	return sess.LocalNode(), nil
}

func parseRoutes(raw []string, rejectTailnet bool) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(raw))
	for _, r := range raw {
		p, err := netip.ParsePrefix(r)
		if err != nil {
			return nil, fail(CodeInvalidArgument, "routes must be CIDR prefixes such as 192.168.1.0/24", false)
		}
		p = p.Masked()
		if p.Bits() == 0 {
			return nil, fail(CodeInvalidArgument, "default routes are not accepted here, use the exit node option", false)
		}
		if rejectTailnet && slices.ContainsFunc(domain.TailnetPrefixes, p.Overlaps) {
			return nil, fail(CodeInvalidArgument, "routes must not overlap the Tailscale address ranges", false)
		}
		out = append(out, p)
	}
	return out, nil
}

func mergeRoutes(current, add, remove []netip.Prefix) []netip.Prefix {
	next := slices.Clone(current)
	for _, p := range add {
		if !slices.Contains(next, p) {
			next = append(next, p)
		}
	}
	next = slices.DeleteFunc(next, func(p netip.Prefix) bool { return slices.Contains(remove, p) })
	slices.SortFunc(next, netip.Prefix.Compare)
	return next
}

func routePrefixes(n domain.LocalNode) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(n.Routes))
	for _, r := range n.Routes {
		out = append(out, r.Prefix)
	}
	return out
}

func advertisedPrefixes(n domain.LocalNode) []netip.Prefix {
	out := routePrefixes(n)
	if n.ExitNode.Offered {
		out = append(out, domain.ExitRoutes...)
	}
	return out
}
