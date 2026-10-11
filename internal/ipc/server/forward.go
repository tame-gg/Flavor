package server

import (
	"context"
	"errors"
	"sync"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/v1"
	"git.lunarlabs.dev/flavor/flavor/internal/service"
)

func (h *handlers) forwardRoute(r service.Route) *v1.ForwardRoute {
	out := &v1.ForwardRoute{Decision: inspection(r.Result, 0, h.svc.InstanceID())}
	if address := r.DialAddress(); address != "" {
		out.Network = networkRef(r.Network, "")
		out.Target = address
	}
	return out
}

func errorDetail(err error) *v1.FlavorErrorDetail {
	var se *service.Error
	if !errors.As(err, &se) {
		se = &service.Error{Code: service.CodeInternal, SafeMessage: "internal error", Retryable: true}
	}
	return &v1.FlavorErrorDetail{
		Code:        v1.FlavorErrorCode(v1.FlavorErrorCode_value["FLAVOR_ERROR_CODE_"+string(se.Code)]),
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
			c := h.connection(ev)
			if ev.Kind == service.ForwardOpened {
				out.Event = &v1.ForwardResponse_Opened{Opened: c}
			} else {
				out.Event = &v1.ForwardResponse_Closed{Closed: c}
			}
		case service.ForwardRefused:
			out.Event = &v1.ForwardResponse_Refused{Refused: refused(ev)}
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

func (h *handlers) connection(ev service.ForwardEvent) *v1.ForwardConnection {
	return &v1.ForwardConnection{
		Id:            ev.ConnID,
		Client:        ev.Client,
		Route:         h.forwardRoute(ev.Route),
		BytesSent:     ev.BytesSent,
		BytesReceived: ev.BytesReceived,
		Destination:   ev.Destination,
	}
}

func refused(ev service.ForwardEvent) *v1.ForwardRefused {
	return &v1.ForwardRefused{Id: ev.ConnID, Client: ev.Client, Reason: errorDetail(ev.Err), Destination: ev.Destination}
}

func (h *handlers) Proxy(ctx context.Context, req *connect.Request[v1.ProxyRequest], stream *connect.ServerStream[v1.ProxyResponse]) error {
	var mu sync.Mutex
	emit := func(ev service.ForwardEvent) {
		out := &v1.ProxyResponse{}
		switch ev.Kind {
		case service.ForwardStarted:
			out.Event = &v1.ProxyResponse_Started{Started: &v1.ProxyStarted{ListenAddress: ev.Listen.String()}}
		case service.ForwardOpened:
			out.Event = &v1.ProxyResponse_Opened{Opened: h.connection(ev)}
		case service.ForwardClosed:
			out.Event = &v1.ProxyResponse_Closed{Closed: h.connection(ev)}
		case service.ForwardRefused:
			out.Event = &v1.ProxyResponse_Refused{Refused: refused(ev)}
		}
		mu.Lock()
		defer mu.Unlock()
		_ = stream.Send(out)
	}
	if err := h.svc.Proxy(ctx, service.ProxyRequest{Listen: req.Msg.Listen}, emit); err != nil {
		return toConnect(err)
	}
	return nil
}
