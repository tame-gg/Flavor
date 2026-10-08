package syndns

import (
	"context"
	"encoding/binary"
	"fmt"
	"net/netip"
	"strings"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/inspect"
	"git.lunarlabs.dev/flavor/flavor/internal/naming"
	"git.lunarlabs.dev/flavor/flavor/internal/synthetic"
	"golang.org/x/net/dns/dnsmessage"
)

const (
	TTL             uint32 = 60
	edeOptionCode   uint16 = 15
	edeInfoCodeNone uint16 = 0
)

type Resolver interface {
	Inspect(ctx context.Context, destination string) (inspect.Result, uint64, error)
}

type Addresser interface {
	For(ctx context.Context, network domain.NetworkID, real netip.Addr) (synthetic.Addresses, error)
}

type Engine struct {
	Resolver  Resolver
	Addresser Addresser
	Ambiguous func(name string, candidates, networks int)
}

type outcome struct {
	rcode dnsmessage.RCode
	addrs []netip.Addr
	ede   string
}

func (e *Engine) Answer(ctx context.Context, query []byte) ([]byte, error) {
	var p dnsmessage.Parser
	h, err := p.Start(query)
	if err != nil {
		return nil, err
	}
	q, err := p.Question()
	if err != nil {
		return nil, err
	}
	_ = p.SkipAllQuestions()
	_ = p.SkipAllAnswers()
	_ = p.SkipAllAuthorities()
	edns := false
	for {
		ah, err := p.AdditionalHeader()
		if err != nil {
			break
		}
		edns = edns || ah.Type == dnsmessage.TypeOPT
		_ = p.SkipAdditional()
	}

	out := e.decide(ctx, q)
	resp := dnsmessage.Header{ID: h.ID, Response: true, Authoritative: true, RecursionDesired: h.RecursionDesired, RCode: out.rcode}
	b := dnsmessage.NewBuilder(nil, resp)
	b.EnableCompression()
	if err := b.StartQuestions(); err != nil {
		return nil, err
	}
	if err := b.Question(q); err != nil {
		return nil, err
	}
	if err := b.StartAnswers(); err != nil {
		return nil, err
	}
	for _, a := range out.addrs {
		rh := dnsmessage.ResourceHeader{Name: q.Name, Class: dnsmessage.ClassINET, TTL: TTL}
		if a.Is4() {
			err = b.AResource(rh, dnsmessage.AResource{A: a.As4()})
		} else {
			err = b.AAAAResource(rh, dnsmessage.AAAAResource{AAAA: a.As16()})
		}
		if err != nil {
			return nil, err
		}
	}
	if err := b.StartAuthorities(); err != nil {
		return nil, err
	}
	if len(out.addrs) == 0 && (out.rcode == dnsmessage.RCodeSuccess || out.rcode == dnsmessage.RCodeNameError) {
		if err := b.SOAResource(soaHeader(), soa()); err != nil {
			return nil, err
		}
	}
	if err := b.StartAdditionals(); err != nil {
		return nil, err
	}
	if edns {
		var opt dnsmessage.ResourceHeader
		if err := opt.SetEDNS0(1232, dnsmessage.RCodeSuccess, false); err != nil {
			return nil, err
		}
		var r dnsmessage.OPTResource
		if out.ede != "" {
			data := make([]byte, 2, 2+len(out.ede))
			binary.BigEndian.PutUint16(data, edeInfoCodeNone)
			r.Options = append(r.Options, dnsmessage.Option{Code: edeOptionCode, Data: append(data, out.ede...)})
		}
		if err := b.OPTResource(opt, r); err != nil {
			return nil, err
		}
	}
	return b.Finish()
}

func (e *Engine) decide(ctx context.Context, q dnsmessage.Question) outcome {
	if q.Class != dnsmessage.ClassINET {
		return outcome{rcode: dnsmessage.RCodeRefused}
	}
	name := strings.TrimSuffix(strings.ToLower(q.Name.String()), ".")
	inZone := name == naming.Suffix || strings.HasSuffix(name, "."+naming.Suffix)
	res, _, err := e.Resolver.Inspect(ctx, name)
	switch {
	case err != nil && inZone:
		return outcome{rcode: dnsmessage.RCodeNameError}
	case err != nil:
		return outcome{rcode: dnsmessage.RCodeRefused}
	case res.Query.Kind == inspect.KindAddress:
		return outcome{rcode: dnsmessage.RCodeRefused}
	}
	switch res.Decision {
	case inspect.DecisionNoMatch:
		if inZone {
			return outcome{rcode: dnsmessage.RCodeNameError}
		}
		return outcome{rcode: dnsmessage.RCodeRefused}
	case inspect.DecisionAmbiguous:
		candidates, networks := tied(res)
		if e.Ambiguous != nil {
			e.Ambiguous(name, candidates, networks)
		}
		return outcome{rcode: dnsmessage.RCodeServerFailure, ede: fmt.Sprintf("Ambiguous Flavor destination: %d network candidates", networks)}
	}
	var sel inspect.Candidate
	for _, c := range res.Candidates {
		if c.Status == inspect.StatusSelected {
			sel = c
		}
	}
	real, ok := preferredAddress(sel.Device)
	if !ok {
		return outcome{rcode: dnsmessage.RCodeSuccess}
	}
	addrs, err := e.Addresser.For(ctx, sel.Network.ID, real)
	if err != nil {
		return outcome{rcode: dnsmessage.RCodeServerFailure}
	}
	switch q.Type {
	case dnsmessage.TypeAAAA:
		return outcome{rcode: dnsmessage.RCodeSuccess, addrs: valid(addrs.V6)}
	case dnsmessage.TypeA:
		return outcome{rcode: dnsmessage.RCodeSuccess, addrs: valid(addrs.V4)}
	}
	return outcome{rcode: dnsmessage.RCodeSuccess}
}

func valid(a netip.Addr) []netip.Addr {
	if a.IsValid() {
		return []netip.Addr{a}
	}
	return nil
}

func preferredAddress(d domain.Device) (netip.Addr, bool) {
	for _, a := range d.Addresses {
		if a.Unmap().Is4() {
			return a.Unmap(), true
		}
	}
	if len(d.Addresses) > 0 {
		return d.Addresses[0], true
	}
	return netip.Addr{}, false
}

func tied(res inspect.Result) (candidates, networks int) {
	seen := map[domain.NetworkID]bool{}
	for _, c := range res.Candidates {
		if c.Status == inspect.StatusTied {
			candidates++
			seen[c.Network.ID] = true
		}
	}
	return candidates, len(seen)
}

func soaHeader() dnsmessage.ResourceHeader {
	return dnsmessage.ResourceHeader{Name: dnsmessage.MustNewName(naming.Suffix + "."), Class: dnsmessage.ClassINET, TTL: TTL}
}

func soa() dnsmessage.SOAResource {
	return dnsmessage.SOAResource{
		NS:      dnsmessage.MustNewName("ns." + naming.Suffix + "."),
		MBox:    dnsmessage.MustNewName("hostmaster." + naming.Suffix + "."),
		Serial:  1,
		Refresh: 3600,
		Retry:   600,
		Expire:  86400,
		MinTTL:  TTL,
	}
}
