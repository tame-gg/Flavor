package service

import (
	"net/netip"
	"slices"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func TestAnswersFollowTheCNAMEChainOnly(t *testing.T) {
	name := dnsmessage.MustNewName("wiki.corp.example.")
	edge := dnsmessage.MustNewName("Edge.Corp.Example.")
	other := dnsmessage.MustNewName("evil.example.")
	rr := func(n dnsmessage.Name, body dnsmessage.ResourceBody) dnsmessage.Resource {
		return dnsmessage.Resource{Header: dnsmessage.ResourceHeader{Name: n, Class: dnsmessage.ClassINET}, Body: body}
	}
	got := answers([]dnsmessage.Resource{
		rr(other, &dnsmessage.AResource{A: [4]byte{203, 0, 113, 66}}),
		rr(name, &dnsmessage.CNAMEResource{CNAME: dnsmessage.MustNewName("edge.corp.example.")}),
		rr(edge, &dnsmessage.AResource{A: [4]byte{10, 1, 2, 3}}),
	}, name)
	if !slices.Equal(got, []netip.Addr{netip.MustParseAddr("10.1.2.3")}) {
		t.Fatalf("%v", got)
	}
}
