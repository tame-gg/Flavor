# Flavor documentation

Flavor is an open-source desktop app, daemon and CLI for using several Tailscale and Headscale networks at the same time on Linux. Start with the [project README](../README.md) for an overview.

## Using Flavor

| Guide | What it covers |
| --- | --- |
| [Install, verify, upgrade and uninstall](install.md) | Requirements, the release tarball, signature checks, Arch Linux packages, building from source, upgrading from Lattice, uninstalling |
| [Getting started](getting-started.md) | Adding a Tailscale account or Headscale server, signing in, finding devices, connecting to a service |
| [Names, decisions and connections](networking.md) | How Flavor tells overlapping addresses apart, Flavor names, the decision rules, Connection Inspector, Conflict Center, preferences, port forwarding, SOCKS5 and workspaces |
| [flavorctl reference](cli.md) | Every command and option, with example output |
| [System-wide names (experimental)](system-wide-names.md) | The optional root helper: what it changes, how it is authorized and confined, and how to turn it on and off |
| [Troubleshooting](troubleshooting.md) | Logs, diagnostics and fixes for common problems |

## How it works

| Page | What it covers |
| --- | --- |
| [Security model](security.md) | Privileges, local access control, credentials, logs, the webview, the helper and release integrity |
| [Architecture](architecture.md) | Components, IPC, UI synchronization, data locations and repository layout |

## Project

- [Contributing](../CONTRIBUTING.md): development setup, tests and pull requests
- [Security policy](../SECURITY.md): how to report a vulnerability
- [Code of conduct](../CODE_OF_CONDUCT.md)
