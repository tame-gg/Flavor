<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/readme/banner-dark.svg">
  <img src="assets/readme/banner-light.svg" alt="Flavor: every tailnet, side by side" width="100%">
</picture>

<br>
<br>

[![CI](https://img.shields.io/github/actions/workflow/status/tame-gg/flavor/ci.yml?branch=main&style=flat-square&label=ci&labelColor=0F1419)](https://github.com/tame-gg/flavor/actions/workflows/ci.yml) [![License](https://img.shields.io/badge/license-MIT-6BA3C7?style=flat-square&labelColor=0F1419)](LICENSE) [![Platform](https://img.shields.io/badge/platform-Linux-6BA3C7?style=flat-square&labelColor=0F1419)](#requirements) [![Stars](https://img.shields.io/github/stars/tame-gg/flavor?style=flat-square&color=6BA3C7&labelColor=0F1419)](https://github.com/tame-gg/flavor/stargazers) [![Last commit](https://img.shields.io/github/last-commit/tame-gg/flavor?style=flat-square&color=6BA3C7&labelColor=0F1419)](https://github.com/tame-gg/flavor/commits) [![Code size](https://img.shields.io/github/languages/code-size/tame-gg/flavor?style=flat-square&color=6BA3C7&labelColor=0F1419)](https://github.com/tame-gg/flavor)

[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?style=flat-square&logo=go&logoColor=white&labelColor=0F1419)](go.mod) [![Rust](https://img.shields.io/badge/Rust-stable-CE422B?style=flat-square&logo=rust&logoColor=white&labelColor=0F1419)](Cargo.toml) [![Tauri](https://img.shields.io/badge/Tauri-2-FFC131?style=flat-square&logo=tauri&logoColor=white&labelColor=0F1419)](desktop/src-tauri) [![React](https://img.shields.io/badge/React-19-61DAFB?style=flat-square&logo=react&logoColor=white&labelColor=0F1419)](desktop) [![tsnet](https://img.shields.io/badge/tsnet-1.90.9-6BA3C7?style=flat-square&logo=tailscale&logoColor=white&labelColor=0F1419)](https://pkg.go.dev/tailscale.com/tsnet)

**[Quick start](#quick-start)** · **[How it works](#how-it-works)** · **[CLI](#cli)** · **[Security](#security-model)** · **[Development](#development)**

</div>

---

The system Tailscale client joins one tailnet at a time. Switching between a work account, a personal tailnet and a self-hosted Headscale means logging out, logging in and dropping every connection in between.

**Flavor keeps them all connected at once.** A small per-user daemon runs one isolated, embedded Tailscale node for each network you add. Each has its own device identity, state directory and peer list. A Tauri desktop app and a CLI drive it over a local socket. It needs no root and no system `tailscaled`.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/readme/networks-dark.png">
  <img src="assets/readme/networks-light.png" alt="Flavor networks view with a Headscale lab, a Tailscale work tailnet and a network waiting for sign-in">
</picture>

<table>
  <tr>
    <td width="50%">
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="assets/readme/signin-dark.png">
        <img src="assets/readme/signin-light.png" alt="Sign-in panel showing the destination host before opening the browser">
      </picture>
      <p align="center"><sub><b>Explicit sign-in.</b> Shows the destination host and opens only after you click.</sub></p>
    </td>
    <td width="50%">
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="assets/readme/devices-dark.png">
        <img src="assets/readme/devices-light.png" alt="Devices view showing 100.64.0.1 on two networks as two separate machines">
      </picture>
      <p align="center"><sub><b>Same IP, two machines.</b> Devices are keyed by network and node, never by address.</sub></p>
    </td>
  </tr>
</table>

## Features

<table>
  <tr>
    <td width="50%" valign="top"><b>Many tailnets at once</b><br>Tailscale and any number of Headscale servers side by side in one daemon. Each network runs its own embedded <a href="https://pkg.go.dev/tailscale.com/tsnet"><code>tsnet</code></a> node.</td>
    <td width="50%" valign="top"><b>Overlapping addresses are fine</b><br>Both networks handing out <code>100.64.0.1</code> is normal. Every device is identified by <code>{network, node}</code> all the way from the daemon to the UI.</td>
  </tr>
  <tr>
    <td width="50%" valign="top"><b>Unprivileged</b><br>Runs as your user, with no system <code>tailscale</code> or <code>tailscaled</code>. Optional system-wide names use a small socket-activated helper limited to <code>CAP_NET_ADMIN</code>.</td>
    <td width="50%" valign="top"><b>Sign in your way</b><br>Browser sign-in (including OIDC on a different host) or a one-time pre-auth key that is used once and never stored.</td>
  </tr>
  <tr>
    <td width="50%" valign="top"><b>Survives restarts</b><br>Node identities persist per network, so reconnecting after a reboot needs no keys. <i>Connect automatically when Flavor starts</i> is a per-network choice.</td>
    <td width="50%" valign="top"><b>Remove vs delete</b><br>Removing a network keeps its identity on disk for later. Deleting the identity is a separate, explicit step. Flavor never deletes machines on the control server.</td>
  </tr>
  <tr>
    <td width="50%" valign="top"><b>Connection Inspector</b><br>Type an address or name and see every network it exists on, which device or subnet route matched, how it matched, and whether the answer is unique. Longest-prefix routes and network-qualified names like <code>postgres.home.flavor.internal</code> are explained the same way. The same engine powers <code>flavorctl explain</code>.</td>
    <td width="50%" valign="top"><b>Conflict Center</b><br>Every address, DNS name, device name and subnet route that exists more than once, split into expected overlaps (network-specific names or a more specific route decide) and real ambiguities.</td>
  </tr>
  <tr>
    <td width="50%" valign="top"><b>Device Explorer</b><br>Search all networks at once by name, address, network, OS or tag, with qualifiers like <code>is:online</code> and <code>tag:db</code>. Each device opens a details panel with copy, inspect and SSH actions.</td>
    <td width="50%" valign="top"><b>Live and resilient UI</b><br>A snapshot plus an ordered event stream. The UI resyncs on gaps or daemon restarts and keeps showing last-known state while the daemon is away.</td>
  </tr>
  <tr>
    <td width="50%" valign="top"><b>Workspaces</b><br>Name a set of networks after what you are doing, like Work or On Call, and connect them in one step. Optionally disconnect everything else. Identities are never touched.</td>
    <td width="50%" valign="top"><b>Destination preferences</b><br>When an address or name exists on several networks, tell Flavor which one you mean. The inspector, conflict view and CLI all explain the choice. System routing is not changed.</td>
  </tr>
  <tr>
    <td width="50%" valign="top"><b>Scriptable</b><br><code>flavorctl</code> speaks the same API as the desktop app, with <code>--json</code> output for automation.</td>
    <td width="50%" valign="top"><b>Private by default</b><br>No telemetry. Device inventories, sign-in links and keys stay on your machine, and Tailscale log upload is disabled for every session.</td>
  </tr>
</table>

## How it works

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/readme/architecture-dark.svg">
  <img src="assets/readme/architecture-light.svg" alt="Architecture: React UI and Tauri shell talk Connect-RPC over a Unix socket to flavord, which runs one tsnet NetworkSession per Tailscale or Headscale control plane">
</picture>

- **`flavord`**: the per-user Go daemon. It owns the SQLite network registry, the system keyring, an in-memory event bus with bounded replay, and one `NetworkSession` per network.
- **IPC**: [Connect-RPC](https://connectrpc.com) over HTTP/2 cleartext on `$XDG_RUNTIME_DIR/flavor/flavord.sock`, mode `0600`. Every connection's UID is checked with `SO_PEERCRED`. The contract lives in [`proto/flavor/v1`](proto/flavor/v1).
- **Desktop**: [Tauri 2](https://tauri.app) with a thin Rust client ([`crates/flavor-ipc`](crates/flavor-ipc)) built on [connect-rust](https://github.com/connectrpc/connect-rust). The React webview never touches the socket. It only calls a fixed list of app commands.
- **Sync**: the UI loads a snapshot stamped with a sequence number, then streams events after that sequence. Missed events, a slow consumer or a restarted daemon all trigger a fresh snapshot instead of silent drift.

## Quick start

### Install a release

Releases ship a tarball per architecture (`linux-amd64`, `linux-arm64`) with the daemon, CLI, desktop app, the `flavor-netd` helper, its systemd units, polkit action, SELinux module and AppArmor profile. The desktop app needs glibc 2.39+, `libwebkit2gtk-4.1` and `libayatana-appindicator3`.

```bash
v=0.1.0-beta.2
base=https://github.com/tame-gg/Flavor/releases/download/v$v
curl -fLO $base/flavor-$v-linux-amd64.tar.gz -fLO $base/SHA256SUMS -fLO $base/SHA256SUMS.sigstore.json -fLO $base/SHA256SUMS.asc

cosign verify-blob --bundle SHA256SUMS.sigstore.json \
  --certificate-identity-regexp '^https://github.com/tame-gg/Flavor/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com SHA256SUMS
gh attestation verify flavor-$v-linux-amd64.tar.gz --repo tame-gg/Flavor
gpg --verify SHA256SUMS.asc SHA256SUMS
sha256sum --check --ignore-missing SHA256SUMS

tar -xzf flavor-$v-linux-amd64.tar.gz
sudo ./flavor-$v-linux-amd64/install.sh
systemctl --user enable --now flavord
```

Any one of the three signature checks is enough; each covers `SHA256SUMS`, which covers the tarballs and SBOMs. `install.sh` copies files to `/usr` and `/etc` from the tarball's manifest, loads the AppArmor profile or SELinux module when that LSM is active, and enables `flavor-netd.socket`. Re-running it with a newer tarball upgrades in place and restarts running `flavord` user services.

System-wide names (`postgres.home.flavor.internal` in any app, through a TUN device and systemd-resolved) are experimental and off by default. To try them, run `systemctl --user edit flavord`, add the lines below, and restart it:

```ini
[Service]
ExecStart=
ExecStart=/usr/bin/flavord --synthetic-helper=/run/flavor/netd.sock
```

### Upgrading from Lattice

Flavor was called Lattice until `v0.1.0-beta.2`. Install Flavor the same way, then enable `flavord`:

- `install.sh` runs Lattice's uninstaller first. It stops `latticed` and removes Lattice's files, but keeps your data.
- On its first start, `flavord` moves `~/.local/share/lattice` and `~/.config/lattice` to the `flavor` paths. That covers the database, your networks and every device identity, so networks reconnect without signing in again. Lattice never stored credentials in the system keyring, so there is nothing to move there.
- `flavord` refuses to start while `latticed` is still running, and leaves the Lattice data alone (logging a warning) if `~/.local/share/flavor` already exists.
- A `systemctl --user edit latticed` override, such as the system-wide names setting, is not carried over. Recreate it with `systemctl --user edit flavord`.

### Uninstall

```bash
sudo /usr/libexec/flavor/uninstall
```

It stops `flavord` for logged-in users and the helper, removes any `flv-u*` interface, unloads the SELinux module and AppArmor profile, and deletes every installed file. Per-user data is kept:

```bash
rm -rf ~/.local/share/flavor ~/.config/flavor
```

> [!WARNING]
> Removing `~/.local/share/flavor` deletes every local device identity. The machines stay registered on their control servers until an administrator removes them.

### Build from source

Requires Go 1.27+, Rust stable, Node 24+ and the desktop libraries above.

```bash
./scripts/go.sh build -o ~/.local/bin/flavord   ./cmd/flavord
./scripts/go.sh build -o ~/.local/bin/flavorctl ./cmd/flavorctl

install -Dm644 packaging/systemd/user/flavord.service ~/.config/systemd/user/flavord.service
systemctl --user daemon-reload
systemctl --user enable --now flavord

(cd desktop && npm ci && npm run tauri build)
./target/release/flavor-desktop
```

> [!NOTE]
> Always build Go through `./scripts/go.sh`. It applies the toolchain settings in [`go.env`](go.env) that the pinned Tailscale version needs.

`./scripts/release.sh VERSION [amd64|arm64]` builds the same tarball a release ships; the binaries and the tarball are reproducible for a given commit. It needs `cargo install cargo-auditable --locked`, which embeds the desktop app's crate list. Each tarball's SBOM covers the Go modules, the Rust crates and the npm packages bundled into the desktop frontend; the last come from `/usr/share/flavor/frontend/package-lock.json`, the lockfile trimmed to production packages.

Closing the window hides Flavor in the tray by default. Quitting the desktop app never stops `flavord` or disconnects your networks.

## CLI

```console
$ flavorctl add --name "Home lab" --headscale https://headscale.example.com --auto-connect
01JA7Q3M0000000000000HOME0

$ printf '%s\n' "$PREAUTH_KEY" | flavorctl enroll 01JA7Q3M0000000000000HOME0

$ flavorctl list
ID                          NAME      PROVIDER   STATE           AUTO
01JA7Q3M0000000000000HOME0  Home lab  headscale  connected       true
01JA7Q3M0000000000000WORK0  Work      tailscale  authenticating  false
sign in to Work: https://login.tailscale.com/a/…

$ flavorctl explain 100.64.0.1
destination  100.64.0.1 (address)
decision     ambiguous: exists on 2 network(s); use a full DNS name to pick one
reason       multiple matches

NETWORK    DEVICE    MATCH           STATUS  ADDRESSES
Home       desktop   device address  tied    100.64.0.1
LunarLabs  prod-api  device address  tied    100.64.0.1

$ flavorctl forward postgres.home.flavor.internal:5432
Forwarding

  127.0.0.1:41753
      ↓
  postgres.home.flavor.internal:5432
      ↓
  Home
      ↓
  100.64.0.9:5432

Reason: network qualified name

$ flavorctl socks          # SOCKS5 on 127.0.0.1:1080, this user only
$ curl --proxy socks5h://127.0.0.1:1080 http://grafana.home.flavor.internal:3000/
$ flavorctl preference set 100.64.0.1 --network LunarLabs
$ flavorctl workspace create --name "On Call" <network-id> <network-id>
$ flavorctl workspace activate --disconnect-others "On Call"
$ flavorctl conflicts
$ flavorctl --json explain prod-api
$ flavorctl devices
$ flavorctl remove [--delete-identity] <network-id>
$ flavorctl diag
```

Pre-auth keys are read from stdin, so they never appear in the process list or in command-line arguments.

`forward` and `socks` run until you press Ctrl+C. They listen on loopback only, refuse connections from other local users, re-check every new connection with the same decision engine as the inspector, and never guess: an address that exists on two networks is refused until you use a Flavor name, a preference or `--network`.

## Security model

- **Same-user boundary.** The socket is `0600` inside your private runtime directory, and the daemon rejects any peer whose UID differs from its own. It does not try to defend against malware already running as you.
- **Keys are transient.** A pre-auth key goes from the request to the embedded node once, then is dropped. It is never written to SQLite, config, the keyring, logs, events or diagnostics. Automated tests plant a canary key and check that it never shows up in logs, RPC responses, events, diagnostics or any file the daemon writes.
- **No ambient credentials.** The daemon clears `TS_AUTHKEY`/`TS_AUTH_KEY` at startup and refuses to start a node if they are still set. Tailscale log upload is disabled for the whole process.
- **Untrusted webview.** The Tauri capability grants only Flavor's own commands and event listening: no shell, filesystem, HTTP or opener access. CSP is `self` only. Sign-in links are re-read from the daemon by Rust, limited to `http`/`https` without credentials, and opened in the external browser only after you click.
- **Logical isolation.** Sessions run in one process with separate state directories and node keys. They are not process-sandboxed from each other.
- **Minimal privileged helper.** `flavor-netd` is socket-activated, runs with only `CAP_NET_ADMIN` under systemd sandboxing, and is confined by its SELinux module or AppArmor profile. It creates exactly one TUN device with Flavor's own addresses and routes and points systemd-resolved at Flavor for `flavor.internal` only; it never edits `/etc/resolv.conf`. Every change needs polkit authorization and an active local login session, identity comes from the kernel (`SO_PEERCRED`, pidfd), and everything is removed when the owning `flavord` goes away. Routes are host-wide, so system-wide names are supported on single-user machines only.

## Not in this release

Flavor deliberately ships no placeholder screens. These are planned and not built:

- System-wide names on multi-user machines
- Exit nodes, subnet route controls and an HTTP proxy
- Windows and macOS
- Remote node removal on the control server

## Development

```bash
./scripts/go.sh run ./cmd/flavord              # daemon on the default socket
cd desktop && npm install && npm run tauri dev  # desktop app with hot reload
```

No tailnet handy? `./scripts/go.sh run ./test/fakedaemon` serves the same API with simulated sessions. Control URLs containing `login` or `approval` simulate those states.

```bash
./scripts/go.sh test ./...          # Go: services, sessions, IPC end to end
cargo test --workspace              # Rust: client against a real daemon, Tauri settings
cd desktop && npm test              # Frontend: sync controller, device keys, link rules
./scripts/generate-proto.sh         # regenerate Go, TypeScript and Rust bindings
```

Real control planes: `test/acceptance/headscale-concurrent.sh` runs two local Headscale servers in Docker and checks concurrent sessions, restart persistence, remove vs delete, and that no key reaches disk or logs.

```bash
docker compose -f test/integration/headscale/compose.yml --profile dual up -d
./test/acceptance/headscale-concurrent.sh
```

<details>
<summary><b>Repository layout</b></summary>

```text
cmd/flavord            daemon entry point
cmd/flavorctl          command-line client
internal/app            process lifecycle: lock, startup order, phased shutdown
internal/service        use-cases: networks, devices, diagnostics, snapshot
internal/session        one tsnet node per network; lifecycle and peer projection
internal/ipc            Unix socket transport, peer credentials, Connect handlers
internal/store          SQLite registry and retained identities
internal/secret         Secret Service keyring and in-memory store
internal/events         event bus with bounded replay
proto/flavor/v1        IPC contract
gen/                    generated Go and TypeScript bindings
crates/flavor-proto    generated Rust bindings
crates/flavor-ipc      Rust daemon client
desktop/                React UI and Tauri shell
packaging/              systemd user unit
test/                   fake daemon, Headscale harness, acceptance script
```

</details>

## License

[MIT](LICENSE) © 2026 Luna
