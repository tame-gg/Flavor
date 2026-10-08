# Install, verify, upgrade and uninstall

Flavor is a beta for Linux on x86_64 and arm64. There are three ways to install it:

- [Release tarball](#release-tarball): any systemd-based distribution. Installs to `/usr` and `/etc` with an installer script.
- [Arch Linux](#arch-linux): build a pacman package from the PKGBUILDs in this repository.
- [From source](#from-source): build it yourself, optionally without root.

## Requirements

| | Requirement |
| --- | --- |
| Architecture | x86_64 (`amd64`) or arm64 (`aarch64`) |
| System | Linux with systemd. The daemon runs as a systemd user service. |
| Desktop app | glibc 2.39 or newer, WebKitGTK 4.1 and libayatana-appindicator (tray icon). Ubuntu 24.04, Debian 13 and Fedora 40 or newer, and Arch Linux ship a new enough glibc. |
| Control servers | A Tailscale account, or a Headscale server you can reach |
| [System-wide names](system-wide-names.md) (optional) | systemd-resolved, polkit, and a computer with a single user |

Desktop libraries by distribution:

| Distribution | Packages |
| --- | --- |
| Debian, Ubuntu | `libwebkit2gtk-4.1-0` `libayatana-appindicator3-1` |
| Fedora | `webkit2gtk4.1` `libayatana-appindicator-gtk3` |
| Arch Linux | `webkit2gtk-4.1` `libayatana-appindicator` (installed automatically by the PKGBUILDs) |

`flavord` and `flavorctl` are static binaries and do not need these libraries.

### Does Flavor need root?

Running Flavor does not. The daemon, the desktop app and the CLI all run as your user. Root is only needed for:

- **Installing system-wide.** The tarball installer and pacman write to `/usr` and `/etc`. You can avoid this by [building from source](#from-source) into your home directory.
- **The optional [system-wide names](system-wide-names.md) helper**, `flavor-netd`. It runs as root with only `CAP_NET_ADMIN` and is started on demand by a systemd socket. It makes no changes unless an authorized user turns the feature on.

## Release tarball

Each [release](https://github.com/tame-gg/Flavor/releases) has one tarball per architecture with the daemon, CLI, desktop app, the optional helper, its systemd units, polkit policy, SELinux module and AppArmor profile, plus an installer. It is meant for recent systemd-based distributions.

```bash
v=0.1.0-beta.2
base=https://github.com/tame-gg/Flavor/releases/download/v$v
curl -fLO $base/flavor-$v-linux-amd64.tar.gz -fLO $base/SHA256SUMS -fLO $base/SHA256SUMS.asc -fLO $base/SHA256SUMS.sigstore.json

sha256sum --check --ignore-missing SHA256SUMS
```

Check one of the signatures on `SHA256SUMS` as described in [Verify the download](#verify-the-download), then install:

```bash
tar -xzf flavor-$v-linux-amd64.tar.gz
sudo ./flavor-$v-linux-amd64/install.sh
systemctl --user enable --now flavord
```

On arm64, replace `amd64` with `arm64`. Then open **Flavor** from your application menu, or run `flavor-desktop`. Continue with [Getting started](getting-started.md).

### What the installer does

`install.sh` must run as root. It:

1. Removes an existing [Lattice](#upgrading-from-lattice) installation, keeping your data.
2. Copies the files listed in the tarball's manifest to `/usr` and `/etc`, refusing any path outside those two directories.
3. Loads the AppArmor profile if AppArmor is enabled, or installs the SELinux module if SELinux is enabled.
4. Enables `flavor-netd.socket`. The helper behind it starts when a program connects to the socket, and changes nothing unless a user with an active local session is authorized by polkit, which happens only when `flavord` runs with system-wide names turned on.
5. Restarts `flavord` for logged-in users who already run it, so an upgrade takes effect.

It does not enable `flavord` for anyone; each user does that once with `systemctl --user enable --now flavord`.

Installed files:

| Path | What |
| --- | --- |
| `/usr/bin/flavord` | daemon |
| `/usr/bin/flavorctl` | command-line client |
| `/usr/bin/flavor-desktop` | desktop app |
| `/usr/lib/systemd/user/flavord.service` | user service for the daemon |
| `/usr/libexec/flavor/flavor-netd` | optional privileged helper |
| `/usr/lib/systemd/system/flavor-netd.socket`, `.service` | helper socket and service |
| `/usr/share/polkit-1/actions/dev.lunarlabs.flavor.netd.policy` | polkit policy for the helper |
| `/etc/apparmor.d/flavor-netd` | AppArmor profile for the helper |
| `/usr/share/selinux/packages/flavor/` | SELinux module and its sources |
| `/usr/share/applications/dev.lunarlabs.flavor.desktop`, `/usr/share/icons/hicolor/128x128/apps/dev.lunarlabs.flavor.png` | application menu entry and icon |
| `/usr/share/flavor/frontend/package-lock.json` | the npm packages bundled into the desktop app, for SBOM tools |
| `/usr/share/flavor/manifest`, `/usr/libexec/flavor/uninstall` | file list and uninstaller |

## Verify the download

`SHA256SUMS` lists the SHA-256 checksum of every tarball and SBOM in the release. Check your tarball against it:

```console
$ sha256sum --check --ignore-missing SHA256SUMS
flavor-0.1.0-beta.2-linux-amd64.tar.gz: OK
```

Then confirm that `SHA256SUMS` itself is genuine, with either the Sigstore signature or the GPG signature. The two are independent: one comes from the release workflow, the other from the maintainer's key. GitHub build provenance checks the tarball directly instead.

### Sigstore (cosign)

`SHA256SUMS.sigstore.json` is a keyless signature made by the release workflow. It proves the file was produced by this repository's `release.yml` for a version tag.

```console
$ cosign verify-blob --bundle SHA256SUMS.sigstore.json \
    --certificate-identity-regexp '^https://github.com/tame-gg/Flavor/\.github/workflows/release\.yml@refs/tags/v' \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com SHA256SUMS
Verified OK
```

Tested with cosign 3.1.

### GitHub build provenance

Each tarball has a build-provenance attestation that ties it to the workflow run that built it. It covers the tarballs only, not `SHA256SUMS` or the SBOMs. Checking it needs the [GitHub CLI](https://cli.github.com), signed in to any GitHub account:

```bash
gh attestation verify flavor-$v-linux-amd64.tar.gz --repo tame-gg/Flavor
```

### GPG

`SHA256SUMS.asc` is signed by the maintainer's release key:

```
0DFC 4321 62BF 84C0 FD78  0619 FBB9 6BCE C036 1F01
```

The key is published with the maintainer's GitHub account, together with their other keys:

```bash
curl -fsSL https://github.com/ohemilyy.gpg | gpg --import
gpg --verify SHA256SUMS.asc SHA256SUMS
```

The output should say `Good signature` and show the fingerprint above. A warning that the key is not certified with a trusted signature only means you have not marked the key as trusted in your own keyring.

### SBOMs

Each release also has SPDX SBOMs, covered by `SHA256SUMS`:

- `flavor-<version>-linux-<arch>.spdx.json` lists what that tarball ships: the Go modules in the daemon, CLI and helper, the Rust crates in the desktop app (embedded with [cargo-auditable](https://github.com/rust-secure-code/cargo-auditable)), and the npm packages bundled into the desktop interface.
- `flavor-<version>-source.spdx.json` covers the whole repository.

## Arch Linux

The repository contains two PKGBUILDs. They are not on the AUR yet, so build them from a clone:

| Package | What it does |
| --- | --- |
| [`flavor-bin`](../packaging/aur/flavor-bin) | Repackages the signed release tarball. Checks the GPG signature on `SHA256SUMS` and the tarball's checksum. |
| [`flavor`](../packaging/aur/flavor) | Builds from the signed release tag with Go, Rust and npm. Takes a few minutes. |

```bash
git clone https://github.com/tame-gg/Flavor.git
cd Flavor/packaging/aur/flavor-bin
curl -fsSL https://github.com/ohemilyy.gpg | gpg --import
makepkg -si
systemctl --user enable --now flavord
```

For the source build, use `packaging/aur/flavor` instead. `makepkg` verifies the GPG signature, which is why the key is imported first.

Following Arch conventions, the packages do not enable any service. For [system-wide names](system-wide-names.md), also run `sudo systemctl enable --now flavor-netd.socket`. On systems with AppArmor enabled, the packages load the helper's profile when they are installed.

## From source

Requirements: Go 1.27.1 or newer, Rust (stable), Node.js 24 or newer with npm, and the desktop libraries' development packages. On Debian or Ubuntu those are `libwebkit2gtk-4.1-dev`, `libayatana-appindicator3-dev` and `librsvg2-dev`.

To install into your home directory without root (no helper, so no system-wide names):

```bash
git clone https://github.com/tame-gg/Flavor.git && cd Flavor
./scripts/go.sh build -o ~/.local/bin/flavord ./cmd/flavord
./scripts/go.sh build -o ~/.local/bin/flavorctl ./cmd/flavorctl
install -Dm644 packaging/systemd/user/flavord.service ~/.config/systemd/user/flavord.service
systemctl --user daemon-reload
systemctl --user enable --now flavord

(cd desktop && npm ci && npm run tauri build)
./target/release/flavor-desktop
```

Always build Go code through `./scripts/go.sh`: it applies settings from [`go.env`](../go.env) that the pinned Tailscale library needs.

To build a release tarball, run `./scripts/release.sh VERSION [amd64|arm64]`. It needs `jq` and [cargo-auditable](https://github.com/rust-secure-code/cargo-auditable) (`cargo install cargo-auditable --locked`), which records the desktop app's crate list for the SBOM. The release workflow also passes `SELINUX_PP=<path>` with the SELinux module it builds on Fedora; without it, the tarball has the module's sources but no compiled `flavor_netd.pp`. With the same toolchain versions, rebuilding a commit produces identical files. Set `DESKTOP=0` to skip the desktop app, for example to cross-build the daemon for another architecture.

For a development setup, see [CONTRIBUTING.md](../CONTRIBUTING.md).

## Upgrade

- **Tarball:** download the new tarball and run its `install.sh`. It replaces the files in place and restarts `flavord` for logged-in users who run it.
- **Arch Linux:** pull the repository and run `makepkg -si` again.

Your networks, device identities, workspaces and preferences are kept.

## Upgrading from Lattice

Flavor was called Lattice until v0.1.0-beta.2. Install Flavor the same way, then enable `flavord`:

- `install.sh` runs Lattice's uninstaller first. It stops `latticed` and removes Lattice's files, but keeps your data.
- On its first start, `flavord` moves `~/.local/share/lattice` and `~/.config/lattice` to the `flavor` paths. That includes the database, your networks and every device identity, so networks reconnect without signing in again. Lattice never stored credentials in the system keyring, so nothing needs to move there.
- While there is Lattice data to move, `flavord` refuses to start if `latticed` is still running. If `~/.local/share/flavor` already exists, it leaves the Lattice data alone and logs a warning.
- A `systemctl --user edit latticed` override, such as the system-wide names setting, is not carried over. Recreate it with `systemctl --user edit flavord`.

## Uninstall

Tarball installation:

```bash
sudo /usr/libexec/flavor/uninstall
```

It stops `flavord` for logged-in users and stops the helper, removes any `flv-u*` interface, unloads the SELinux module and AppArmor profile, and deletes every installed file. Arch Linux packages: `sudo pacman -Rns flavor-bin` (or `flavor`).

Either way, your own data is kept. To delete it too:

```bash
rm -rf ~/.local/share/flavor ~/.config/flavor
```

> [!WARNING]
> Deleting `~/.local/share/flavor` deletes this computer's identity on every network. The machines stay registered on their control servers until an administrator removes them.

## Tested environments

| Environment | What was tested | Version |
| --- | --- | --- |
| Ubuntu 24.04, GitHub Actions | builds, Go, Rust and frontend test suites | every commit |
| Ubuntu 24.04 container | `install.sh` and `uninstall` file layout, upgrading over Lattice v0.1.0-beta.1 | v0.1.0-beta.2 |
| Arch Linux container | `flavor-bin` and `flavor` packages: build, install, run `flavord` and `flavorctl`, remove | v0.1.0-beta.2 |
| Arch-based desktop | daemon and CLI against two real Headscale 0.26.1 servers; migration from Lattice with a real node | v0.1.0-beta.2 |
| Ubuntu 26.04 VM (AppArmor) and Fedora 43 VM (SELinux) | full install, system-wide names through the helper, upgrade, uninstall | pre-release build of v0.1.0-beta.1, as Lattice |

Live testing against Tailscale's hosted coordination server is still pending; the automated acceptance tests currently run against Headscale.

Older distributions are untested. Debian 12 and Ubuntu 22.04, for example, are too old for the desktop app, and their AppArmor 3 may refuse the helper's profile, which declares AppArmor ABI 4.0; `install.sh` stops at the first error.
