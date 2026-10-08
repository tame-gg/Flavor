package netd

import (
	"net/netip"
	"strings"

	netdv1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/netd/v1"
	"git.lunarlabs.dev/flavor/flavor/internal/synthetic/layout"
)

const (
	MinMTU        = 1280
	MaxMTU        = 9000
	maxDomains    = 32
	FlavorDomain = "flavor.internal"
)

var (
	compatibilityRange = netip.MustParsePrefix("198.18.0.0/15")
	specialUse         = []string{"localhost", "local", "arpa", "invalid", "onion", "test"}
)

type ranges struct {
	v6  netip.Prefix
	v4  netip.Prefix
	mtu uint32
}

func (r ranges) hostV6() netip.Addr     { return layout.HostAddress(r.v6) }
func (r ranges) resolverV6() netip.Addr { return layout.ResolverAddress(r.v6) }

func (r ranges) resolvers() []netip.Addr {
	out := []netip.Addr{r.resolverV6()}
	if r.v4.IsValid() {
		out = append(out, layout.ResolverV4(r.v4))
	}
	return out
}

func prefixOf(p *netdv1.IpPrefix, size int) (netip.Prefix, bool) {
	if p == nil || len(p.Address) != size || p.Bits > uint32(size*8) {
		return netip.Prefix{}, false
	}
	a, _ := netip.AddrFromSlice(p.Address)
	pre := netip.PrefixFrom(a, int(p.Bits))
	return pre, pre.Masked() == pre
}

func validateCreate(req *netdv1.CreateSyntheticInterfaceRequest) (ranges, netdv1.ErrorCode) {
	v6, ok := prefixOf(req.GetV6InstallPrefix(), 16)
	if !ok || !layout.ValidULA(v6) {
		return ranges{}, netdv1.ErrorCode_ERROR_CODE_INVALID_V6_PREFIX
	}
	r := ranges{v6: v6, mtu: req.GetMtu()}
	if req.GetV4Pool() != nil {
		v4, ok := prefixOf(req.GetV4Pool(), 4)
		if !ok || v4.Bits() < compatibilityRange.Bits() || v4.Bits() > 29 || !compatibilityRange.Contains(v4.Addr()) {
			return ranges{}, netdv1.ErrorCode_ERROR_CODE_INVALID_V4_PREFIX
		}
		r.v4 = v4
	}
	if r.mtu < MinMTU || r.mtu > MaxMTU {
		return ranges{}, netdv1.ErrorCode_ERROR_CODE_INVALID_MTU
	}
	return r, netdv1.ErrorCode_ERROR_CODE_UNSPECIFIED
}

func overlapping(r ranges, occupied []netip.Prefix) (netip.Prefix, bool) {
	for _, o := range occupied {
		if o.Bits() == 0 {
			continue
		}
		if o.Overlaps(r.v6) || (r.v4.IsValid() && o.Overlaps(r.v4)) {
			return o, true
		}
	}
	return netip.Prefix{}, false
}

func validateDomains(in []string) ([]string, netdv1.ErrorCode) {
	if len(in) > maxDomains-1 {
		return nil, netdv1.ErrorCode_ERROR_CODE_INVALID_DOMAIN
	}
	out := []string{FlavorDomain}
	seen := map[string]bool{FlavorDomain: true}
	for _, raw := range in {
		d := strings.TrimSuffix(strings.ToLower(raw), ".")
		if d == "" || d == "~" || raw == "." || d == "~." {
			return nil, netdv1.ErrorCode_ERROR_CODE_DNS_CAPTURE_NOT_ALLOWED
		}
		if !validSuffix(d) {
			return nil, netdv1.ErrorCode_ERROR_CODE_INVALID_DOMAIN
		}
		if reserved(d) {
			return nil, netdv1.ErrorCode_ERROR_CODE_DNS_CAPTURE_NOT_ALLOWED
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out, netdv1.ErrorCode_ERROR_CODE_UNSPECIFIED
}

func validSuffix(d string) bool {
	if len(d) > 253 {
		return false
	}
	labels := strings.Split(d, ".")
	if len(labels) < 2 {
		return false
	}
	for _, l := range labels {
		if len(l) == 0 || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return false
		}
		for _, c := range l {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

func reserved(d string) bool {
	for _, s := range specialUse {
		if d == s || strings.HasSuffix(d, "."+s) {
			return true
		}
	}
	return false
}
