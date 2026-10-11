package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/v1"
	"git.lunarlabs.dev/flavor/flavor/internal/ipc/client"
)

func runExitNode(ctx context.Context, c *client.Client, args []string, asJSON bool, out io.Writer) error {
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
			return fmt.Errorf("exit-node list: expected at most one network\n\n%s", usage)
		}
		req := &v1.ListDevicesRequest{}
		if len(rest) == 1 {
			n, err := findNetwork(nets.Msg.Networks, rest[0])
			if err != nil {
				return err
			}
			req.NetworkId = n.Id
		}
		res, err := c.Devices.ListDevices(ctx, connect.NewRequest(req))
		if err != nil {
			return err
		}
		res.Msg.Devices = exitNodeOptions(res.Msg.Devices)
		if asJSON {
			return emit(out, res.Msg)
		}
		return printExitNodes(out, res.Msg.Devices, networkNames(nets.Msg.Networks))
	case "set":
		if len(rest) != 2 {
			return fmt.Errorf("exit-node set: expected a network and a device\n\n%s", usage)
		}
		n, err := findNetwork(nets.Msg.Networks, rest[0])
		if err != nil {
			return err
		}
		list, err := c.Devices.ListDevices(ctx, connect.NewRequest(&v1.ListDevicesRequest{NetworkId: n.Id}))
		if err != nil {
			return err
		}
		d, err := findExitNode(exitNodeOptions(list.Msg.Devices), rest[1], n.DisplayName)
		if err != nil {
			return err
		}
		res, err := c.ExitNodes.SetExitNode(ctx, connect.NewRequest(&v1.SetExitNodeRequest{NetworkId: n.Id, NodeId: d.Id.GetNodeId()}))
		if err != nil {
			return err
		}
		if asJSON {
			return emit(out, res.Msg)
		}
		fmt.Fprintf(out, "%s is now the exit node of %s; destinations on no tailnet leave through it\n", d.Hostname, n.DisplayName)
	case "clear":
		if len(rest) != 1 {
			return fmt.Errorf("exit-node clear: expected a network\n\n%s", usage)
		}
		n, err := findNetwork(nets.Msg.Networks, rest[0])
		if err != nil {
			return err
		}
		res, err := c.ExitNodes.ClearExitNode(ctx, connect.NewRequest(&v1.ClearExitNodeRequest{NetworkId: n.Id}))
		if err != nil {
			return err
		}
		if asJSON {
			return emit(out, res.Msg)
		}
		fmt.Fprintf(out, "%s no longer has an exit node\n", n.DisplayName)
	default:
		return fmt.Errorf("unknown exit-node command %q\n\n%s", sub, usage)
	}
	return nil
}

func networkNames(nets []*v1.Network) map[string]string {
	names := map[string]string{}
	for _, n := range nets {
		names[n.Id] = n.DisplayName
	}
	return names
}

func findNetwork(nets []*v1.Network, ref string) (*v1.Network, error) {
	var named []*v1.Network
	for _, n := range nets {
		if n.Id == ref {
			return n, nil
		}
		if n.DisplayName == ref {
			named = append(named, n)
		}
	}
	switch len(named) {
	case 0:
		return nil, fmt.Errorf("no network with id or name %q", ref)
	case 1:
		return named[0], nil
	}
	ids := make([]string, 0, len(named))
	for _, n := range named {
		ids = append(ids, n.Id)
	}
	return nil, fmt.Errorf("several networks are named %q, use an id: %s", ref, strings.Join(ids, ", "))
}

func exitNodeOptions(devices []*v1.Device) []*v1.Device {
	var out []*v1.Device
	for _, d := range devices {
		if d.ExitNodeOption {
			out = append(out, d)
		}
	}
	return out
}

func exitNodeState(d *v1.Device) string {
	switch {
	case d.ExitNode:
		return "selected"
	case d.ExitNodeOption:
		return "yes"
	}
	return ""
}

func findExitNode(options []*v1.Device, ref, network string) (*v1.Device, error) {
	var found []*v1.Device
	for _, d := range options {
		if d.Id.GetNodeId() == ref {
			return d, nil
		}
		if strings.EqualFold(d.Hostname, ref) || strings.EqualFold(strings.TrimSuffix(d.DnsName, "."), strings.TrimSuffix(ref, ".")) {
			found = append(found, d)
		}
	}
	switch len(found) {
	case 0:
		return nil, fmt.Errorf("no exit node %q on %s; see flavorctl exit-node list", ref, network)
	case 1:
		return found[0], nil
	}
	lines := make([]string, 0, len(found))
	for _, d := range found {
		lines = append(lines, fmt.Sprintf("  %s  %s  %s", d.Id.GetNodeId(), d.Hostname, strings.Join(d.Addresses, ",")))
	}
	return nil, fmt.Errorf("several exit nodes on %s match %q, use the node id:\n%s", network, ref, strings.Join(lines, "\n"))
}

func printExitNodes(out io.Writer, devices []*v1.Device, names map[string]string) error {
	if len(devices) == 0 {
		fmt.Fprintln(out, "no exit nodes on your connected networks")
		return nil
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NETWORK\tDEVICE\tADDRESSES\tONLINE\tSELECTED")
	for _, d := range devices {
		selected := ""
		if d.ExitNode {
			selected = "yes"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%s\n", names[d.Id.GetNetworkId()], d.Hostname, strings.Join(d.Addresses, ","), d.Online, selected)
	}
	return w.Flush()
}
