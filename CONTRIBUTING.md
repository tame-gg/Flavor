# Contributing to Flavor

Thanks for your interest in Flavor. Bug reports, documentation fixes, testing on more distributions and code are all welcome.

- **Found a bug?** [Open a bug report](https://github.com/tame-gg/Flavor/issues/new?template=bug_report.yml).
- **Have an idea?** [Open a feature request](https://github.com/tame-gg/Flavor/issues/new?template=feature_request.yml). For larger changes, please open an issue before writing code, so the approach can be agreed first.
- **Found a security problem?** Report it privately as described in [SECURITY.md](SECURITY.md), not in a public issue.

## How the project is organized

Flavor is a Go daemon (`flavord`), a Go CLI (`flavorctl`), a desktop app (Rust and Tauri with a React interface) and an optional Go helper (`flavor-netd`). The [Architecture](https://github.com/tame-gg/Flavor/wiki/Architecture) wiki page explains how they fit together and where each part lives.

`main` is the stable branch: every commit on it should be releasable. Work happens on branches and lands through pull requests.

## Development setup

You need Linux. Install:

- Go 1.27.1 or newer
- Rust (stable) with Cargo
- Node.js 24 or newer with npm
- The desktop app's libraries with development headers. On Debian or Ubuntu: `libwebkit2gtk-4.1-dev libayatana-appindicator3-dev librsvg2-dev`. On Arch Linux: `webkit2gtk-4.1 libayatana-appindicator`.

Optional:

- [buf](https://buf.build/docs/cli/installation/) and network access, to regenerate API code after changing `.proto` files
- Docker with Compose, and Python 3, for the Headscale integration and acceptance tests
- [cargo-auditable](https://github.com/rust-secure-code/cargo-auditable), for release builds

Always run Go through `./scripts/go.sh`. It applies [`go.env`](go.env), which sets build options the pinned Tailscale library needs.

### Run Flavor from source

The daemon, the desktop app and `flavorctl` all find each other through `$XDG_RUNTIME_DIR/flavor/flavord.sock`, and only one daemon can run per user. If Flavor is installed, stop it first, and give the development daemon its own data directory so it cannot touch your real networks:

```bash
systemctl --user stop flavord

dev=$(mktemp -d)
XDG_DATA_HOME=$dev/data XDG_CONFIG_HOME=$dev/config ./scripts/go.sh run ./cmd/flavord
```

No tailnet handy? `test/fakedaemon` serves the same API with simulated networks and devices, including overlapping addresses and subnet routes. Control URLs containing `login` or `approval` simulate those sign-in states:

```bash
XDG_DATA_HOME=$dev/data XDG_CONFIG_HOME=$dev/config ./scripts/go.sh run ./test/fakedaemon
```

In another terminal, start the desktop app with hot reload, or use the CLI:

```bash
cd desktop
npm ci
npm run tauri dev
```

```bash
./scripts/go.sh run ./cmd/flavorctl list
```

## Tests

Run these before opening a pull request. CI runs them for pull requests and pushes to `main`.

```bash
./scripts/go.sh vet ./...
./scripts/go.sh test ./...
(cd desktop && npm ci && npm run build && npm test)
cargo test --workspace
```

- The Go tests cover the services, sessions, IPC, the decision engine and the helper, which is tested against fakes.
- `npm run build` type-checks and builds the interface into `desktop/dist`; `npm test` runs the frontend tests.
- The Rust tests run the IPC client against a real daemon and build `test/fakedaemon` with Go. They also compile the desktop app, which needs `desktop/dist` to exist, so build the frontend first.

These tests are safe to run on your workstation. They need no root, never create network interfaces and never change routes, DNS settings or other system state. Some tests read kernel state without changing it (netlink route dumps, `/proc`). `TestIdentityComesFromTheKernel` needs Linux 6.5 or newer, because it checks pidfd-based peer credentials.

### Changing the API

The daemon API lives in [`proto/flavor/v1`](proto/flavor/v1) and the helper protocol in [`proto/flavor/netd/v1`](proto/flavor/netd/v1). After editing a `.proto` file, regenerate the Go, TypeScript and Rust bindings and commit them with your change:

```bash
./scripts/generate-proto.sh
```

CI fails if the generated files are out of date.

### Integration and acceptance tests

These run real Headscale servers in Docker. The integration test checks a single session against Headscale:

```bash
docker compose -f test/integration/headscale/compose.yml up -d headscale-a
export FLAVOR_INTEGRATION=1
export FLAVOR_HEADSCALE_URL=http://127.0.0.1:18080
export FLAVOR_HEADSCALE_AUTHKEY="$(./test/integration/headscale/scripts/create-preauth-key.sh a flavor)"
./scripts/go.sh test ./internal/session -run TestIntegrationHeadscale -count=1 -v
```

The acceptance test builds the real `flavord` and `flavorctl`, connects two Headscale networks at once, restarts the daemon and checks that sessions come back without keys, that removing and deleting identities behave as documented, and that no key reaches the logs or disk:

```bash
docker compose -f test/integration/headscale/compose.yml --profile dual up -d
./test/acceptance/headscale-concurrent.sh
```

See [test/integration/headscale/README.md](test/integration/headscale/README.md) for details. Ordinary `go test ./...` skips the integration test unless `FLAVOR_INTEGRATION` is set.

### The privileged helper and the installer

`packaging/install.sh` writes to `/usr` and `/etc` and enables a system service, and `flavor-netd` changes host networking. Do not try them on your workstation. Use a disposable virtual machine. The helper's unit tests use fake kernel, polkit, logind and systemd-resolved backends; the manual validation done so far is described in [System-wide names](https://github.com/tame-gg/Flavor/wiki/System-Wide-Names#testing-status).

### Release builds

```bash
./scripts/release.sh 0.1.0-beta.5 amd64
```

builds a release tarball. It needs `jq` and cargo-auditable; the release workflow also passes `SELINUX_PP` with the SELinux module it builds on Fedora. With the same toolchain versions, two clean builds of a commit produce identical files; if you change the build, check that this still holds.

## Code conventions

- Match the code around your change: its naming, structure and error handling.
- Format Go with `gofmt` and Rust with `rustfmt`, using their defaults.
- The codebase does not use code comments. Express intent through names, small functions and tests.
- Keep the webview unprivileged: the React interface must not talk to the daemon socket directly. CI checks this.
- Never log secrets or full sign-in links. Wrap credentials in `secret.Secret`, which redacts itself, and log URLs through the helpers in `internal/logging`.
- Add or update tests for behavior changes, and update the [README](README.md) and the [wiki](https://github.com/tame-gg/Flavor/wiki) when user-facing behavior changes.

## Pull requests

1. Create a branch from `main`.
2. Keep each pull request to one topic.
3. Write the title like a commit message: lowercase, starting with `feat:`, `fix:` or `chore:`, for example `fix: refuse ambiguous socks destinations`. Pull requests are usually squash-merged, so the title becomes the commit message.
4. Fill in the pull request template, including how you tested the change.
5. CI must pass before merging.

## License

Flavor is licensed under the [MIT License](LICENSE). By contributing, you agree that your contributions are licensed under the same terms.
