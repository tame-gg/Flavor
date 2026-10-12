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

func runDNS(ctx context.Context, c *client.Client, args []string, asJSON bool, out io.Writer) error {
	if len(args) > 1 {
		return fmt.Errorf("dns: expected at most one network\n\n%s", usage)
	}
	req := &v1.ListDNSRequest{}
	if len(args) == 1 {
		nets, err := c.Networks.ListNetworks(ctx, connect.NewRequest(&v1.ListNetworksRequest{}))
		if err != nil {
			return err
		}
		n, err := findNetwork(nets.Msg.Networks, args[0])
		if err != nil {
			return err
		}
		req.NetworkId = n.Id
	}
	res, err := c.DNS.ListDNS(ctx, connect.NewRequest(req))
	if err != nil {
		return err
	}
	if asJSON {
		return emit(out, res.Msg)
	}
	return printDNS(out, res.Msg)
}

func printDNS(out io.Writer, r *v1.ListDNSResponse) error {
	var rows []string
	for _, n := range r.Networks {
		name := n.Network.GetDisplayName()
		for _, rec := range n.Records {
			rows = append(rows, fmt.Sprintf("%s\t%s\trecord\t%s", name, rec.Name, strings.Join(rec.Addresses, ",")))
		}
		for _, rt := range n.Routes {
			rows = append(rows, fmt.Sprintf("%s\t*.%s\tsplit dns\t%s", name, rt.Suffix, strings.Join(rt.Resolvers, ",")))
		}
		for _, d := range n.SearchDomains {
			rows = append(rows, fmt.Sprintf("%s\t%s\tsearch domain\t-", name, d))
		}
	}
	if len(rows) == 0 {
		fmt.Fprintln(out, "your connected networks publish no DNS records, split DNS or search domains")
		return nil
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NETWORK\tNAME\tKIND\tVALUE")
	for _, row := range rows {
		fmt.Fprintln(w, row)
	}
	return w.Flush()
}
