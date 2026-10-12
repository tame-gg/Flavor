package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"strings"
	"text/tabwriter"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/v1"
	"git.lunarlabs.dev/flavor/flavor/internal/ipc/client"
	"google.golang.org/protobuf/encoding/protojson"
)

type routeRow struct {
	network  string
	route    string
	approved bool
}

func runRoutes(ctx context.Context, c *client.Client, args []string, asJSON bool, out io.Writer) error {
	if len(args) == 0 {
		args = []string{"list"}
	}
	nets, err := c.Networks.ListNetworks(ctx, connect.NewRequest(&v1.ListNetworksRequest{}))
	if err != nil {
		return err
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list", "ls":
		if len(rest) > 1 {
			return fmt.Errorf("routes list: expected at most one network\n\n%s", usage)
		}
		selected := connectedNetworks(nets.Msg.Networks)
		if len(rest) == 1 {
			n, err := findNetwork(nets.Msg.Networks, rest[0])
			if err != nil {
				return err
			}
			selected = []*v1.Network{n}
		}
		var rows []routeRow
		var all []networkRoutes
		for _, n := range selected {
			res, err := c.Routes.GetRoutes(ctx, connect.NewRequest(&v1.GetRoutesRequest{NetworkId: n.Id}))
			if err != nil {
				return err
			}
			rows = append(rows, routeRows(n.DisplayName, res.Msg.Routes)...)
			all = append(all, networkRoutes{NetworkID: n.Id, Response: res.Msg})
		}
		if asJSON {
			return emitRoutes(out, all)
		}
		return printRoutes(out, rows)
	case "advertise", "withdraw":
		if len(rest) < 2 {
			return fmt.Errorf("routes %s: expected a network and at least one CIDR\n\n%s", sub, usage)
		}
		n, err := findNetwork(nets.Msg.Networks, rest[0])
		if err != nil {
			return err
		}
		req := &v1.UpdateRoutesRequest{NetworkId: n.Id}
		if sub == "advertise" {
			req.Add = rest[1:]
		} else {
			req.Remove = rest[1:]
		}
		res, err := c.Routes.UpdateRoutes(ctx, connect.NewRequest(req))
		if err != nil {
			return err
		}
		if asJSON {
			return emit(out, res.Msg)
		}
		for _, cidr := range rest[1:] {
			if sub == "advertise" {
				fmt.Fprintln(out, advertiseSummary(cidr, n.DisplayName, res.Msg.Routes))
			} else {
				fmt.Fprintf(out, "no longer advertising %s on %s\n", cidr, n.DisplayName)
			}
		}
	case "available":
		if len(rest) > 1 {
			return fmt.Errorf("routes available: expected at most one network\n\n%s", usage)
		}
		req := &v1.ListSubnetRoutesRequest{}
		if len(rest) == 1 {
			n, err := findNetwork(nets.Msg.Networks, rest[0])
			if err != nil {
				return err
			}
			req.NetworkId = n.Id
		}
		res, err := c.Routes.ListSubnetRoutes(ctx, connect.NewRequest(req))
		if err != nil {
			return err
		}
		if asJSON {
			return emit(out, res.Msg)
		}
		return printSubnetRoutes(out, res.Msg.Routes)
	case "use", "unuse":
		if len(rest) < 2 {
			return fmt.Errorf("routes %s: expected a network and CIDRs or --all\n\n%s", sub, usage)
		}
		n, err := findNetwork(nets.Msg.Networks, rest[0])
		if err != nil {
			return err
		}
		req := &v1.UseSubnetRoutesRequest{NetworkId: n.Id}
		for _, a := range rest[1:] {
			switch {
			case a == "--all" && sub == "use":
				req.All = true
			case a == "--all":
				req.None = true
			case sub == "use":
				req.Add = append(req.Add, a)
			default:
				req.Remove = append(req.Remove, a)
			}
		}
		res, err := c.Routes.UseSubnetRoutes(ctx, connect.NewRequest(req))
		if err != nil {
			return err
		}
		if asJSON {
			return emit(out, res.Msg)
		}
		return printSubnetRoutes(out, res.Msg.Routes)
	default:
		return fmt.Errorf("unknown routes command %q\n\n%s", sub, usage)
	}
	return nil
}

func printSubnetRoutes(out io.Writer, routes []*v1.SubnetRoute) error {
	if len(routes) == 0 {
		fmt.Fprintln(out, "no subnet routes are advertised on your connected networks")
		return nil
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NETWORK\tROUTE\tVIA\tON THIS COMPUTER")
	for _, r := range routes {
		var via []string
		for _, d := range r.Routers {
			name := d.Hostname
			if !d.Online {
				name += " (offline)"
			}
			via = append(via, name)
		}
		use := subnetRouteUse(r)
		for _, o := range r.Overlaps {
			use += "; overlaps a route on " + o.DisplayName
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Network.GetDisplayName(), r.Prefix, cmp.Or(strings.Join(via, ", "), "-"), use)
	}
	return w.Flush()
}

func subnetRouteUse(r *v1.SubnetRoute) string {
	switch r.Use {
	case v1.SubnetRouteUse_SUBNET_ROUTE_USE_INSTALLED:
		return "in use"
	case v1.SubnetRouteUse_SUBNET_ROUTE_USE_PENDING:
		if r.Detail != "" {
			return "waiting: " + r.Detail
		}
		return "waiting"
	case v1.SubnetRouteUse_SUBNET_ROUTE_USE_REFUSED:
		return "refused: " + r.Detail
	case v1.SubnetRouteUse_SUBNET_ROUTE_USE_NOT_ADVERTISED:
		return "chosen, but no device advertises it now"
	}
	return "not used"
}

type networkRoutes struct {
	NetworkID string
	Response  *v1.GetRoutesResponse
}

func emitRoutes(out io.Writer, all []networkRoutes) error {
	type entry struct {
		NetworkID string          `json:"networkId"`
		Routes    json.RawMessage `json:"routes"`
	}
	entries := make([]entry, 0, len(all))
	for _, n := range all {
		b, err := protojson.Marshal(n.Response)
		if err != nil {
			return err
		}
		entries = append(entries, entry{NetworkID: n.NetworkID, Routes: b})
	}
	b, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(b))
	return err
}

func connectedNetworks(nets []*v1.Network) []*v1.Network {
	var out []*v1.Network
	for _, n := range nets {
		if n.State == v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_CONNECTED || n.State == v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_DEGRADED {
			out = append(out, n)
		}
	}
	return out
}

func approvalText(approved bool) string {
	if approved {
		return "approved"
	}
	return "not approved yet"
}

func routeRows(network string, routes []*v1.AdvertisedRoute) []routeRow {
	var rows []routeRow
	for _, r := range routes {
		rows = append(rows, routeRow{network, r.Prefix, r.Approved})
	}
	return rows
}

func printRoutes(out io.Writer, rows []routeRow) error {
	if len(rows) == 0 {
		fmt.Fprintln(out, "no routes advertised on your connected networks")
		return nil
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NETWORK\tROUTE\tSTATE")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.network, r.route, approvalText(r.approved))
	}
	return w.Flush()
}

func advertiseSummary(cidr, network string, routes []*v1.AdvertisedRoute) string {
	key := cidr
	if p, err := netip.ParsePrefix(cidr); err == nil {
		key = p.Masked().String()
	}
	for _, r := range routes {
		if r.Prefix != key {
			continue
		}
		if r.Approved {
			return fmt.Sprintf("advertising %s on %s; it is already approved", cidr, network)
		}
		return fmt.Sprintf("advertising %s on %s; approve it on the control server", cidr, network)
	}
	return fmt.Sprintf("advertising %s on %s", cidr, network)
}
