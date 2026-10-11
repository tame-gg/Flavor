package server

import (
	"context"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/v1"
)

func (h *handlers) SetExitNode(ctx context.Context, req *connect.Request[v1.SetExitNodeRequest]) (*connect.Response[v1.SetExitNodeResponse], error) {
	d, err := h.svc.SetExitNode(ctx, req.Msg.NetworkId, req.Msg.NodeId)
	if err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.SetExitNodeResponse{Device: device(d)}), nil
}

func (h *handlers) ClearExitNode(ctx context.Context, req *connect.Request[v1.ClearExitNodeRequest]) (*connect.Response[v1.ClearExitNodeResponse], error) {
	if err := h.svc.ClearExitNode(ctx, req.Msg.NetworkId); err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.ClearExitNodeResponse{}), nil
}
