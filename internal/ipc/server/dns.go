package server

import (
	"context"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/v1"
)

func (h *handlers) ListDNS(ctx context.Context, req *connect.Request[v1.ListDNSRequest]) (*connect.Response[v1.ListDNSResponse], error) {
	nets, err := h.svc.ListDNS(ctx, req.Msg.NetworkId)
	if err != nil {
		return nil, toConnect(err)
	}
	out := &v1.ListDNSResponse{}
	for _, n := range nets {
		pn := &v1.NetworkDNS{Network: networkRef(n.Network, n.State), SearchDomains: n.DNS.SearchDomains}
		for _, r := range n.DNS.Records {
			pr := &v1.DNSRecord{Name: r.Name}
			for _, a := range r.Addresses {
				pr.Addresses = append(pr.Addresses, a.String())
			}
			pn.Records = append(pn.Records, pr)
		}
		for _, r := range n.DNS.Routes {
			pr := &v1.DNSRoute{Suffix: r.Suffix}
			for _, a := range r.Resolvers {
				pr.Resolvers = append(pr.Resolvers, a.String())
			}
			pn.Routes = append(pn.Routes, pr)
		}
		out.Networks = append(out.Networks, pn)
	}
	return connect.NewResponse(out), nil
}
