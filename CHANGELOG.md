# Changelog

All notable changes to Flavor are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

[0.1.0-beta.3]: https://github.com/tame-gg/Flavor/compare/v0.1.0-beta.2...v0.1.0-beta.3
[0.1.0-beta.2]: https://github.com/tame-gg/Flavor/compare/v0.1.0-beta.1...v0.1.0-beta.2
[0.1.0-beta.1]: https://github.com/tame-gg/Flavor/releases/tag/v0.1.0-beta.1
