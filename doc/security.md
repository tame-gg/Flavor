# Security model

Flavor holds the keys to several private networks at once, and its optional helper changes host networking. This page describes what each part can do, how access is controlled, and what Flavor does not protect against. To report a vulnerability, see [SECURITY.md](../SECURITY.md).

Flavor has not had an independent security audit.

## Components and privileges

| Component | Runs as | Privileges | Talks to |
| --- | --- | --- | --- |
| `flavor-desktop` | you | none beyond your user | `flavord`, through the Tauri shell |
| `flavorctl` | you | none beyond your user | `flavord` |
| `flavord` | you, as a systemd user service | none; no Linux capabilities | your control servers and peers, `flavor-netd` if enabled |
| `flavor-netd` (optional) | root, started on demand by a systemd socket | `CAP_NET_ADMIN` only | the kernel, `/dev/net/tun`, systemd-resolved, polkit, logind |

Everything except the optional helper runs with your user's permissions. The helper is only used when you turn on [system-wide names](system-wide-names.md).

## Who can talk to the daemon

`flavord` listens on a Unix socket at `$XDG_RUNTIME_DIR/flavor/flavord.sock`.

- The `flavor` runtime directory is forced to mode `0700` and must be owned by you; the socket is `0600`.
- Every connection's user ID is read from the kernel with `SO_PEERCRED`. A connection from another user ID is closed before any request is read, and requests without a matching peer are rejected with HTTP 403.
- Only one `flavord` runs per user, enforced with a lock file in the runtime directory.

The boundary is your user account. Any program running as you can use the daemon, just as it can read your files. Flavor does not try to defend against malware that already runs as you.

## Network identities and data on disk

Each network you add gets its own embedded Tailscale node ([`tsnet`](https://pkg.go.dev/tailscale.com/tsnet)) with its own node key, state directory and peer list.

| What | Where |
| --- | --- |
| Network list, workspaces, preferences, synthetic address mappings | `~/.local/share/flavor/database/flavor.db` (`0600`) |
| Node identity and state for one network | `~/.local/share/flavor/networks/<network-id>/tsnet/` |
| Desktop settings | `~/.config/flavor/config.toml` |

- Flavor's data and config directories are forced to `0700`, must be owned by you, and must not be symlinks. Network directories are derived from the network ID, so a crafted name cannot point outside them.
- The database has no columns for keys or secrets, and a test enforces that.
- All sessions run inside one `flavord` process. They are kept apart logically (separate keys, state and peers, and devices are always identified by network), but they are not sandboxed from each other. A bug that gives an attacker code execution inside `flavord` exposes every network.
- Removing a network leaves its identity files on disk until you delete them, which is a separate, explicit step (`flavorctl remove --delete-identity`, or the checkbox in the app). Adding a network again creates a new identity; the old files are not reused. Files are deleted normally, not securely erased. Flavor never removes machines from the control server; an administrator has to do that.

## Sign-in and pre-auth keys

- **Pre-auth keys** are read from stdin by `flavorctl enroll` (so they never appear in the process list) or from the desktop app's password field. The daemon validates the key, hands it to the embedded node once and clears it. It is never written to the database, config files, keyring, logs, events or diagnostics. Automated tests plant canary keys and check the daemon's logs, its state and diagnostics responses, its events and every file under its data and config directories.
- **Browser sign-in links** stay in memory. Logs record only the host name. The desktop app opens a link only after you click, and only after the Rust side has fetched the link from the daemon again and checked that it is `http` or `https` with no embedded user name or password. Links from an expired sign-in attempt are refused.
- **The system keyring** is not used to store anything today. Flavor only checks whether it is available, for the diagnostics page.
- **Ambient credentials** are ignored. `flavord` clears `TS_AUTHKEY` and `TS_AUTH_KEY` when it starts, and a node will not start if they are still set, so a key meant for another tool cannot be sent to the wrong control server.

## Telemetry and logs

- Flavor has no telemetry of its own.
- Tailscale's log upload service is turned off for every session (`TS_NO_LOGS_NO_SUPPORT`), and the embedded client's internal logs are discarded.
- `flavord` logs to stderr, which systemd stores in your user journal (`journalctl --user -u flavord`).
- Diagnostics (`flavorctl diag` and the Diagnostics page) contain an allowlisted summary: versions, database schema, keyring state and, per network, the provider, control server, state and device counts. They never include keys or sign-in links.

## The desktop app

The webview that renders the interface is treated as untrusted.

- Its [Tauri capability](../desktop/src-tauri/capabilities/main.json) allows only Flavor's own commands plus event listening. It has no shell, filesystem, HTTP or URL-opener permissions, and no access to forwarding or the SOCKS5 proxy.
- The Content Security Policy only allows Flavor's own bundled scripts, styles and fonts; no remote content is loaded.
- The webview never talks to the daemon socket directly. A CI check fails the build if the frontend ever contains a socket client.
- Text that comes from control servers or peers (names, errors) is rendered as plain text.

## Forwarding and the SOCKS5 proxy

`flavorctl forward` and `flavorctl socks` run inside `flavord` and connect through the chosen network's embedded node. They do not change routes, DNS or firewall rules, and they need no extra privileges.

- They listen on loopback only. Non-loopback listen addresses are rejected.
- Loopback TCP is reachable by every local user, so each accepted connection is matched to its owner through `/proc/net/tcp` and refused unless it belongs to your user ID.
- Every new connection is resolved again with the same decision engine as the Connection Inspector. A destination that exists on several networks is refused unless you name the network or have set a preference. Flavor never picks one at random.
- The SOCKS5 proxy supports `CONNECT` without authentication; the loopback and user ID checks above are what keep it private.
- At most 32 forwards and proxies run at once, with up to 64 connections each.

## System-wide names (experimental helper)

The optional `flavor-netd` helper is the only part of Flavor that runs as root. In short:

- It can create one TUN interface per host (`flv-u<uid>`), add routes for Flavor's own synthetic address ranges only, and point systemd-resolved at Flavor for DNS names under `flavor.internal`. `flavord` asks for no other DNS domains; the helper would accept other validated suffixes, but never the root domain or special-use names such as `local` and `arpa`. It never edits `/etc/resolv.conf`, firewall rules or sysctls, and never routes your real tailnet ranges.
- Its socket is open to every local user so that callers reach the authorization check; connecting starts the helper, but it changes nothing without the authorization below.
- Every change needs polkit authorization (action `dev.lunarlabs.flavor.netd.manage-interface`, allowed for active local sessions without a password prompt) and an active, local (not SSH) login session for the calling user.
- It runs with only `CAP_NET_ADMIN`, under systemd sandboxing, and is confined by an AppArmor profile or SELinux module when those are active.
- Everything it created is removed when `flavord` exits or your session becomes inactive.
- Because routes and DNS settings apply to the whole host, other local users could reach your networks through it. It is supported on single-user machines only.

Details: [System-wide names](system-wide-names.md).

## Releases and dependencies

- Rebuilding a release commit with the same toolchain versions produces byte-identical binaries and tarballs. The release workflow builds with Go from `go.mod` but takes the current stable Rust and Node 24, so outside rebuilds need matching versions.
- Each release has a `SHA256SUMS` file covering every tarball and SBOM. It is signed twice, independently: by the release workflow with Sigstore (cosign), and with the maintainer's GPG key. Each tarball also has a GitHub build-provenance attestation from the workflow. See [Verify the download](install.md#verify-the-download).
- Each tarball ships an SPDX SBOM listing its Go modules, Rust crates and bundled npm packages. A source SBOM covers the whole repository.
- Dependency versions are locked (`go.sum`, `Cargo.lock`, `desktop/package-lock.json`). The Tailscale client library is pinned to `tailscale.com v1.90.9`, and upgrades are deliberate.

## What Flavor does not protect against

- Malware or other programs running as your user.
- Code execution inside `flavord`, which reaches every network's keys and state.
- Local root, or anyone who can read your home directory.
- A malicious or compromised control server, beyond validating and bounding what it sends. Like any Tailscale client, the control server decides which peers and routes your node sees.
- Recovery of deleted identity files from disk.
- Other users on a shared machine when system-wide names are enabled (see above).
