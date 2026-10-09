package client

import (
	"context"
	"net"
	"net/http"

	"git.lunarlabs.dev/flavor/flavor/gen/go/flavor/v1/flavorv1connect"
	"git.lunarlabs.dev/flavor/flavor/internal/ipc/transport"
)

const BaseURL = "http://flavord"

type Client struct {
	Daemon      flavorv1connect.DaemonServiceClient
	Networks    flavorv1connect.NetworkServiceClient
	Devices     flavorv1connect.DeviceServiceClient
	Diagnostics flavorv1connect.DiagnosticsServiceClient
	Events      flavorv1connect.EventServiceClient
	Inspector   flavorv1connect.InspectorServiceClient
	Conflicts   flavorv1connect.ConflictServiceClient
	Workspaces  flavorv1connect.WorkspaceServiceClient
	Preferences flavorv1connect.PreferenceServiceClient
	Forwards    flavorv1connect.ForwardServiceClient
}

func HTTPClient(socket string) *http.Client {
	var p http.Protocols
	p.SetUnencryptedHTTP2(true)
	return &http.Client{Transport: &http.Transport{
		Protocols: &p,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return transport.Dial(ctx, socket)
		},
	}}
}

func New(socket string) *Client {
	h := HTTPClient(socket)
	return &Client{
		Daemon:      flavorv1connect.NewDaemonServiceClient(h, BaseURL),
		Networks:    flavorv1connect.NewNetworkServiceClient(h, BaseURL),
		Devices:     flavorv1connect.NewDeviceServiceClient(h, BaseURL),
		Diagnostics: flavorv1connect.NewDiagnosticsServiceClient(h, BaseURL),
		Events:      flavorv1connect.NewEventServiceClient(h, BaseURL),
		Inspector:   flavorv1connect.NewInspectorServiceClient(h, BaseURL),
		Conflicts:   flavorv1connect.NewConflictServiceClient(h, BaseURL),
		Workspaces:  flavorv1connect.NewWorkspaceServiceClient(h, BaseURL),
		Preferences: flavorv1connect.NewPreferenceServiceClient(h, BaseURL),
		Forwards:    flavorv1connect.NewForwardServiceClient(h, BaseURL),
	}
}
