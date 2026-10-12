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

func (s *Service) UpdateRoutes(ctx context.Context, rawNetwork string, add, remove []string) (domain.LocalNode, error) {
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
	current := routePrefixes(sess.LocalNode())
	next := mergeRoutes(current, toAdd, toRemove)
	if len(next) > maxAdvertisedRoutes {
		return domain.LocalNode{}, fail(CodeInvalidArgument, "too many advertised routes", false)
	}
	if !slices.Equal(current, next) {
		err := sess.SetAdvertisedRoutes(ctx, next)
		if err = s.sessionError(sess, err, "setting advertised routes failed", "could not change the advertised routes"); err != nil {
			return domain.LocalNode{}, err
		}
		if err := s.cfg.Store.Networks().SetAdvertisedRoutes(ctx, id, next); err != nil {
			return domain.LocalNode{}, s.storeErr(err)
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

var exitRoutes = []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0"), netip.MustParsePrefix("::/0")}

func routePrefixes(n domain.LocalNode) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(n.Routes)+2)
	for _, r := range n.Routes {
		out = append(out, r.Prefix)
	}
	if n.OffersExitNode {
		out = append(out, exitRoutes...)
	}
	slices.SortFunc(out, netip.Prefix.Compare)
	return out
}

func (s *Service) OfferExitNode(ctx context.Context, rawNetwork string, offer bool) (domain.LocalNode, error) {
	id, err := parseID(rawNetwork)
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
	current := routePrefixes(sess.LocalNode())
	var next []netip.Prefix
	if offer {
		next = mergeRoutes(current, exitRoutes, nil)
	} else {
		next = mergeRoutes(current, nil, exitRoutes)
	}
	if !slices.Equal(current, next) {
		err := sess.SetAdvertisedRoutes(ctx, next)
		if err = s.sessionError(sess, err, "changing the exit node offer failed", "could not change the exit node offer"); err != nil {
			return domain.LocalNode{}, err
		}
		if err := s.cfg.Store.Networks().SetAdvertisedRoutes(ctx, id, next); err != nil {
			return domain.LocalNode{}, s.storeErr(err)
		}
	}
	return sess.LocalNode(), nil
}
