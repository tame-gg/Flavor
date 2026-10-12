package server

import (
	"context"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/v1"
	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/service"
)

func (h *handlers) GetRoutes(ctx context.Context, req *connect.Request[v1.GetRoutesRequest]) (*connect.Response[v1.GetRoutesResponse], error) {
	n, err := h.svc.GetRoutes(ctx, req.Msg.NetworkId)
	if err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.GetRoutesResponse{
		Routes: advertisedRoutes(n.Routes),
	}), nil
}

func (h *handlers) UpdateRoutes(ctx context.Context, req *connect.Request[v1.UpdateRoutesRequest]) (*connect.Response[v1.UpdateRoutesResponse], error) {
	n, err := h.svc.UpdateRoutes(ctx, req.Msg.NetworkId, req.Msg.Add, req.Msg.Remove)
	if err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.UpdateRoutesResponse{
		Routes: advertisedRoutes(n.Routes),
	}), nil
}

func advertisedRoutes(routes []domain.AdvertisedRoute) []*v1.AdvertisedRoute {
	out := make([]*v1.AdvertisedRoute, 0, len(routes))
	for _, r := range routes {
		out = append(out, &v1.AdvertisedRoute{Prefix: r.Prefix.String(), Approved: r.Approved})
	}
	return out
}

var subnetRouteUses = map[service.SubnetRouteState]v1.SubnetRouteUse{
	service.RouteNotUsed:       v1.SubnetRouteUse_SUBNET_ROUTE_USE_NOT_USED,
	service.RouteInstalled:     v1.SubnetRouteUse_SUBNET_ROUTE_USE_INSTALLED,
	service.RoutePending:       v1.SubnetRouteUse_SUBNET_ROUTE_USE_PENDING,
	service.RouteRefused:       v1.SubnetRouteUse_SUBNET_ROUTE_USE_REFUSED,
	service.RouteNotAdvertised: v1.SubnetRouteUse_SUBNET_ROUTE_USE_NOT_ADVERTISED,
}

func (h *handlers) ListSubnetRoutes(ctx context.Context, req *connect.Request[v1.ListSubnetRoutesRequest]) (*connect.Response[v1.ListSubnetRoutesResponse], error) {
	routes, err := h.svc.SubnetRoutes(ctx, req.Msg.NetworkId)
	if err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.ListSubnetRoutesResponse{Routes: subnetRoutes(routes)}), nil
}

func (h *handlers) UseSubnetRoutes(ctx context.Context, req *connect.Request[v1.UseSubnetRoutesRequest]) (*connect.Response[v1.UseSubnetRoutesResponse], error) {
	m := req.Msg
	routes, err := h.svc.UseSubnetRoutes(ctx, m.NetworkId, m.Add, m.Remove, m.All, m.None)
	if err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.UseSubnetRoutesResponse{Routes: subnetRoutes(routes)}), nil
}

func subnetRoutes(in []service.SubnetRoute) []*v1.SubnetRoute {
	out := make([]*v1.SubnetRoute, 0, len(in))
	for _, r := range in {
		pr := &v1.SubnetRoute{Network: networkRef(r.Network, r.State), Prefix: r.Prefix.String(), Use: subnetRouteUses[r.Use], Detail: r.Refused}
		for _, d := range r.Routers {
			pr.Routers = append(pr.Routers, device(d))
		}
		for _, n := range r.Overlaps {
			pr.Overlaps = append(pr.Overlaps, networkRef(n, ""))
		}
		out = append(out, pr)
	}
	return out
}
