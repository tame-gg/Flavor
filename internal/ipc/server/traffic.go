package server

import (
	"context"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/v1"
)

func (h *handlers) GetTrafficStats(ctx context.Context, _ *connect.Request[v1.GetTrafficStatsRequest]) (*connect.Response[v1.GetTrafficStatsResponse], error) {
	nets, tick, err := h.svc.GetTrafficStats(ctx)
	if err != nil {
		return nil, toConnect(err)
	}
	out := &v1.GetTrafficStatsResponse{IntervalMs: uint32(tick.Milliseconds())}
	for _, n := range nets {
		pn := &v1.NetworkTraffic{NetworkId: string(n.NetworkID)}
		for _, s := range n.Samples {
			pn.Samples = append(pn.Samples, &v1.TrafficSample{AtUnixMs: s.At.UnixMilli(), RxBytesPerSecond: s.Rx, TxBytesPerSecond: s.Tx})
		}
		out.Networks = append(out.Networks, pn)
	}
	return connect.NewResponse(out), nil
}
