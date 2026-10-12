package dataplane

import (
	"net/netip"
	"slices"
	"sync"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
)

type RouteTable struct {
	mu        sync.RWMutex
	networks  map[netip.Prefix]domain.NetworkID
	installed []netip.Prefix
}

func (t *RouteTable) Want(networks map[netip.Prefix]domain.NetworkID) {
	t.mu.Lock()
	t.networks = networks
	t.mu.Unlock()
}

func (t *RouteTable) Install(installed []netip.Prefix) {
	t.mu.Lock()
	t.installed = slices.Clone(installed)
	t.mu.Unlock()
}

func (t *RouteTable) Lookup(a netip.Addr) (domain.NetworkID, bool) {
	if t == nil {
		return "", false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	var best netip.Prefix
	for _, p := range t.installed {
		if _, ok := t.networks[p]; ok && p.Contains(a) && (!best.IsValid() || p.Bits() > best.Bits()) {
			best = p
		}
	}
	if !best.IsValid() {
		return "", false
	}
	return t.networks[best], true
}
