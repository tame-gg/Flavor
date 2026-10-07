package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/lattice/lattice/gen/go/lattice/v1"
	"git.lunarlabs.dev/lattice/lattice/internal/config"
	"git.lunarlabs.dev/lattice/lattice/internal/ipc/client"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const usage = `usage: latticectl [--runtime-dir DIR] [--json] <command> [args]

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
  explain <destination>                which network an address or name belongs to, and why
  forward [--network N] [--listen ADDR] <destination:port>
                                       listen on loopback and forward through the chosen network
  socks [--listen ADDR]                local SOCKS5 proxy for Lattice destinations (default 127.0.0.1:1080)
  conflicts                            addresses and names that exist more than once
  workspace list
  workspace create --name NAME [--description TEXT] [network-id...]
  workspace edit <workspace> [--name NAME] [--description TEXT] [--networks id,id]
  workspace activate [--disconnect-others] <workspace>
  workspace deactivate
  workspace delete <workspace>
                                       <workspace> is an id or an exact name
  preference list
  preference set <destination> --network <network>
  preference remove <destination>      <network> is an id or an exact name

--json prints the daemon response as JSON for info, list, devices, diag, explain, conflicts, workspace list and preference list.
`

func main() {
	runtimeDir := flag.String("runtime-dir", "", "runtime directory parent (defaults to $XDG_RUNTIME_DIR)")
	asJSON := flag.Bool("json", false, "print machine-readable JSON")
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
	base, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx := base
	if !longRunning[flag.Arg(0)] {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(base, 60*time.Second)
		defer cancel()
	}
	if err := run(ctx, client.New(paths.Socket), flag.Arg(0), flag.Args()[1:], *asJSON, os.Stdin, os.Stdout); err != nil {
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

func emit(out io.Writer, m proto.Message) error {
	b, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(m)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(b))
	return err
}

func run(ctx context.Context, c *client.Client, cmd string, args []string, asJSON bool, stdin io.Reader, out io.Writer) error {
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
		if asJSON {
			return emit(out, res.Msg)
		}
		m := res.Msg
		fmt.Fprintf(out, "latticed %s (commit %s)\nprotocol %d.%d\ninstance %s\n", m.DaemonVersion, m.BuildCommit, m.ProtocolMajor, m.ProtocolMinor, m.DaemonInstanceId)
	case "list":
		res, err := c.Networks.ListNetworks(ctx, connect.NewRequest(&v1.ListNetworksRequest{}))
		if err != nil {
			return err
		}
		if asJSON {
			return emit(out, res.Msg)
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
		if asJSON {
			return emit(out, res.Msg)
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
		if asJSON {
			return emit(out, res.Msg)
		}
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		for _, ch := range res.Msg.Checks {
			fmt.Fprintf(w, "%s\t%s\t%s\n", ch.Status, ch.Name, ch.SafeDetail)
		}
		return w.Flush()
	case "explain", "inspect":
		if err := need(1); err != nil {
			return err
		}
		res, err := c.Inspector.InspectDestination(ctx, connect.NewRequest(&v1.InspectDestinationRequest{Destination: args[0]}))
		if err != nil {
			return err
		}
		if asJSON {
			return emit(out, res.Msg)
		}
		return printExplain(out, res.Msg)
	case "conflicts":
		res, err := c.Conflicts.ListConflicts(ctx, connect.NewRequest(&v1.ListConflictsRequest{}))
		if err != nil {
			return err
		}
		if asJSON {
			return emit(out, res.Msg)
		}
		return printConflicts(out, res.Msg)
	case "forward":
		return runForward(ctx, c, args, out)
	case "socks", "proxy":
		return runSocks(ctx, c, args, out)
	case "workspace", "workspaces":
		return runWorkspace(ctx, c, args, asJSON, out)
	case "preference", "preferences", "prefer":
		return runPreference(ctx, c, args, asJSON, out)
	default:
		return fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
	}
	return nil
}

func enumName(s, prefix string) string {
	return strings.ToLower(strings.TrimPrefix(s, prefix))
}

func printExplain(out io.Writer, r *v1.InspectDestinationResponse) error {
	what := enumName(r.Kind.String(), "DESTINATION_KIND_")
	fmt.Fprintf(out, "destination  %s (%s)", r.Normalized, what)
	if r.Port > 0 {
		fmt.Fprintf(out, ", port %d", r.Port)
	}
	fmt.Fprintln(out)
	decision := enumName(r.Decision.String(), "RESOLUTION_DECISION_")
	switch r.Decision {
	case v1.ResolutionDecision_RESOLUTION_DECISION_AMBIGUOUS:
		nets := map[string]bool{}
		for _, c := range r.Candidates {
			if c.Status == v1.CandidateStatus_CANDIDATE_STATUS_TIED {
				nets[c.Network.GetId()] = true
			}
		}
		decision += fmt.Sprintf(": exists on %d network(s); use a full DNS name to pick one", len(nets))
	case v1.ResolutionDecision_RESOLUTION_DECISION_UNIQUE:
		for _, c := range r.Candidates {
			if c.Status == v1.CandidateStatus_CANDIDATE_STATUS_SELECTED {
				decision += fmt.Sprintf(": %s on %s", c.Device.GetHostname(), c.Network.GetDisplayName())
			}
		}
	}
	fmt.Fprintf(out, "decision     %s\n", strings.ReplaceAll(decision, "_", " "))
	fmt.Fprintf(out, "reason       %s\n", strings.ReplaceAll(enumName(r.Reason.String(), "DECISION_REASON_"), "_", " "))
	if p := r.Preference; p != nil {
		state := map[v1.PreferenceState]string{
			v1.PreferenceState_PREFERENCE_STATE_APPLIED:               "applied",
			v1.PreferenceState_PREFERENCE_STATE_NETWORK_NOT_CONNECTED: "not applied, network not connected",
			v1.PreferenceState_PREFERENCE_STATE_NO_MATCH_ON_NETWORK:   "not applied, no matching device on that network",
		}[p.State]
		fmt.Fprintf(out, "preference   %s (%s)\n", p.Network.GetDisplayName(), state)
	}
	if len(r.Candidates) > 0 {
		fmt.Fprintln(out)
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "NETWORK\tDEVICE\tMATCH\tSTATUS\tADDRESSES")
		for _, c := range r.Candidates {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				c.Network.GetDisplayName(), c.Device.GetHostname(),
				matchText(c),
				enumName(c.Status.String(), "CANDIDATE_STATUS_"),
				strings.Join(c.Device.GetAddresses(), ","))
		}
		if err := w.Flush(); err != nil {
			return err
		}
	}
	if len(r.NotInspected) > 0 {
		names := make([]string, 0, len(r.NotInspected))
		for _, n := range r.NotInspected {
			names = append(names, n.DisplayName)
		}
		fmt.Fprintf(out, "\nnot checked (not connected): %s\n", strings.Join(names, ", "))
	}
	return nil
}

func printConflicts(out io.Writer, r *v1.ListConflictsResponse) error {
	if len(r.Conflicts) == 0 {
		fmt.Fprintln(out, "no addresses or names exist more than once on your connected networks")
	}
	for i, c := range r.Conflicts {
		if i > 0 {
			fmt.Fprintln(out)
		}
		kind := strings.ReplaceAll(enumName(c.Type.String(), "CONFLICT_TYPE_"), "_", " ")
		state := "expected overlap: network-specific DNS names stay unambiguous"
		if c.Type == v1.ConflictType_CONFLICT_TYPE_SUBNET_OVERLAP {
			state = "expected overlap: the more specific route decides"
		}
		if c.Severity == v1.ConflictSeverity_CONFLICT_SEVERITY_AMBIGUOUS && c.Type == v1.ConflictType_CONFLICT_TYPE_SUBNET_OVERLAP {
			state = "ambiguous: the same route is advertised on several networks"
		} else if c.Severity == v1.ConflictSeverity_CONFLICT_SEVERITY_AMBIGUOUS {
			state = "ambiguous: no network-specific name tells these apart"
			if c.Scope == v1.ConflictScope_CONFLICT_SCOPE_WITHIN_NETWORK {
				state = "ambiguous: several devices on one network share it"
			}
		}
		fmt.Fprintf(out, "%s  %s\n  %s\n  id %s\n", c.Value, kind, state, c.Id)
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		if c.Type == v1.ConflictType_CONFLICT_TYPE_SUBNET_OVERLAP {
			fmt.Fprintln(w, "  NETWORK\tROUTER\tROUTE")
			for _, m := range c.Members {
				fmt.Fprintf(w, "  %s\t%s\t%s\n", m.Network.GetDisplayName(), m.Device.GetHostname(), m.Route)
			}
		} else {
			fmt.Fprintln(w, "  NETWORK\tDEVICE\tADDRESSES\tUNIQUE NAME")
			for _, m := range c.Members {
				name := m.UniqueName
				if name == "" {
					name = "-"
				}
				fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", m.Network.GetDisplayName(), m.Device.GetHostname(), strings.Join(m.Device.GetAddresses(), ","), name)
			}
		}
		if err := w.Flush(); err != nil {
			return err
		}
	}
	if len(r.NotInspected) > 0 {
		names := make([]string, 0, len(r.NotInspected))
		for _, n := range r.NotInspected {
			names = append(names, n.DisplayName)
		}
		fmt.Fprintf(out, "\nnot checked (not connected): %s\n", strings.Join(names, ", "))
	}
	return nil
}

func runWorkspace(ctx context.Context, c *client.Client, args []string, asJSON bool, out io.Writer) error {
	if len(args) == 0 {
		args = []string{"list"}
	}
	list, err := c.Workspaces.ListWorkspaces(ctx, connect.NewRequest(&v1.ListWorkspacesRequest{}))
	if err != nil {
		return err
	}
	find := func(ref string) (*v1.Workspace, error) {
		for _, w := range list.Msg.Workspaces {
			if w.Id == ref || w.Name == ref {
				return w, nil
			}
		}
		return nil, fmt.Errorf("no workspace named %q", ref)
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list", "ls":
		if asJSON {
			return emit(out, list.Msg)
		}
		nets, err := c.Networks.ListNetworks(ctx, connect.NewRequest(&v1.ListNetworksRequest{}))
		if err != nil {
			return err
		}
		names := map[string]string{}
		for _, n := range nets.Msg.Networks {
			names[n.Id] = n.DisplayName
		}
		if len(list.Msg.Workspaces) == 0 {
			fmt.Fprintln(out, "no workspaces")
			return nil
		}
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tACTIVE\tNETWORKS")
		for _, ws := range list.Msg.Workspaces {
			members := make([]string, 0, len(ws.NetworkIds))
			for _, id := range ws.NetworkIds {
				members = append(members, names[id])
			}
			active := ""
			if ws.Id == list.Msg.ActiveWorkspaceId {
				active = "yes"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", ws.Id, ws.Name, active, strings.Join(members, ", "))
		}
		return w.Flush()
	case "create":
		fs := flag.NewFlagSet("workspace create", flag.ContinueOnError)
		name := fs.String("name", "", "workspace name")
		desc := fs.String("description", "", "optional description")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		res, err := c.Workspaces.CreateWorkspace(ctx, connect.NewRequest(&v1.CreateWorkspaceRequest{Name: *name, Description: *desc, NetworkIds: fs.Args()}))
		if err != nil {
			return err
		}
		fmt.Fprintln(out, res.Msg.Workspace.Id)
	case "edit":
		if len(rest) == 0 {
			return fmt.Errorf("workspace edit: expected a workspace\n\n%s", usage)
		}
		ws, err := find(rest[0])
		if err != nil {
			return err
		}
		fs := flag.NewFlagSet("workspace edit", flag.ContinueOnError)
		name := fs.String("name", "", "new name")
		desc := fs.String("description", "", "new description")
		networks := fs.String("networks", "", "comma-separated network ids, replaces membership")
		if err := fs.Parse(rest[1:]); err != nil {
			return err
		}
		req := &v1.UpdateWorkspaceRequest{WorkspaceId: ws.Id}
		fs.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "name":
				req.Name = name
			case "description":
				req.Description = desc
			case "networks":
				ids := []string{}
				for _, id := range strings.Split(*networks, ",") {
					if id = strings.TrimSpace(id); id != "" {
						ids = append(ids, id)
					}
				}
				req.Networks = &v1.NetworkIDList{NetworkIds: ids}
			}
		})
		_, err = c.Workspaces.UpdateWorkspace(ctx, connect.NewRequest(req))
		return err
	case "activate":
		fs := flag.NewFlagSet("workspace activate", flag.ContinueOnError)
		others := fs.Bool("disconnect-others", false, "disconnect networks outside the workspace")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return fmt.Errorf("workspace activate: expected a workspace\n\n%s", usage)
		}
		ws, err := find(fs.Arg(0))
		if err != nil {
			return err
		}
		res, err := c.Workspaces.ActivateWorkspace(ctx, connect.NewRequest(&v1.ActivateWorkspaceRequest{WorkspaceId: ws.Id, DisconnectOthers: *others}))
		if err != nil {
			return err
		}
		if asJSON {
			return emit(out, res.Msg)
		}
		fmt.Fprintf(out, "activated %s\n", ws.Name)
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		for _, r := range res.Msg.Results {
			fmt.Fprintf(w, "  %s\t%s\t%s\n", r.Network.GetDisplayName(), strings.ReplaceAll(enumName(r.Outcome.String(), "ACTIVATION_OUTCOME_"), "_", " "), r.SafeMessage)
		}
		return w.Flush()
	case "deactivate":
		_, err := c.Workspaces.DeactivateWorkspace(ctx, connect.NewRequest(&v1.DeactivateWorkspaceRequest{}))
		return err
	case "delete", "rm":
		if len(rest) != 1 {
			return fmt.Errorf("workspace delete: expected a workspace\n\n%s", usage)
		}
		ws, err := find(rest[0])
		if err != nil {
			return err
		}
		_, err = c.Workspaces.DeleteWorkspace(ctx, connect.NewRequest(&v1.DeleteWorkspaceRequest{WorkspaceId: ws.Id}))
		return err
	default:
		return fmt.Errorf("unknown workspace command %q\n\n%s", sub, usage)
	}
	return nil
}

func runPreference(ctx context.Context, c *client.Client, args []string, asJSON bool, out io.Writer) error {
	if len(args) == 0 {
		args = []string{"list"}
	}
	nets, err := c.Networks.ListNetworks(ctx, connect.NewRequest(&v1.ListNetworksRequest{}))
	if err != nil {
		return err
	}
	names := map[string]string{}
	for _, n := range nets.Msg.Networks {
		names[n.Id] = n.DisplayName
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list", "ls":
		res, err := c.Preferences.ListDestinationPreferences(ctx, connect.NewRequest(&v1.ListDestinationPreferencesRequest{}))
		if err != nil {
			return err
		}
		if asJSON {
			return emit(out, res.Msg)
		}
		if len(res.Msg.Preferences) == 0 {
			fmt.Fprintln(out, "no destination preferences")
			return nil
		}
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "DESTINATION\tKIND\tPREFERRED NETWORK")
		for _, p := range res.Msg.Preferences {
			fmt.Fprintf(w, "%s\t%s\t%s\n", p.Destination, enumName(p.Kind.String(), "DESTINATION_KIND_"), names[p.NetworkId])
		}
		return w.Flush()
	case "set":
		if len(rest) == 0 {
			return fmt.Errorf("preference set: expected a destination\n\n%s", usage)
		}
		fs := flag.NewFlagSet("preference set", flag.ContinueOnError)
		network := fs.String("network", "", "preferred network id or name")
		if err := fs.Parse(rest[1:]); err != nil {
			return err
		}
		id := *network
		for nid, name := range names {
			if name == *network {
				id = nid
			}
		}
		res, err := c.Preferences.SetDestinationPreference(ctx, connect.NewRequest(&v1.SetDestinationPreferenceRequest{Destination: rest[0], NetworkId: id}))
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s now prefers %s (a Lattice preference; system routing is not changed)\n", res.Msg.Preference.Destination, names[res.Msg.Preference.NetworkId])
	case "remove", "rm", "delete":
		if len(rest) != 1 {
			return fmt.Errorf("preference remove: expected a destination\n\n%s", usage)
		}
		_, err := c.Preferences.DeleteDestinationPreference(ctx, connect.NewRequest(&v1.DeleteDestinationPreferenceRequest{Destination: rest[0]}))
		return err
	default:
		return fmt.Errorf("unknown preference command %q\n\n%s", sub, usage)
	}
	return nil
}

func matchText(c *v1.ResolutionCandidate) string {
	kind := strings.ReplaceAll(enumName(c.Match.String(), "MATCH_KIND_"), "_", " ")
	if c.Match == v1.MatchKind_MATCH_KIND_SUBNET_ROUTE {
		return kind + " " + c.MatchedValue
	}
	return kind
}

var longRunning = map[string]bool{"forward": true, "socks": true, "proxy": true}

func splitFlags(args []string) (positional []string, flags []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			if !strings.Contains(a, "=") && i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		positional = append(positional, a)
	}
	return positional, flags
}

func reasonText(r v1.DecisionReason) string {
	return strings.ReplaceAll(enumName(r.String(), "DECISION_REASON_"), "_", " ")
}

func runForward(ctx context.Context, c *client.Client, args []string, out io.Writer) error {
	positional, flags := splitFlags(args)
	fs := flag.NewFlagSet("forward", flag.ContinueOnError)
	network := fs.String("network", "", "network id or name to use")
	listen := fs.String("listen", "", "loopback address to listen on (default 127.0.0.1 with a free port)")
	if err := fs.Parse(flags); err != nil {
		return err
	}
	if len(positional) != 1 {
		return fmt.Errorf("forward: expected one destination with a port\n\n%s", usage)
	}
	stream, err := c.Forwards.Forward(ctx, connect.NewRequest(&v1.ForwardRequest{Destination: positional[0], Listen: *listen, Network: *network}))
	if err != nil {
		return err
	}
	for stream.Receive() {
		ev := stream.Msg()
		switch {
		case ev.GetStarted() != nil:
			s := ev.GetStarted()
			r := s.GetRoute()
			fmt.Fprintf(out, "Forwarding\n\n  %s\n      ↓\n  %s\n      ↓\n  %s\n      ↓\n  %s\n\nReason: %s\n",
				s.ListenAddress, positional[0], r.GetNetwork().GetDisplayName(), r.Target, reasonText(r.GetDecision().GetReason()))
			fmt.Fprintln(out, "Each new connection is checked again before it is forwarded. Ctrl+C to stop.")
		case ev.GetOpened() != nil:
			o := ev.GetOpened()
			fmt.Fprintf(out, "opened  #%d  %s -> %s %s\n", o.Id, o.Client, o.GetRoute().GetNetwork().GetDisplayName(), o.GetRoute().Target)
		case ev.GetClosed() != nil:
			o := ev.GetClosed()
			fmt.Fprintf(out, "closed  #%d  sent %d B, received %d B\n", o.Id, o.BytesSent, o.BytesReceived)
		case ev.GetRefused() != nil:
			o := ev.GetRefused()
			fmt.Fprintf(out, "refused #%d  %s: %s\n", o.Id, o.Client, o.GetReason().GetSafeMessage())
		}
	}
	if err := stream.Err(); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

func runSocks(ctx context.Context, c *client.Client, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("socks", flag.ContinueOnError)
	listen := fs.String("listen", "127.0.0.1:1080", "loopback address to listen on")
	if err := fs.Parse(args); err != nil {
		return err
	}
	stream, err := c.Forwards.Proxy(ctx, connect.NewRequest(&v1.ProxyRequest{Listen: *listen}))
	if err != nil {
		return err
	}
	for stream.Receive() {
		ev := stream.Msg()
		switch {
		case ev.GetStarted() != nil:
			addr := ev.GetStarted().ListenAddress
			fmt.Fprintf(out, "SOCKS5 proxy on %s (this user only)\n\n", addr)
			fmt.Fprintf(out, "Only devices and routes on your Lattice networks are reachable; ambiguous addresses are refused.\n")
			fmt.Fprintf(out, "Let Lattice resolve names: use socks5h / remote DNS, for example\n\n  curl --proxy socks5h://%s http://grafana.home.lattice.internal:3000/\n\nCtrl+C to stop.\n\n", addr)
		case ev.GetOpened() != nil:
			o := ev.GetOpened()
			fmt.Fprintf(out, "opened  #%d  %s -> %s %s\n", o.Id, o.Destination, o.GetRoute().GetNetwork().GetDisplayName(), o.GetRoute().Target)
		case ev.GetClosed() != nil:
			o := ev.GetClosed()
			fmt.Fprintf(out, "closed  #%d  sent %d B, received %d B\n", o.Id, o.BytesSent, o.BytesReceived)
		case ev.GetRefused() != nil:
			o := ev.GetRefused()
			fmt.Fprintf(out, "refused #%d  %s: %s\n", o.Id, o.Destination, o.GetReason().GetSafeMessage())
		}
	}
	if err := stream.Err(); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}
