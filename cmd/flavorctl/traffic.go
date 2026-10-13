package main

import (
	"context"
	"fmt"
	"io"
	"slices"
	"text/tabwriter"
	"time"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/v1"
	"git.lunarlabs.dev/flavor/flavor/internal/ipc/client"
	"github.com/dustin/go-humanize"
)

func runTraffic(ctx context.Context, c *client.Client, args []string, asJSON bool, out io.Writer) error {
	if len(args) > 0 {
		return fmt.Errorf("traffic: expected no arguments\n\n%s", usage)
	}
	stats := func() (*v1.GetTrafficStatsResponse, error) {
		res, err := c.Traffic.GetTrafficStats(ctx, connect.NewRequest(&v1.GetTrafficStatsRequest{}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	r, err := stats()
	if err != nil {
		return err
	}
	if slices.ContainsFunc(r.Networks, func(n *v1.NetworkTraffic) bool { return len(n.Samples) == 0 }) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(r.IntervalMs) * time.Millisecond * 3 / 2):
		}
		if r, err = stats(); err != nil {
			return err
		}
	}
	if asJSON {
		return emit(out, r)
	}
	if len(r.Networks) == 0 {
		fmt.Fprintln(out, "no connected networks")
		return nil
	}
	nets, err := c.Networks.ListNetworks(ctx, connect.NewRequest(&v1.ListNetworksRequest{}))
	if err != nil {
		return err
	}
	names := networkNames(nets.Msg.Networks)
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NETWORK\tDOWNLOAD\tUPLOAD")
	for _, n := range r.Networks {
		down, up := "-", "-"
		if len(n.Samples) > 0 {
			s := n.Samples[len(n.Samples)-1]
			down, up = humanize.Bytes(s.RxBytesPerSecond)+"/s", humanize.Bytes(s.TxBytesPerSecond)+"/s"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", names[n.NetworkId], down, up)
	}
	return w.Flush()
}
