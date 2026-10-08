# Architecture

Flavor is a per-user daemon with two clients, plus an optional privileged helper.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="../assets/readme/architecture-dark.svg">
  <img src="../assets/readme/architecture-light.svg" alt="The Flavor desktop app, flavorctl and your own apps talk to flavord, a per-user daemon, over a local Unix socket, SOCKS5 or a forwarded port. flavord runs one embedded tsnet node per network, each connected to its own Tailscale or Headscale control server. An optional root helper, flavor-netd, creates a TUN device, routes and systemd-resolved configuration for system-wide Flavor names.">
</picture>

## Components

| Component | Language | Role |
| --- | --- | --- |
| `flavord` | Go | Per-user daemon. Owns the network registry, runs one embedded Tailscale node per network, answers the desktop app and CLI, and runs forwarding and the SOCKS5 proxy. |
| `flavor-desktop` | Rust (Tauri 2) and TypeScript (React) | Desktop app. A thin Rust shell talks to the daemon; the React interface only calls a fixed list of the shell's commands. |
| `flavorctl` | Go | Command-line client for the same API. |
| `flavor-netd` | Go | Optional root helper for [system-wide names](system-wide-names.md): creates the TUN interface, routes and systemd-resolved settings. |

### flavord

- **Sessions.** Each network is a `NetworkSession` built on [`tsnet`](https://pkg.go.dev/tailscale.com/tsnet), Tailscale's embeddable client, pinned to `tailscale.com v1.90.9`. Each has its own state directory, node key and peer list. Tailscale networks use Tailscale's default control server; Headscale networks use the configured control URL. No system `tailscaled` is involved.
- **Identity.** Devices are keyed by `{network ID, node ID}` from the session all the way to the UI. Addresses are attributes, never keys.
- **Decision engine** (`internal/inspect`). One `Resolve` function answers "which network does this destination belong to, and why" for the Connection Inspector, `flavorctl explain`, forwarding, the SOCKS5 proxy and the synthetic DNS server. `Conflicts` builds the Conflict Center from the same data.
- **Storage.** A SQLite database stores durable intent: networks, workspaces, destination preferences and synthetic address mappings. Live state such as peers and connection status lives in memory.
- **Events.** An in-memory event bus with bounded replay feeds the clients.
- **Connections.** Forwarding and the SOCKS5 proxy accept loopback connections from the same user and dial through the chosen session's netstack (`tsnet.Server.Dial`).
- **System-wide names** (only with `--synthetic-helper`). A synthetic address allocator, a DNS answerer and a userspace netstack (gVisor) behind the TUN interface the helper provides.

### IPC

- The desktop app and `flavorctl` call `flavord` with [Connect-RPC](https://connectrpc.com) over HTTP/2 cleartext on a Unix socket, `$XDG_RUNTIME_DIR/flavor/flavord.sock`. The kernel-reported peer user ID must match the daemon's.
- The API is defined in [`proto/flavor/v1`](../proto/flavor/v1). Generated code lives in `gen/` (Go and TypeScript) and `crates/flavor-proto` (Rust).
- The protocol is versioned. The daemon currently reports protocol 1.3, and the desktop app requires major version 1.
- `flavord` and `flavor-netd` use a separate, smaller protocol: one protobuf message per `SOCK_SEQPACKET` packet, defined in [`proto/flavor/netd/v1`](../proto/flavor/netd/v1).

### Keeping the UI in sync

The desktop app loads a snapshot of the daemon's state, stamped with a sequence number and the daemon's instance ID, then subscribes to events after that sequence. A gap in the sequence, a slow consumer or a restarted daemon (new instance ID) makes the app fetch a fresh snapshot instead of drifting. While the daemon is unreachable, the app keeps showing the last known state and retries.

## Data locations

| What | Path |
| --- | --- |
| Database | `$XDG_DATA_HOME/flavor/database/flavor.db` (default `~/.local/share/flavor/database/flavor.db`) |
| Network identities | `$XDG_DATA_HOME/flavor/networks/<network-id>/tsnet/` |
| Desktop settings | `$XDG_CONFIG_HOME/flavor/config.toml` |
| Socket and lock | `$XDG_RUNTIME_DIR/flavor/` |

All of these directories are created with mode `0700` and must be owned by the user.

## Repository layout

```text
cmd/flavord             daemon entry point
cmd/flavorctl           command-line client
cmd/flavor-netd         privileged helper entry point
internal/app            process lifecycle: lock, Lattice migration, startup order, shutdown
internal/config         XDG paths and directory permission checks
internal/domain         networks, devices, workspaces, preferences, labels
internal/events         event bus with bounded replay
internal/inspect        decision engine and conflict detection
internal/ipc            Unix socket transport, peer credentials, Connect handlers and client
internal/logging        structured logging and URL redaction
internal/naming         Flavor names (<device>.<network>.flavor.internal)
internal/netd           flavor-netd server, flavord's client and supervisor
internal/provider       Tailscale and Headscale session configuration
internal/relay          byte relay shared by forwarding, SOCKS5 and the data plane
internal/secret         keyring and in-memory secret stores
internal/service        use cases: networks, devices, inspect, forward, SOCKS5, workspaces, diagnostics
internal/session        one tsnet node per network; lifecycle and peer projection
internal/store          SQLite schema, migrations and repositories
internal/synthetic      synthetic address layout and allocation
internal/syndns         DNS answers for flavor.internal
internal/dataplane      userspace netstack behind the TUN interface
internal/version        version and capability reporting
proto/                  API definitions (daemon API and helper protocol)
gen/                    generated Go and TypeScript bindings
crates/flavor-proto     generated Rust bindings
crates/flavor-ipc       Rust client for the daemon, used by the desktop shell
desktop/                React interface (src/) and Tauri shell (src-tauri/)
packaging/              systemd units, polkit policy, AppArmor, SELinux, installer, AUR PKGBUILDs
scripts/                Go wrapper, code generation, release build
test/fakedaemon         daemon with simulated sessions, for UI and CLI work
test/integration        Headscale test environment (Docker Compose)
test/acceptance         end-to-end test against two real Headscale servers
```
