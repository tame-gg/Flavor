package app_test

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/lattice/lattice/gen/go/lattice/v1"
	"git.lunarlabs.dev/lattice/lattice/internal/app"
	"git.lunarlabs.dev/lattice/lattice/internal/syndns"
	"git.lunarlabs.dev/lattice/lattice/internal/synthetic"
	"golang.org/x/net/dns/dnsmessage"
)

type dnsAnswer struct {
	rcode dnsmessage.RCode
	addrs []netip.Addr
	ttl   uint32
	soa   *dnsmessage.SOAResource
	ede   string
}

func ask(t *testing.T, server string, name string, qtype dnsmessage.Type, edns bool) dnsAnswer {
	t.Helper()
	c, err := net.Dial("udp", server)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return askOn(t, c, false, name, qtype, edns)
}

func askOn(t *testing.T, c net.Conn, framed bool, name string, qtype dnsmessage.Type, edns bool) dnsAnswer {
	t.Helper()
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: 7, RecursionDesired: true})
	_ = b.StartQuestions()
	_ = b.Question(dnsmessage.Question{Name: dnsmessage.MustNewName(name + "."), Type: qtype, Class: dnsmessage.ClassINET})
	if edns {
		_ = b.StartAdditionals()
		var h dnsmessage.ResourceHeader
		_ = h.SetEDNS0(1232, dnsmessage.RCodeSuccess, false)
		_ = b.OPTResource(h, dnsmessage.OPTResource{})
	}
	q, _ := b.Finish()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	if framed {
		q = append(binary.BigEndian.AppendUint16(nil, uint16(len(q))), q...)
	}
	if _, err := c.Write(q); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1500)
	var n int
	var err error
	if framed {
		if _, err = io.ReadFull(c, buf[:2]); err == nil {
			n = int(binary.BigEndian.Uint16(buf[:2]))
			_, err = io.ReadFull(c, buf[:n])
		}
	} else {
		n, err = c.Read(buf)
	}
	if err != nil {
		t.Fatal(err)
	}
	msg := new(dnsmessage.Message)
	if err := msg.Unpack(buf[:n]); err != nil {
		t.Fatal(err)
	}
	out := dnsAnswer{rcode: msg.Header.RCode}
	for _, a := range msg.Answers {
		out.ttl = a.Header.TTL
		switch r := a.Body.(type) {
		case *dnsmessage.AResource:
			out.addrs = append(out.addrs, netip.AddrFrom4(r.A))
		case *dnsmessage.AAAAResource:
			out.addrs = append(out.addrs, netip.AddrFrom16(r.AAAA))
		}
	}
	for _, a := range msg.Authorities {
		if s, ok := a.Body.(*dnsmessage.SOAResource); ok {
			out.soa = s
		}
	}
	for _, a := range msg.Additionals {
		if o, ok := a.Body.(*dnsmessage.OPTResource); ok {
			for _, opt := range o.Options {
				if opt.Code == 15 && len(opt.Data) >= 2 {
					out.ede = string(opt.Data[2:])
				}
			}
		}
	}
	return out
}

func TestExperimentalSyntheticDNS(t *testing.T) {
	ready := make(chan net.Addr, 1)
	d := start(t, testEnv(t), app.Options{
		EngineFactory:   identifyingEngines().Factory,
		ExperimentalDNS: "127.0.0.1:0",
		OnDNSReady:      func(a net.Addr) { ready <- a },
	})
	server := (<-ready).String()
	c := d.client
	ctx := context.Background()
	a := addHeadscale(t, c, "LunarLabs", "https://lunar.example.com", false)
	b := addHeadscale(t, c, "Home", "https://home.example.com", false)
	for _, id := range []string{a.Id, b.Id} {
		if _, err := c.Networks.ConnectNetwork(ctx, connect.NewRequest(&v1.ConnectNetworkRequest{NetworkId: id})); err != nil {
			t.Fatal(err)
		}
		eventually(t, "connected", func() bool {
			return networkState(t, c, id) == v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_CONNECTED
		})
	}
	base := snapshot(t, c)
	col, stopWatch := watch(t, c, base.DaemonInstanceId, base.SnapshotSequence)
	defer stopWatch()

	home := ask(t, server, "postgres.home.lattice.internal", dnsmessage.TypeAAAA, false)
	lunar := ask(t, server, "postgres.lunarlabs.lattice.internal", dnsmessage.TypeAAAA, false)
	if home.rcode != dnsmessage.RCodeSuccess || len(home.addrs) != 1 || home.ttl != syndns.TTL {
		t.Fatalf("%+v", home)
	}
	if len(lunar.addrs) != 1 || lunar.addrs[0] == home.addrs[0] {
		t.Fatalf("the two postgres devices must get distinct synthetic addresses: %v %v", home.addrs, lunar.addrs)
	}
	if !netip.MustParsePrefix("fd00::/8").Contains(home.addrs[0]) {
		t.Fatalf("not a ULA: %v", home.addrs[0])
	}

	insp, err := c.Inspector.InspectDestination(ctx, connect.NewRequest(&v1.InspectDestinationRequest{Destination: "postgres.home.lattice.internal"}))
	if err != nil {
		t.Fatal(err)
	}
	stable := ask(t, server, insp.Msg.Candidates[0].StableName, dnsmessage.TypeAAAA, false)
	if len(stable.addrs) != 1 || stable.addrs[0] != home.addrs[0] {
		t.Fatalf("stable and friendly names must answer the same: %v vs %v", stable.addrs, home.addrs)
	}

	v4 := ask(t, server, "postgres.home.lattice.internal", dnsmessage.TypeA, false)
	lunar4 := ask(t, server, "postgres.lunarlabs.lattice.internal", dnsmessage.TypeA, false)
	if v4.rcode != dnsmessage.RCodeSuccess || len(v4.addrs) != 1 || !synthetic.CompatibilityRange.Contains(v4.addrs[0]) {
		t.Fatalf("A answers come from the IPv4 compatibility pool: %+v", v4)
	}
	if len(lunar4.addrs) != 1 || lunar4.addrs[0] == v4.addrs[0] {
		t.Fatalf("the two postgres devices must get distinct IPv4 synthetic addresses: %v %v", v4.addrs, lunar4.addrs)
	}

	amb := ask(t, server, "postgres", dnsmessage.TypeAAAA, true)
	if amb.rcode != dnsmessage.RCodeServerFailure || len(amb.addrs) != 0 {
		t.Fatalf("ambiguous name must fail, not pick: %+v", amb)
	}
	if amb.ede != "Ambiguous Lattice destination: 2 network candidates" {
		t.Fatalf("extended error %q", amb.ede)
	}
	if plain := ask(t, server, "postgres", dnsmessage.TypeAAAA, false); plain.ede != "" || plain.rcode != dnsmessage.RCodeServerFailure {
		t.Fatalf("no EDNS in the query, so no OPT in the answer: %+v", plain)
	}
	eventually(t, "ambiguity warning published", func() bool {
		for _, ev := range col.snapshot() {
			if w := ev.GetDaemonWarning(); w != nil && w.Code == "dns_ambiguous" && strings.Contains(w.SafeMessage, "postgres") {
				return true
			}
		}
		return false
	})

	if _, err := c.Preferences.SetDestinationPreference(ctx, connect.NewRequest(&v1.SetDestinationPreferenceRequest{Destination: "postgres", NetworkId: b.Id})); err != nil {
		t.Fatal(err)
	}
	if pref := ask(t, server, "postgres", dnsmessage.TypeAAAA, false); len(pref.addrs) != 1 || pref.addrs[0] != home.addrs[0] {
		t.Fatalf("preference must resolve the ambiguity immediately: %+v", pref)
	}

	missing := ask(t, server, "nothing.home.lattice.internal", dnsmessage.TypeAAAA, false)
	if missing.rcode != dnsmessage.RCodeNameError || missing.soa == nil || missing.soa.MinTTL != syndns.TTL {
		t.Fatalf("absent name under the zone is NXDOMAIN with a short negative TTL: %+v", missing)
	}
	if out := ask(t, server, "example.com", dnsmessage.TypeAAAA, false); out.rcode != dnsmessage.RCodeRefused {
		t.Fatalf("names outside Lattice are refused: %+v", out)
	}

	addHeadscale(t, c, "home", "https://home2.example.com", false)
	if dup := ask(t, server, "postgres.home.lattice.internal", dnsmessage.TypeAAAA, false); dup.rcode != dnsmessage.RCodeServerFailure {
		t.Fatalf("duplicate friendly network labels must not publish an answer: %+v", dup)
	}
	if again := ask(t, server, insp.Msg.Candidates[0].StableName, dnsmessage.TypeAAAA, false); len(again.addrs) != 1 || again.addrs[0] != home.addrs[0] {
		t.Fatalf("stable name keeps working when the friendly label collides: %+v", again)
	}
}
