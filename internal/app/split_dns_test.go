package app_test

import (
	"context"
	"net"
	"net/netip"
	"testing"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/v1"
	"git.lunarlabs.dev/flavor/flavor/internal/app"
	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/provider"
	"git.lunarlabs.dev/flavor/flavor/internal/session/sessiontest"
	"golang.org/x/net/dns/dnsmessage"
)

func fakeResolver(conn net.Conn, answers map[string]netip.Addr) {
	defer conn.Close()
	buf := make([]byte, 512)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		var q dnsmessage.Message
		if q.Unpack(buf[:n]) != nil || len(q.Questions) != 1 {
			return
		}
		resp := dnsmessage.Message{Header: dnsmessage.Header{ID: q.ID, Response: true}, Questions: q.Questions}
		if a, ok := answers[q.Questions[0].Name.String()]; ok && q.Questions[0].Type == dnsmessage.TypeA {
			resp.Answers = []dnsmessage.Resource{{
				Header: dnsmessage.ResourceHeader{Name: q.Questions[0].Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 60},
				Body:   &dnsmessage.AResource{A: a.As4()},
			}}
		}
		b, _ := resp.Pack()
		if _, err := conn.Write(b); err != nil {
			return
		}
	}
}

func TestSplitDNSResolvesThroughTheNetwork(t *testing.T) {
	engines := &sessiontest.Sequence{Prepare: func(i int, cfg provider.ResolvedSessionConfig, e *sessiontest.Engine) {
		st := sessiontest.StatusWithPeer("dns-"+string(cfg.NetworkID), "100.64.0.53")
		st.Self.NodeID = domain.NodeID("self-" + string(cfg.NetworkID))
		st.Peers[0].Hostname = "ns"
		st.DNS = domain.NetworkDNS{
			Routes:        []domain.DNSRoute{{Suffix: "corp.example", Resolvers: []netip.AddrPort{netip.MustParseAddrPort("100.64.0.53:53")}}},
			SearchDomains: []string{"corp.example"},
		}
		e.SetStatus(st)
		e.SetDial(func(_ context.Context, network, address string) (net.Conn, error) {
			ours, theirs := net.Pipe()
			if network == "udp" && address == "100.64.0.53:53" {
				go fakeResolver(theirs, map[string]netip.Addr{"wiki.corp.example.": netip.MustParseAddr("10.1.2.3")})
			} else {
				theirs.Close()
			}
			return ours, nil
		})
	}}
	d := start(t, testEnv(t), app.Options{EngineFactory: engines.Factory})
	n := addHeadscale(t, d.client, "Corp", "https://corp.example.com", false)
	connectNetworks(t, d.client, n)

	for _, dest := range []string{"wiki.corp.example", "wiki"} {
		r := inspectDestination(t, d.client, dest)
		if r.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_UNIQUE || r.DecidedBy != v1.MatchKind_MATCH_KIND_DNS_RECORD || r.Candidates[0].MatchedValue != "10.1.2.3" {
			t.Fatalf("%s: %+v", dest, r)
		}
	}
	if r := inspectDestination(t, d.client, "missing.corp.example"); r.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_NO_MATCH {
		t.Fatalf("%+v", r)
	}

	res, err := d.client.DNS.ListDNS(context.Background(), connect.NewRequest(&v1.ListDNSRequest{NetworkId: n.Id}))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Msg.Networks) != 1 || len(res.Msg.Networks[0].Routes) != 1 || res.Msg.Networks[0].Routes[0].Resolvers[0] != "100.64.0.53:53" || res.Msg.Networks[0].SearchDomains[0] != "corp.example" {
		t.Fatalf("%+v", res.Msg)
	}
}
