package domain

import "net/netip"

type AdvertisedRoute struct {
	Prefix   netip.Prefix
	Approved bool
}

var TailnetPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("fd7a:115c:a1e0::/48"),
}
