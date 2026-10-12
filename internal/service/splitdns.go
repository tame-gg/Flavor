package service

import (
	"context"
	"math/rand/v2"
	"net"
	"net/netip"
	"slices"
	"strings"
	"time"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/inspect"
	"git.lunarlabs.dev/flavor/flavor/internal/session"
	"golang.org/x/net/dns/dnsmessage"
)

const (
	splitDNSTimeout  = 3 * time.Second
	splitQueryBudget = time.Second
	splitDNSTTL      = 30 * time.Second
	splitDNSEntries  = 1024
)

type splitAnswer struct {
	addrs   []netip.Addr
	expires time.Time
}

func (s *Service) resolveSplitDNS(ctx context.Context, q inspect.Query, nets []inspect.Network) {
	if q.Kind != inspect.KindName || q.Qualified() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, splitDNSTimeout)
	defer cancel()
	for i := range nets {
		n := &nets[i]
		if !n.Live {
			continue
		}
		sess, ok := s.cfg.Sessions.Get(n.Network.ID)
		if !ok {
			continue
		}
		dns := sess.DNS()
		for _, name := range inspect.SearchNames(q.Name, n.SearchDomains) {
			if publishes(*n, name) {
				break
			}
			route, ok := dns.RouteFor(name)
			if !ok {
				continue
			}
			if addrs := s.cachedLookup(ctx, sess, *n, route, name); len(addrs) > 0 {
				n.Records = append(n.Records, domain.DNSRecord{Name: name, Addresses: addrs})
				break
			}
		}
	}
}

func (s *Service) cachedLookup(ctx context.Context, sess *session.Session, n inspect.Network, route domain.DNSRoute, name string) []netip.Addr {
	key := string(n.Network.ID) + "|" + name
	s.mu.Lock()
	a, ok := s.splitDNS[key]
	s.mu.Unlock()
	if ok && time.Now().Before(a.expires) {
		return a.addrs
	}
	addrs := lookupRoute(ctx, sess, n, route, name)
	s.mu.Lock()
	if s.splitDNS == nil || len(s.splitDNS) >= splitDNSEntries {
		s.splitDNS = make(map[string]splitAnswer)
	}
	s.splitDNS[key] = splitAnswer{addrs: addrs, expires: time.Now().Add(splitDNSTTL)}
	s.mu.Unlock()
	return addrs
}

func lookupRoute(ctx context.Context, sess *session.Session, n inspect.Network, route domain.DNSRoute, name string) []netip.Addr {
	for _, resolver := range route.Resolvers {
		dial := (&net.Dialer{}).DialContext
		if reachable(n, resolver.Addr()) {
			dial = sess.Dial
		}
		if addrs := lookup(ctx, dial, resolver, name); len(addrs) > 0 {
			return addrs
		}
	}
	return nil
}

func publishes(n inspect.Network, name string) bool {
	for _, r := range n.Records {
		if r.Name == name {
			return true
		}
	}
	for _, d := range n.Devices {
		if domain.NormalizeDNSName(d.DNSName) == name {
			return true
		}
	}
	return false
}

func reachable(n inspect.Network, a netip.Addr) bool {
	if n.ExitNode.ID.NodeID != "" {
		return true
	}
	for _, d := range n.Devices {
		if slices.Contains(d.Addresses, a) || slices.ContainsFunc(d.Routes, func(p netip.Prefix) bool { return p.Contains(a) }) {
			return true
		}
	}
	return false
}

func lookup(ctx context.Context, dial func(context.Context, string, string) (net.Conn, error), resolver netip.AddrPort, name string) []netip.Addr {
	var out []netip.Addr
	for _, t := range []dnsmessage.Type{dnsmessage.TypeA, dnsmessage.TypeAAAA} {
		qctx, cancel := context.WithTimeout(ctx, splitQueryBudget)
		addrs, err := query(qctx, dial, resolver, name, t)
		cancel()
		if err == nil {
			out = append(out, addrs...)
		}
	}
	return out
}

func query(ctx context.Context, dial func(context.Context, string, string) (net.Conn, error), resolver netip.AddrPort, name string, t dnsmessage.Type) ([]netip.Addr, error) {
	qname, err := dnsmessage.NewName(name + ".")
	if err != nil {
		return nil, err
	}
	id := uint16(rand.Uint32())
	msg, err := (&dnsmessage.Message{
		Header:    dnsmessage.Header{ID: id, RecursionDesired: true},
		Questions: []dnsmessage.Question{{Name: qname, Type: t, Class: dnsmessage.ClassINET}},
	}).Pack()
	if err != nil {
		return nil, err
	}
	conn, err := dial(ctx, "udp", resolver.String())
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if d, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(d)
	}
	if _, err := conn.Write(msg); err != nil {
		return nil, err
	}
	buf := make([]byte, 1232)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return nil, err
		}
		var resp dnsmessage.Message
		if resp.Unpack(buf[:n]) != nil || resp.ID != id || !resp.Response || len(resp.Questions) != 1 || !sameName(resp.Questions[0].Name, qname) || resp.Questions[0].Type != t {
			continue
		}
		return answers(resp.Answers, qname), nil
	}
}

func answers(rrs []dnsmessage.Resource, qname dnsmessage.Name) []netip.Addr {
	owners := []dnsmessage.Name{qname}
	var out []netip.Addr
	for _, rr := range rrs {
		if !slices.ContainsFunc(owners, func(o dnsmessage.Name) bool { return sameName(o, rr.Header.Name) }) {
			continue
		}
		switch r := rr.Body.(type) {
		case *dnsmessage.CNAMEResource:
			owners = append(owners, r.CNAME)
		case *dnsmessage.AResource:
			out = append(out, netip.AddrFrom4(r.A))
		case *dnsmessage.AAAAResource:
			out = append(out, netip.AddrFrom16(r.AAAA).Unmap())
		}
	}
	return out
}

func sameName(a, b dnsmessage.Name) bool {
	return strings.EqualFold(a.String(), b.String())
}

type NetworkDNS struct {
	Network domain.Network
	State   domain.NetworkConnectionState
	DNS     domain.NetworkDNS
}

func (s *Service) ListDNS(ctx context.Context, rawID string) ([]NetworkDNS, error) {
	var id domain.NetworkID
	if rawID != "" {
		var err error
		if id, err = parseID(rawID); err != nil {
			return nil, err
		}
		if _, err := s.cfg.Store.Networks().Get(ctx, id); err != nil {
			return nil, s.storeErr(err)
		}
	}
	nets, _, err := s.liveNetworks(ctx)
	if err != nil {
		return nil, err
	}
	var out []NetworkDNS
	for _, n := range nets {
		if id != "" && n.Network.ID != id {
			continue
		}
		v := NetworkDNS{Network: n.Network, State: n.State}
		if n.Live {
			v.DNS = s.dns(n.Network.ID)
		}
		out = append(out, v)
	}
	return out, nil
}
