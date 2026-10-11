package domain

import (
	"net/netip"
	"slices"
	"sort"
	"strings"
)

type DNSRecord struct {
	Name      string
	Addresses []netip.Addr
}

func NormalizeDNSName(name string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
}

func (r DNSRecord) Target() (netip.Addr, bool) {
	for _, a := range r.Addresses {
		if a.Is4() {
			return a, true
		}
	}
	if len(r.Addresses) > 0 {
		return r.Addresses[0], true
	}
	return netip.Addr{}, false
}

func MergeDNSRecords(records []DNSRecord) []DNSRecord {
	byName := make(map[string][]netip.Addr)
	for _, r := range records {
		name := NormalizeDNSName(r.Name)
		if name == "" {
			continue
		}
		for _, a := range r.Addresses {
			a = a.Unmap()
			if a.IsValid() && !slices.Contains(byName[name], a) {
				byName[name] = append(byName[name], a)
			}
		}
	}
	out := make([]DNSRecord, 0, len(byName))
	for name, addrs := range byName {
		slices.SortFunc(addrs, func(a, b netip.Addr) int { return a.Compare(b) })
		out = append(out, DNSRecord{Name: name, Addresses: addrs})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func CloneDNSRecords(records []DNSRecord) []DNSRecord {
	if records == nil {
		return nil
	}
	out := make([]DNSRecord, len(records))
	for i, r := range records {
		out[i] = DNSRecord{Name: r.Name, Addresses: slices.Clone(r.Addresses)}
	}
	return out
}
