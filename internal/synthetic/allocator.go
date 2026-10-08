package synthetic

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/store"
	"git.lunarlabs.dev/flavor/flavor/internal/synthetic/layout"
)

var (
	CompatibilityRange = netip.MustParsePrefix("198.18.0.0/15")
	ErrPoolOverlaps    = errors.New("no free IPv4 compatibility prefix: every candidate overlaps a local route or address")
	ErrPoolOutside     = errors.New("IPv4 pool must be inside 198.18.0.0/15")
	ErrInUse           = errors.New("synthetic address has active flows")
)

const PoolBits = 20

type Allocator struct {
	db  *store.DB
	now func() time.Time
	ula netip.Prefix

	mu     sync.Mutex
	pool   netip.Prefix
	active map[netip.Addr]int
}

func Open(ctx context.Context, db *store.DB, now func() time.Time) (*Allocator, error) {
	if now == nil {
		now = time.Now
	}
	ula, err := db.Synthetic().EnsureULA(ctx, layout.NewULA)
	if err != nil {
		return nil, err
	}
	if !layout.ValidULA(ula) {
		return nil, layout.ErrInvalidPrefix
	}
	pool, err := db.Synthetic().V4Pool(ctx)
	if err != nil {
		return nil, err
	}
	return &Allocator{db: db, now: now, ula: ula, pool: pool, active: make(map[netip.Addr]int)}, nil
}

func (a *Allocator) ULA() netip.Prefix { return a.ula }

func (a *Allocator) Pool() netip.Prefix {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pool
}

func (a *Allocator) SetPool(ctx context.Context, p netip.Prefix) error {
	p = p.Masked()
	if !p.Addr().Is4() || p.Bits() < CompatibilityRange.Bits() || !CompatibilityRange.Contains(p.Addr()) || p.Bits() > 29 {
		return ErrPoolOutside
	}
	if err := a.db.Synthetic().SetV4Pool(ctx, p); err != nil {
		return err
	}
	a.mu.Lock()
	a.pool = p
	a.mu.Unlock()
	return nil
}

type Addresses struct {
	V6 netip.Addr
	V4 netip.Addr
}

func (a *Allocator) For(ctx context.Context, network domain.NetworkID, real netip.Addr) (Addresses, error) {
	real = real.Unmap()
	idx, err := a.db.Synthetic().NetworkIndex(ctx, network, a.now())
	if err != nil {
		return Addresses{}, err
	}
	var out Addresses
	if real.Is4() {
		out.V6, err = layout.EmbedV4(a.ula, idx, real)
	} else {
		out.V6, err = a.db.Synthetic().MapV6(ctx, network, real, func(counter uint64) (netip.Addr, error) {
			return layout.AllocatedV6(a.ula, idx, counter)
		})
	}
	if err != nil {
		return Addresses{}, err
	}
	if pool := a.Pool(); pool.IsValid() {
		if out.V4, err = a.db.Synthetic().MapV4(ctx, network, real, pool, layout.FirstMappableV4(pool), a.now()); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (a *Allocator) Resolve(ctx context.Context, synthetic netip.Addr) (domain.NetworkID, netip.Addr, error) {
	if synthetic.Is6() {
		d, err := layout.Decode(a.ula, synthetic)
		if err != nil {
			return "", netip.Addr{}, err
		}
		if d.Kind == layout.KindEmbeddedV4 {
			id, err := a.networkForIndex(ctx, d.Index)
			return id, d.V4, err
		}
	}
	m, err := a.db.Synthetic().Lookup(ctx, synthetic)
	return m.NetworkID, m.Real, err
}

func (a *Allocator) networkForIndex(ctx context.Context, idx uint16) (domain.NetworkID, error) {
	return a.db.Synthetic().NetworkForIndex(ctx, idx)
}

func (a *Allocator) Acquire(synthetic netip.Addr) func() {
	a.mu.Lock()
	a.active[synthetic]++
	a.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			if a.active[synthetic]--; a.active[synthetic] <= 0 {
				delete(a.active, synthetic)
			}
		})
	}
}

func (a *Allocator) inUse(synthetic netip.Addr) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.active[synthetic] > 0
}

func (a *Allocator) Release(ctx context.Context, synthetic netip.Addr) error {
	if a.inUse(synthetic) {
		return ErrInUse
	}
	return a.db.Synthetic().ReleaseV4(ctx, synthetic, a.now())
}

func (a *Allocator) ReclaimIdle(ctx context.Context, idle time.Duration, max int) ([]netip.Addr, error) {
	candidates, err := a.db.Synthetic().V4ByLastUsed(ctx, a.now().Add(-idle))
	if err != nil {
		return nil, err
	}
	var out []netip.Addr
	for _, m := range candidates {
		if len(out) >= max {
			break
		}
		if a.inUse(m.Synthetic) {
			continue
		}
		if err := a.db.Synthetic().ReleaseV4(ctx, m.Synthetic, a.now()); err != nil {
			return out, err
		}
		out = append(out, m.Synthetic)
	}
	return out, nil
}

func (a *Allocator) EnsurePool(ctx context.Context, occupied []netip.Prefix) error {
	pool := a.Pool()
	if !pool.IsValid() {
		p, err := ChoosePool(occupied)
		if err != nil {
			return err
		}
		return a.SetPool(ctx, p)
	}
	if o, busy := overlap(pool, occupied); busy {
		a.mu.Lock()
		a.pool = netip.Prefix{}
		a.mu.Unlock()
		return fmt.Errorf("%w: stored pool %s overlaps %s; IPv4 synthetic addresses are disabled", ErrPoolOverlaps, pool, o)
	}
	return nil
}

func overlap(p netip.Prefix, occupied []netip.Prefix) (netip.Prefix, bool) {
	for _, o := range occupied {
		if o.Addr().Is4() && o.Bits() > 0 && p.Overlaps(o) {
			return o, true
		}
	}
	return netip.Prefix{}, false
}

func ChoosePool(occupied []netip.Prefix) (netip.Prefix, error) {
	step := uint32(1) << (32 - PoolBits)
	top := CompatibilityRange.Addr().As4()
	base := uint32(top[0])<<24 | uint32(top[1])<<16
	count := uint32(1) << (PoolBits - CompatibilityRange.Bits())
	for i := count; i > 0; i-- {
		v := base + (i-1)*step
		cand := netip.PrefixFrom(netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}), PoolBits)
		if _, busy := overlap(cand, occupied); !busy {
			return cand, nil
		}
	}
	return netip.Prefix{}, ErrPoolOverlaps
}
