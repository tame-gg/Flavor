package server

import (
	"context"
	"errors"
	"sync"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/lattice/lattice/gen/go/lattice/v1"
	"git.lunarlabs.dev/lattice/lattice/internal/service"
)

func (h *handlers) forwardRoute(r service.Route) *v1.ForwardRoute {
	out := &v1.ForwardRoute{Decision: inspection(r.Result, 0, h.svc.InstanceID())}
	if r.Target.IsValid() {
		out.Network = networkRef(r.Network, "")
		out.Target = r.Target.String()
	}
	return out
}

func errorDetail(err error) *v1.LatticeErrorDetail {
	var se *service.Error
	if !errors.As(err, &se) {
		se = &service.Error{Code: service.CodeInternal, SafeMessage: "internal error", Retryable: true}
	}
	return &v1.LatticeErrorDetail{
		Code:        v1.LatticeErrorCode(v1.LatticeErrorCode_value["LATTICE_ERROR_CODE_"+string(se.Code)]),
		SafeMessage: se.SafeMessage,
		Retryable:   se.Retryable,
	}
}

func (h *handlers) Forward(ctx context.Context, req *connect.Request[v1.ForwardRequest], stream *connect.ServerStream[v1.ForwardResponse]) error {
	var mu sync.Mutex
	emit := func(ev service.ForwardEvent) {
		out := &v1.ForwardResponse{}
		switch ev.Kind {
		case service.ForwardStarted:
			out.Event = &v1.ForwardResponse_Started{Started: &v1.ForwardStarted{ListenAddress: ev.Listen.String(), Route: h.forwardRoute(ev.Route)}}
		case service.ForwardOpened, service.ForwardClosed:
			c := &v1.ForwardConnection{Id: ev.ConnID, Client: ev.Client, Route: h.forwardRoute(ev.Route), BytesSent: ev.BytesSent, BytesReceived: ev.BytesReceived}
			if ev.Kind == service.ForwardOpened {
				out.Event = &v1.ForwardResponse_Opened{Opened: c}
			} else {
				out.Event = &v1.ForwardResponse_Closed{Closed: c}
			}
		case service.ForwardRefused:
			out.Event = &v1.ForwardResponse_Refused{Refused: &v1.ForwardRefused{Id: ev.ConnID, Client: ev.Client, Reason: errorDetail(ev.Err)}}
		}
		mu.Lock()
		defer mu.Unlock()
		_ = stream.Send(out)
	}
	if err := h.svc.Forward(ctx, service.ForwardRequest{Destination: req.Msg.Destination, Listen: req.Msg.Listen, Network: req.Msg.Network}, emit); err != nil {
		return toConnect(err)
	}
	return nil
}
