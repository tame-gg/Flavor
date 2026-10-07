package session

import (
	"log/slog"
	"net/netip"
	"time"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/events"
	"git.lunarlabs.dev/lattice/lattice/internal/provider"
)

type Engine = engine

func NewSessionWithFactory(cfg provider.ResolvedSessionConfig, bus *events.Bus, log *slog.Logger, f engineFactory) (*Session, error) {
	return newSession(cfg, bus, log, f)
}

func FakeFactory(shared *fakeEngine, counter *int) engineFactory {
	return newFakeFactory(shared, counter)
}

func NewFakeEngine() *fakeEngine {
	ip := netip.MustParseAddr("100.64.0.1")
	return &fakeEngine{
		notifies: make(chan notifySnap, 16),
		watchErr: make(chan error, 1),
		status: statusSnap{
			BackendState: "Running",
			TailscaleIPs: []netip.Addr{ip},
			Self: &peerSnap{
				NodeID:    "node-self",
				Hostname:  "host",
				DNSName:   "host.tailnet.ts.net",
				Addresses: []netip.Addr{ip},
				Online:    true,
			},
		},
	}
}

func (m *Manager) SetEngineFactory(f engineFactory) {
	m.withEngineFactory(f)
}

type FakeEngineHolder struct {
	E *fakeEngine
}

func WrapFakeEngine(e *fakeEngine, authKey string) (Engine, error) {
	e.mu.Lock()
	e.authKey = authKey
	e.mu.Unlock()
	return e, nil
}

func (f *fakeEngine) SetStartErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startErr = err
}

func (f *fakeEngine) SetSlowStart(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.slowStart = d
}

func (f *fakeEngine) AuthKeyCleared() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cleared
}

func (f *fakeEngine) PushBrowse(url string) {
	u := url
	f.push(notifySnap{BrowseToURL: &u})
}

func (f *fakeEngine) PushNetMap() {
	f.push(notifySnap{NetMapChanged: true})
}

func (f *fakeEngine) SetStatus(st statusSnap) { f.setStatus(st) }

func StatusNeedsLogin() statusSnap {
	return statusSnap{BackendState: "NeedsLogin"}
}

func StatusNeedsMachineAuth() statusSnap {
	return statusSnap{BackendState: "NeedsMachineAuth"}
}

func StatusSelf(nodeID, ip string) statusSnap {
	addr := netip.MustParseAddr(ip)
	return statusSnap{
		BackendState: "Running",
		TailscaleIPs: []netip.Addr{addr},
		Self:         selfSnap(nodeID, addr),
	}
}

func StatusWithPeer(nodeID, ip string) statusSnap {
	st := StatusSelf("node-self", "100.64.0.1")
	addr := netip.MustParseAddr(ip)
	st.Peers = []peerSnap{{
		NodeID:    domain.NodeID(nodeID),
		Hostname:  string(nodeID),
		DNSName:   string(nodeID) + ".ts.net",
		Addresses: []netip.Addr{addr},
		Online:    true,
	}}
	return st
}
