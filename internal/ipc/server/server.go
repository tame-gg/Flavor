package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/lattice/lattice/gen/go/lattice/v1"
	"git.lunarlabs.dev/lattice/lattice/gen/go/lattice/v1/latticev1connect"
	"git.lunarlabs.dev/lattice/lattice/internal/events"
	"git.lunarlabs.dev/lattice/lattice/internal/ipc/transport"
	"git.lunarlabs.dev/lattice/lattice/internal/secret"
	"git.lunarlabs.dev/lattice/lattice/internal/service"
)

const maxMessageBytes = 1 << 20

type handlers struct {
	svc *service.Service
}

func New(svc *service.Service) *http.Server {
	h := &handlers{svc: svc}
	opts := []connect.HandlerOption{connect.WithReadMaxBytes(maxMessageBytes)}
	mux := http.NewServeMux()
	mux.Handle(latticev1connect.NewDaemonServiceHandler(h, opts...))
	mux.Handle(latticev1connect.NewNetworkServiceHandler(h, opts...))
	mux.Handle(latticev1connect.NewDeviceServiceHandler(h, opts...))
	mux.Handle(latticev1connect.NewDiagnosticsServiceHandler(h, opts...))
	mux.Handle(latticev1connect.NewEventServiceHandler(h, opts...))
	var p http.Protocols
	p.SetUnencryptedHTTP2(true)
	return &http.Server{
		Handler:           transport.RequirePeer(mux),
		Protocols:         &p,
		ConnContext:       transport.ConnContext,
		ReadHeaderTimeout: 10 * time.Second,
	}
}

func (h *handlers) GetDaemonInfo(context.Context, *connect.Request[v1.GetDaemonInfoRequest]) (*connect.Response[v1.GetDaemonInfoResponse], error) {
	return connect.NewResponse(daemonInfo(h.svc.Info())), nil
}

func (h *handlers) GetStateSnapshot(ctx context.Context, _ *connect.Request[v1.GetStateSnapshotRequest]) (*connect.Response[v1.GetStateSnapshotResponse], error) {
	snap, err := h.svc.Snapshot(ctx)
	if err != nil {
		return nil, toConnect(err)
	}
	out := &v1.GetStateSnapshotResponse{
		DaemonInstanceId: snap.Info.InstanceID,
		SnapshotSequence: snap.Sequence,
		Daemon:           daemonInfo(snap.Info),
		CapturedAt:       timestamp(snap.CapturedAt),
	}
	for _, n := range snap.Networks {
		out.Networks = append(out.Networks, network(n))
	}
	for _, d := range snap.Devices {
		out.Devices = append(out.Devices, device(d))
	}
	return connect.NewResponse(out), nil
}

func (h *handlers) ListNetworks(ctx context.Context, _ *connect.Request[v1.ListNetworksRequest]) (*connect.Response[v1.ListNetworksResponse], error) {
	views, seq, err := h.svc.ListNetworks(ctx)
	if err != nil {
		return nil, toConnect(err)
	}
	out := &v1.ListNetworksResponse{DaemonInstanceId: h.svc.InstanceID(), SnapshotSequence: seq}
	for _, n := range views {
		out.Networks = append(out.Networks, network(n))
	}
	return connect.NewResponse(out), nil
}

func (h *handlers) GetNetwork(ctx context.Context, req *connect.Request[v1.GetNetworkRequest]) (*connect.Response[v1.GetNetworkResponse], error) {
	v, seq, err := h.svc.GetNetwork(ctx, req.Msg.NetworkId)
	if err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.GetNetworkResponse{
		DaemonInstanceId: h.svc.InstanceID(),
		SnapshotSequence: seq,
		Network:          network(v),
	}), nil
}

func (h *handlers) AddNetwork(ctx context.Context, req *connect.Request[v1.AddNetworkRequest]) (*connect.Response[v1.AddNetworkResponse], error) {
	p, ok := providerFromProto(req.Msg.Provider)
	if !ok {
		return nil, toConnect(&service.Error{Code: service.CodeInvalidArgument, SafeMessage: "unsupported provider"})
	}
	v, err := h.svc.AddNetwork(ctx, service.AddInput{
		DisplayName: req.Msg.DisplayName,
		Provider:    p,
		ControlURL:  req.Msg.ControlUrl,
		AutoConnect: req.Msg.AutoConnect,
	})
	if err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.AddNetworkResponse{Network: network(v)}), nil
}

func (h *handlers) UpdateNetwork(ctx context.Context, req *connect.Request[v1.UpdateNetworkRequest]) (*connect.Response[v1.UpdateNetworkResponse], error) {
	v, err := h.svc.UpdateNetwork(ctx, req.Msg.NetworkId, req.Msg.DisplayName, req.Msg.AutoConnect)
	if err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.UpdateNetworkResponse{Network: network(v)}), nil
}

func (h *handlers) RemoveNetwork(ctx context.Context, req *connect.Request[v1.RemoveNetworkRequest]) (*connect.Response[v1.RemoveNetworkResponse], error) {
	if err := h.svc.RemoveNetwork(ctx, req.Msg.NetworkId); err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.RemoveNetworkResponse{}), nil
}

func (h *handlers) DeleteNetworkIdentity(ctx context.Context, req *connect.Request[v1.DeleteNetworkIdentityRequest]) (*connect.Response[v1.DeleteNetworkIdentityResponse], error) {
	if err := h.svc.DeleteNetworkIdentity(ctx, req.Msg.NetworkId); err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.DeleteNetworkIdentityResponse{}), nil
}

func (h *handlers) ConnectNetwork(ctx context.Context, req *connect.Request[v1.ConnectNetworkRequest]) (*connect.Response[v1.ConnectNetworkResponse], error) {
	if err := h.svc.ConnectNetwork(ctx, req.Msg.NetworkId); err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.ConnectNetworkResponse{}), nil
}

func (h *handlers) DisconnectNetwork(ctx context.Context, req *connect.Request[v1.DisconnectNetworkRequest]) (*connect.Response[v1.DisconnectNetworkResponse], error) {
	if err := h.svc.DisconnectNetwork(ctx, req.Msg.NetworkId); err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.DisconnectNetworkResponse{}), nil
}

func (h *handlers) EnrollNetwork(ctx context.Context, req *connect.Request[v1.EnrollNetworkRequest]) (*connect.Response[v1.EnrollNetworkResponse], error) {
	key := secret.New(req.Msg.GetEnrollment().GetPreAuthKey())
	if err := h.svc.EnrollNetwork(ctx, req.Msg.NetworkId, key); err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.EnrollNetworkResponse{}), nil
}

func (h *handlers) ListDevices(ctx context.Context, req *connect.Request[v1.ListDevicesRequest]) (*connect.Response[v1.ListDevicesResponse], error) {
	devs, seq, err := h.svc.ListDevices(ctx, req.Msg.NetworkId)
	if err != nil {
		return nil, toConnect(err)
	}
	out := &v1.ListDevicesResponse{DaemonInstanceId: h.svc.InstanceID(), SnapshotSequence: seq}
	for _, d := range devs {
		out.Devices = append(out.Devices, device(d))
	}
	return connect.NewResponse(out), nil
}

func (h *handlers) RunDiagnostics(ctx context.Context, _ *connect.Request[v1.RunDiagnosticsRequest]) (*connect.Response[v1.RunDiagnosticsResponse], error) {
	d := h.svc.Diagnostics(ctx)
	out := &v1.RunDiagnosticsResponse{SecretStoreBackend: d.SecretBackend, SecretStoreState: d.SecretState}
	for _, c := range d.Checks {
		out.Checks = append(out.Checks, &v1.DiagnosticCheck{Name: c.Name, Status: c.Status, SafeDetail: c.Detail})
	}
	return connect.NewResponse(out), nil
}

func (h *handlers) WatchEvents(ctx context.Context, req *connect.Request[v1.WatchEventsRequest], stream *connect.ServerStream[v1.DaemonEvent]) error {
	instance := h.svc.InstanceID()
	if id := req.Msg.DaemonInstanceId; id != "" && id != instance {
		return resyncError("daemon instance changed")
	}
	sub, err := h.svc.Bus().Subscribe(ctx, req.Msg.AfterSequence)
	switch {
	case errors.Is(err, events.ErrResyncRequired):
		return resyncError("requested sequence is no longer available")
	case errors.Is(err, events.ErrClosed):
		return toConnect(&service.Error{Code: service.CodeShuttingDown, SafeMessage: "daemon is shutting down", Retryable: true})
	case err != nil:
		return toConnect(err)
	}
	defer sub.Close()
	if err := stream.Send(nil); err != nil {
		return err
	}
	for ev := range sub.Events {
		if err := stream.Send(event(ev, instance)); err != nil {
			return err
		}
	}
	switch err := sub.Err(); {
	case errors.Is(err, events.ErrResyncRequired):
		return resyncError("event stream fell behind")
	case errors.Is(err, events.ErrClosed):
		return toConnect(&service.Error{Code: service.CodeShuttingDown, SafeMessage: "daemon is shutting down", Retryable: true})
	default:
		return ctx.Err()
	}
}

func resyncError(msg string) error {
	ce := connect.NewError(connect.CodeFailedPrecondition, errors.New(msg))
	if d, err := connect.NewErrorDetail(&v1.LatticeErrorDetail{
		Code:        v1.LatticeErrorCode_LATTICE_ERROR_CODE_RESYNC_REQUIRED,
		SafeMessage: msg,
		Retryable:   true,
	}); err == nil {
		ce.AddDetail(d)
	}
	return ce
}

var connectCodes = map[service.Code]connect.Code{
	service.CodeNetworkNotFound:   connect.CodeNotFound,
	service.CodeInvalidControlURL: connect.CodeInvalidArgument,
	service.CodeInvalidArgument:   connect.CodeInvalidArgument,
	service.CodeAlreadyConnected:  connect.CodeAlreadyExists,
	service.CodeBusy:              connect.CodeAborted,
	service.CodeStateDirectory:    connect.CodeFailedPrecondition,
	service.CodeShuttingDown:      connect.CodeUnavailable,
	service.CodeInternal:          connect.CodeInternal,
}

func toConnect(err error) error {
	var se *service.Error
	if !errors.As(err, &se) {
		se = &service.Error{Code: service.CodeInternal, SafeMessage: "internal error", Retryable: true}
	}
	code, ok := connectCodes[se.Code]
	if !ok {
		code = connect.CodeInternal
	}
	ce := connect.NewError(code, errors.New(se.SafeMessage))
	if d, derr := connect.NewErrorDetail(&v1.LatticeErrorDetail{
		Code:        v1.LatticeErrorCode(v1.LatticeErrorCode_value["LATTICE_ERROR_CODE_"+string(se.Code)]),
		SafeMessage: se.SafeMessage,
		Retryable:   se.Retryable,
	}); derr == nil {
		ce.AddDetail(d)
	}
	return ce
}
