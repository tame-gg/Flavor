# Changelog

All notable changes to Flavor are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- You can pick an exit node per network with `flavorctl exit-node list`, `set <network> <device>` and `clear <network>`. Destinations that match no device or route on any network, such as `203.0.113.7` or `example.com`, now leave through the exit node in `flavorctl forward` and the SOCKS5 proxy. Tailnet addresses and Flavor names never do. When two networks have an exit node, such destinations are ambiguous until `--network` or a preference picks one. `flavorctl devices` shows an `EXIT` column. Advertising routes and running an exit node are not covered yet ([#43](https://github.com/tame-gg/Flavor/issues/43)).
- Names that a network's DNS settings publish, such as Headscale `extra_records`, now resolve in `flavorctl explain`, `flavorctl forward`, the SOCKS5 proxy and the Connection Inspector, and they point at the network that published them. A name that two networks both publish is ambiguous, as for devices, and `--network` or a preference picks one. Split DNS and search domains are not covered yet ([#47](https://github.com/tame-gg/Flavor/issues/47)).

## [0.1.0-beta.7] - 2026-10-10

### Fixed

- The Windows installer lists the publisher as tame-gg instead of lunarlabs. When you upgrade from v0.1.0-beta.6 by running the installer, it can't uninstall the old version first and says so; choose to install without uninstalling and continue. Silent upgrades, such as through winget, aren't affected.

## [0.1.0-beta.6] - 2026-10-09

### Added

- Windows support on x64 and ARM64 ([#32](https://github.com/tame-gg/Flavor/issues/32)):
  - `flavord` and `flavorctl` run on Windows. They talk over a named pipe that only your user account can open, and each side checks that the other runs as you. Port forwarding and the SOCKS5 proxy accept only connections from your own user, as on Linux ([#33](https://github.com/tame-gg/Flavor/pull/33)).
  - `flavord` runs without a console window and logs to `%LOCALAPPDATA%\flavor\flavord.log` ([#34](https://github.com/tame-gg/Flavor/pull/34)).
  - The desktop app runs on Windows, with a tray icon in the notification area ([#35](https://github.com/tame-gg/Flavor/pull/35), [#39](https://github.com/tame-gg/Flavor/pull/39)).
  - Releases include a per-user installer and a zip for each architecture, covered by the same SBOMs, checksums, signatures and attestations as the other downloads. The installer needs no administrator rights, starts `flavord` at sign-in, and is not code-signed yet ([#36](https://github.com/tame-gg/Flavor/pull/36)). The [Windows guide](https://github.com/tame-gg/Flavor/wiki/Windows) covers installing, upgrading and uninstalling.
  - On Windows, Flavor keeps its data and settings in `%LOCALAPPDATA%\flavor`, and `flavorctl diag` reports Windows Credential Manager as the secret store.
- `flavord --log-file PATH` appends the daemon log to a file instead of standard error, on every platform.
- CI runs the Go and Rust test suites on Windows (x64 and ARM64), and every release build installs, upgrades and uninstalls the Windows installer before publishing ([#37](https://github.com/tame-gg/Flavor/pull/37), [#40](https://github.com/tame-gg/Flavor/pull/40)).

### Fixed

- `flavord` on Windows accepts data folders owned by the Administrators group or SYSTEM instead of refusing to start with `unexpected owner` ([#38](https://github.com/tame-gg/Flavor/pull/38)).

## [0.1.0-beta.5] - 2026-10-09

### Added

- The daemon log records connect and disconnect requests per network, forward listeners starting and stopping with their network and target, SOCKS5 listeners starting and stopping, refused connections with the reason, including connections from another local user and failed upstream dials, and, at debug level, each connection opening and closing ([#20](https://github.com/tame-gg/Flavor/issues/20)).
- A [Homebrew tap](https://github.com/tame-gg/homebrew-tap) for macOS: `brew install tame-gg/tap/flavor` for the daemon and CLI, and `brew install --cask tame-gg/tap/flavor-desktop` for the app.

### Changed

- A Headscale control server address without a scheme, such as `vpn.example.com`, now means `https://vpn.example.com` instead of failing with `INVALID_CONTROL_URL`. `http://` is never assumed ([#18](https://github.com/tame-gg/Flavor/issues/18)).

### Fixed

- tsnet lines in the debug log show their values again instead of raw format strings. Sign-in links, auth keys and other key material are replaced with `[REDACTED]` ([#19](https://github.com/tame-gg/Flavor/issues/19)).

## [0.1.0-beta.4] - 2026-10-08

### Added

- macOS support on Intel and Apple Silicon ([#15](https://github.com/tame-gg/Flavor/issues/15)):
  - `flavord` and `flavorctl` run on macOS. The IPC socket accepts only your own user, and port forwarding and the SOCKS5 proxy accept only connections from your own user, as on Linux ([#17](https://github.com/tame-gg/Flavor/pull/17)).
  - The desktop app runs on macOS, with a menu bar icon that follows light and dark mode. With close-to-tray, clicking the app in the Dock shows the window again ([#22](https://github.com/tame-gg/Flavor/pull/22)).
  - A LaunchAgent starts `flavord` at login ([#21](https://github.com/tame-gg/Flavor/pull/21)).
  - Releases include `darwin-amd64` and `darwin-arm64` tarballs with `flavord`, `flavorctl`, `Flavor.app` and the LaunchAgent, covered by the same SBOMs, checksums, signatures and attestations as the Linux tarballs ([#23](https://github.com/tame-gg/Flavor/pull/23)). The app is ad-hoc signed but not notarized yet. The [macOS guide](https://github.com/tame-gg/Flavor/wiki/macOS) shows how to open it.
  - On macOS, Flavor keeps its data, settings and socket in `~/Library/Application Support/flavor`, and `flavorctl diag` reports the Keychain as the secret store.
- CI runs the Go and Rust test suites on macOS (Intel and Apple Silicon).

### Changed

- `flavord --synthetic-helper` stops at startup with a clear error on platforms other than Linux. System-wide names remain Linux-only.

### Fixed

- The `flavor` AUR package no longer builds an empty `flavor-debug` package ([#16](https://github.com/tame-gg/Flavor/pull/16)).

## [0.1.0-beta.3] - 2026-10-08

### Added

- `flavorctl --version`. Both `flavorctl --version` and `flavord --version` now print the version, the build commit and the protocol, for example `flavord 0.1.0-beta.3 (commit 1a2b3c4) protocol 1.3` ([#4](https://github.com/tame-gg/Flavor/issues/4)).
- Arch Linux packages on the AUR: [`flavor-bin`](https://aur.archlinux.org/packages/flavor-bin) repackages the signed release and [`flavor`](https://aur.archlinux.org/packages/flavor) builds from the signed tag.
- Documentation in the [project wiki](https://github.com/tame-gg/Flavor/wiki): install and verification, getting started, names and routing decisions, the `flavorctl` reference, system-wide names, troubleshooting, the security model and the architecture.
- `SECURITY.md` with private vulnerability reporting, `CONTRIBUTING.md`, a code of conduct, and issue and pull request templates.

### Changed

- `flavord`, `flavorctl` and `flavor-netd` are built as position-independent executables with full RELRO, and the desktop binary is stripped of its symbol table ([#1](https://github.com/tame-gg/Flavor/issues/1)). The Go binaries now need the system's dynamic loader, but still no shared libraries.
- The release workflow pins every GitHub Action to a commit SHA, runs on Node 24 actions, and builds without the Go and npm caches ([#2](https://github.com/tame-gg/Flavor/issues/2)).
- CI runs on Ubuntu 24.04, the same image as the release builds ([#3](https://github.com/tame-gg/Flavor/issues/3)).

### Fixed

- `scripts/generate-proto.sh` stops if `go env` fails instead of continuing with a broken `PATH` ([#5](https://github.com/tame-gg/Flavor/issues/5)).

## [0.1.0-beta.2] - 2026-10-08

### Changed

- The project is renamed from Lattice to Flavor. Binaries, paths, the polkit action, the SELinux and AppArmor policies, Flavor names (`*.flavor.internal`) and environment variables all use the new name.

### Added

- Upgrading from Lattice keeps everything: `install.sh` removes the Lattice installation first, and `flavord` moves `~/.local/share/lattice` and `~/.config/lattice` to the Flavor paths on its first start, including the database and every device identity.

## [0.1.0-beta.1] - 2026-10-08

First public beta, released as Lattice.

### Added

- Several Tailscale and Headscale networks at once, each as its own embedded Tailscale node with its own identity, managed by a per-user daemon (`flavord`), a desktop app and a CLI (`flavorctl`).
- One device list across networks, the Connection Inspector, the Conflict Center, network-qualified names, destination preferences and workspaces.
- Local port forwarding and a SOCKS5 proxy that reach services on a chosen network without root.
- Experimental system-wide names through the optional `flavor-netd` helper, confined by systemd sandboxing, polkit, AppArmor and SELinux.
- Release tarballs for x86_64 and arm64 with an installer and uninstaller, SBOMs, and `SHA256SUMS` signed with Sigstore and GPG, plus GitHub build-provenance attestations.

[0.1.0-beta.7]: https://github.com/tame-gg/Flavor/compare/v0.1.0-beta.6...v0.1.0-beta.7
[0.1.0-beta.6]: https://github.com/tame-gg/Flavor/compare/v0.1.0-beta.5...v0.1.0-beta.6
[0.1.0-beta.5]: https://github.com/tame-gg/Flavor/compare/v0.1.0-beta.4...v0.1.0-beta.5
[0.1.0-beta.4]: https://github.com/tame-gg/Flavor/compare/v0.1.0-beta.3...v0.1.0-beta.4
[0.1.0-beta.3]: https://github.com/tame-gg/Flavor/compare/v0.1.0-beta.2...v0.1.0-beta.3
[0.1.0-beta.2]: https://github.com/tame-gg/Flavor/compare/v0.1.0-beta.1...v0.1.0-beta.2
[0.1.0-beta.1]: https://github.com/tame-gg/Flavor/releases/tag/v0.1.0-beta.1
