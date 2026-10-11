package server

import (
	"context"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/v1"
	"git.lunarlabs.dev/flavor/flavor/internal/domain"
)

func (h *handlers) GetRoutes(ctx context.Context, req *connect.Request[v1.GetRoutesRequest]) (*connect.Response[v1.GetRoutesResponse], error) {
	n, err := h.svc.GetRoutes(ctx, req.Msg.NetworkId)
	if err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.GetRoutesResponse{
		Routes:           advertisedRoutes(n.Routes),
		ExitNodeOffered:  n.ExitNode.Offered,
		ExitNodeApproved: n.ExitNode.Approved,
	}), nil
}

func (h *handlers) UpdateRoutes(ctx context.Context, req *connect.Request[v1.UpdateRoutesRequest]) (*connect.Response[v1.UpdateRoutesResponse], error) {
	n, err := h.svc.UpdateRoutes(ctx, req.Msg.NetworkId, req.Msg.Add, req.Msg.Remove, req.Msg.ExitNode)
	if err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.UpdateRoutesResponse{
		Routes:           advertisedRoutes(n.Routes),
		ExitNodeOffered:  n.ExitNode.Offered,
		ExitNodeApproved: n.ExitNode.Approved,
	}), nil
}

func advertisedRoutes(routes []domain.AdvertisedRoute) []*v1.AdvertisedRoute {
	out := make([]*v1.AdvertisedRoute, 0, len(routes))
	for _, r := range routes {
		out = append(out, &v1.AdvertisedRoute{Prefix: r.Prefix.String(), Approved: r.Approved})
	}
	return out
}
