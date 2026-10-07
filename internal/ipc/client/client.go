package client

import (
	"context"
	"net"
	"net/http"

	"git.lunarlabs.dev/lattice/lattice/gen/go/lattice/v1/latticev1connect"
)

const BaseURL = "http://latticed"

type Client struct {
	Daemon      latticev1connect.DaemonServiceClient
	Networks    latticev1connect.NetworkServiceClient
	Devices     latticev1connect.DeviceServiceClient
	Diagnostics latticev1connect.DiagnosticsServiceClient
	Events      latticev1connect.EventServiceClient
	Inspector   latticev1connect.InspectorServiceClient
}

func HTTPClient(socket string) *http.Client {
	var p http.Protocols
	p.SetUnencryptedHTTP2(true)
	return &http.Client{Transport: &http.Transport{
		Protocols: &p,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		},
	}}
}

func New(socket string) *Client {
	h := HTTPClient(socket)
	return &Client{
		Daemon:      latticev1connect.NewDaemonServiceClient(h, BaseURL),
		Networks:    latticev1connect.NewNetworkServiceClient(h, BaseURL),
		Devices:     latticev1connect.NewDeviceServiceClient(h, BaseURL),
		Diagnostics: latticev1connect.NewDiagnosticsServiceClient(h, BaseURL),
		Events:      latticev1connect.NewEventServiceClient(h, BaseURL),
		Inspector:   latticev1connect.NewInspectorServiceClient(h, BaseURL),
	}
}
