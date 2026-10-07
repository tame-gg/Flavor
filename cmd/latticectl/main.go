package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/lattice/lattice/gen/go/lattice/v1"
	"git.lunarlabs.dev/lattice/lattice/internal/config"
	"git.lunarlabs.dev/lattice/lattice/internal/ipc/client"
)

const usage = `usage: latticectl [--runtime-dir DIR] <command> [args]

commands:
  info                                 daemon version and protocol
  list                                 configured networks and their state
  devices [network-id]                 devices, optionally for one network
  add --name NAME [--headscale URL] [--auto-connect]
  connect <network-id>
  disconnect <network-id>
  enroll <network-id>                  reads a pre-auth key from stdin
  rename <network-id> <name>
  remove [--delete-identity] <network-id>
  diag                                 safe diagnostics summary
`

func main() {
	runtimeDir := flag.String("runtime-dir", "", "runtime directory parent (defaults to $XDG_RUNTIME_DIR)")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	paths, err := config.ResolveFromOS(*runtimeDir)
	if err != nil {
		fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := run(ctx, client.New(paths.Socket), flag.Arg(0), flag.Args()[1:], os.Stdin, os.Stdout); err != nil {
		fail(err)
	}
}

func fail(err error) {
	var ce *connect.Error
	if errors.As(err, &ce) {
		for _, d := range ce.Details() {
			if m, derr := d.Value(); derr == nil {
				if detail, ok := m.(*v1.LatticeErrorDetail); ok {
					fmt.Fprintf(os.Stderr, "latticectl: %s (%s)\n", detail.SafeMessage, strings.TrimPrefix(detail.Code.String(), "LATTICE_ERROR_CODE_"))
					os.Exit(1)
				}
			}
		}
		if ce.Code() == connect.CodeUnavailable {
			fmt.Fprintln(os.Stderr, "latticectl: the Lattice daemon is not running")
			os.Exit(1)
		}
	}
	fmt.Fprintf(os.Stderr, "latticectl: %v\n", err)
	os.Exit(1)
}

func run(ctx context.Context, c *client.Client, cmd string, args []string, stdin io.Reader, out io.Writer) error {
	need := func(n int) error {
		if len(args) != n {
			return fmt.Errorf("%s: expected %d argument(s)\n\n%s", cmd, n, usage)
		}
		return nil
	}
	switch cmd {
	case "info":
		res, err := c.Daemon.GetDaemonInfo(ctx, connect.NewRequest(&v1.GetDaemonInfoRequest{}))
		if err != nil {
			return err
		}
		m := res.Msg
		fmt.Fprintf(out, "latticed %s (commit %s)\nprotocol %d.%d\ninstance %s\n", m.DaemonVersion, m.BuildCommit, m.ProtocolMajor, m.ProtocolMinor, m.DaemonInstanceId)
	case "list":
		res, err := c.Networks.ListNetworks(ctx, connect.NewRequest(&v1.ListNetworksRequest{}))
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tPROVIDER\tSTATE\tAUTO")
		var prompts []string
		for _, n := range res.Msg.Networks {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%t\n", n.Id, n.DisplayName, enumName(n.Provider.String(), "PROVIDER_TYPE_"), enumName(n.State.String(), "NETWORK_CONNECTION_STATE_"), n.AutoConnect)
			if a := n.Authentication; a != nil && a.AuthUrl != "" {
				prompts = append(prompts, fmt.Sprintf("sign in to %s: %s", n.DisplayName, a.AuthUrl))
			}
		}
		if err := w.Flush(); err != nil {
			return err
		}
		for _, p := range prompts {
			fmt.Fprintln(out, p)
		}
	case "devices":
		req := &v1.ListDevicesRequest{}
		if len(args) > 0 {
			req.NetworkId = args[0]
		}
		res, err := c.Devices.ListDevices(ctx, connect.NewRequest(req))
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "NETWORK\tNODE\tHOSTNAME\tADDRESSES\tONLINE")
		for _, d := range res.Msg.Devices {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%t\n", d.Id.GetNetworkId(), d.Id.GetNodeId(), d.Hostname, strings.Join(d.Addresses, ","), d.Online)
		}
		return w.Flush()
	case "add":
		fs := flag.NewFlagSet("add", flag.ContinueOnError)
		name := fs.String("name", "", "display name")
		headscale := fs.String("headscale", "", "Headscale control server URL")
		auto := fs.Bool("auto-connect", false, "connect automatically when latticed starts")
		if err := fs.Parse(args); err != nil {
			return err
		}
		req := &v1.AddNetworkRequest{DisplayName: *name, Provider: v1.ProviderType_PROVIDER_TYPE_TAILSCALE, AutoConnect: *auto}
		if *headscale != "" {
			req.Provider, req.ControlUrl = v1.ProviderType_PROVIDER_TYPE_HEADSCALE, *headscale
		}
		res, err := c.Networks.AddNetwork(ctx, connect.NewRequest(req))
		if err != nil {
			return err
		}
		fmt.Fprintln(out, res.Msg.Network.Id)
	case "connect":
		if err := need(1); err != nil {
			return err
		}
		_, err := c.Networks.ConnectNetwork(ctx, connect.NewRequest(&v1.ConnectNetworkRequest{NetworkId: args[0]}))
		return err
	case "disconnect":
		if err := need(1); err != nil {
			return err
		}
		_, err := c.Networks.DisconnectNetwork(ctx, connect.NewRequest(&v1.DisconnectNetworkRequest{NetworkId: args[0]}))
		return err
	case "enroll":
		if err := need(1); err != nil {
			return err
		}
		key, err := bufio.NewReader(io.LimitReader(stdin, 1024)).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		_, err = c.Networks.EnrollNetwork(ctx, connect.NewRequest(&v1.EnrollNetworkRequest{
			NetworkId:  args[0],
			Enrollment: &v1.EnrollmentCredential{Credential: &v1.EnrollmentCredential_PreAuthKey{PreAuthKey: strings.TrimSpace(key)}},
		}))
		return err
	case "rename":
		if err := need(2); err != nil {
			return err
		}
		_, err := c.Networks.UpdateNetwork(ctx, connect.NewRequest(&v1.UpdateNetworkRequest{NetworkId: args[0], DisplayName: &args[1]}))
		return err
	case "remove":
		fs := flag.NewFlagSet("remove", flag.ContinueOnError)
		hard := fs.Bool("delete-identity", false, "also delete the local device identity")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return fmt.Errorf("remove: expected a network id\n\n%s", usage)
		}
		if *hard {
			_, err := c.Networks.DeleteNetworkIdentity(ctx, connect.NewRequest(&v1.DeleteNetworkIdentityRequest{NetworkId: fs.Arg(0)}))
			return err
		}
		_, err := c.Networks.RemoveNetwork(ctx, connect.NewRequest(&v1.RemoveNetworkRequest{NetworkId: fs.Arg(0)}))
		return err
	case "diag":
		res, err := c.Diagnostics.RunDiagnostics(ctx, connect.NewRequest(&v1.RunDiagnosticsRequest{}))
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		for _, ch := range res.Msg.Checks {
			fmt.Fprintf(w, "%s\t%s\t%s\n", ch.Status, ch.Name, ch.SafeDetail)
		}
		return w.Flush()
	default:
		return fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
	}
	return nil
}

func enumName(s, prefix string) string {
	return strings.ToLower(strings.TrimPrefix(s, prefix))
}
