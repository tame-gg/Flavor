# flavorctl reference

`flavorctl` is Flavor's command-line client. It talks to the same per-user daemon (`flavord`) as the desktop app, so everything you do in one shows up in the other. Run it without arguments to print this usage:

```text
usage: flavorctl [--runtime-dir DIR] [--json] <command> [args]

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
  socks [--listen ADDR]                local SOCKS5 proxy for Flavor destinations (default 127.0.0.1:1080)
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
```

The examples on this page use the example networks from [Names, decisions and connections](networking.md).

## Global options

| Option | Meaning |
| --- | --- |
| `--json` | Print the daemon's response as JSON (protobuf JSON mapping) instead of a table. Supported by `info`, `list`, `devices`, `diag`, `explain`, `conflicts`, `workspace list` and `preference list`. |
| `--runtime-dir DIR` | Look for the daemon socket in `DIR/flavor/` instead of `$XDG_RUNTIME_DIR/flavor/`. Use the same value you gave `flavord --runtime-dir`. |

Global options go before the command: `flavorctl --json explain 100.64.0.3`.

If the daemon is not running, commands fail with `flavorctl: the Flavor daemon is not running`. Errors from the daemon print a short message followed by an error code in parentheses. `flavorctl` exits with status 1 on errors and 2 when run without a command.

## Daemon

### info

Prints the daemon version, build commit, IPC protocol version and the ID of the running daemon instance.

```console
$ flavorctl info
flavord 0.1.0-beta.2 (commit 0de2000be4098579e451c36e3a8f7f61d88967ef)
protocol 1.3
instance 9f2c41d07a5e4b6c8d13e27f50a9b3c6
```

### diag

A safe diagnostics summary: daemon version, database schema, keyring status and one line per network. It never includes keys or sign-in links, so it is suitable for bug reports.

```console
$ flavorctl diag
ok  daemon                              version 0.1.0-beta.2, protocol 1.3
ok  database                            schema version 4
ok  secret_store                        backend secret-service, state available
ok  network/01J9X4T6K8M2Q7R3V5W0YBZCDE  headscale, control https://hs.home.example.net, state connected, 7 devices, 2 local addresses
ok  network/01J9X4T6K8N3R8S4W6X1ZCA2FG  tailscale, control Tailscale, state connected, 8 devices, 2 local addresses
ok  network/01J9X4T6K9P4S9T5X7Y2A3BHJK  headscale, control https://vpn.acme.example, state connected, 6 devices, 2 local addresses
ok  network/01J9X4T6KAQ5T0V6Y8Z3B4CMNP  headscale, control https://vpn.studio.example.com, state authenticating, 0 devices, 0 local addresses
```

## Networks

### add

Adds a network and prints its ID. Without `--headscale`, the network uses Tailscale's hosted coordination server.

```console
$ flavorctl add --name "Home lab" --headscale https://hs.home.example.net --auto-connect
01J9X4T6K8M2Q7R3V5W0YBZCDE
$ flavorctl add --name Work --auto-connect
01J9X4T6K8N3R8S4W6X1ZCA2FG
```

- `--name` is the display name. It also determines the network part of Flavor names (`Home lab` becomes `home-lab`).
- `--headscale URL` is the Headscale server address, starting with `https://` or `http://`, without a user name or password.
- `--auto-connect` connects the network whenever `flavord` starts.

Adding a network does not connect it. Run `connect`, or `enroll` if you have a pre-auth key.

### connect, disconnect

```console
$ flavorctl connect 01J9X4T6K8N3R8S4W6X1ZCA2FG
$ flavorctl disconnect 01J9X4T6K8N3R8S4W6X1ZCA2FG
```

When a network needs browser sign-in, `list` shows its sign-in link. Open it in a browser, finish signing in, and the network connects on its own.

### enroll

Joins a network with a pre-auth key read from stdin, so the key never appears in your shell history or the process list:

```console
$ printf '%s\n' "$PREAUTH_KEY" | flavorctl enroll 01J9X4T6K8M2Q7R3V5W0YBZCDE
```

The key is used once and never stored.

### list

```console
$ flavorctl list
ID                          NAME          PROVIDER   STATE           AUTO
01J9X4T6K8M2Q7R3V5W0YBZCDE  Home lab      headscale  connected       true
01J9X4T6K8N3R8S4W6X1ZCA2FG  Work          tailscale  connected       true
01J9X4T6K9P4S9T5X7Y2A3BHJK  Acme staging  headscale  connected       false
01J9X4T6KAQ5T0V6Y8Z3B4CMNP  Studio        headscale  authenticating  false
sign in to Studio: https://vpn.studio.example.com/register/…
```

Networks are listed in the order they were added.

### rename

Changes the display name. The device keeps its identity; only the friendly part of its Flavor names changes.

```console
$ flavorctl rename 01J9X4T6K8M2Q7R3V5W0YBZCDE "Homelab"
```

### remove

```console
$ flavorctl remove 01J9X4T6K9P4S9T5X7Y2A3BHJK
$ flavorctl remove --delete-identity 01J9X4T6K9P4S9T5X7Y2A3BHJK
```

`remove` disconnects the network and removes it from Flavor, but leaves this device's identity files on disk. `--delete-identity` also deletes them; it works for a network that is still configured and for one you removed earlier, by its old ID. Adding a network again always creates a new network with a new identity, so this computer signs in and joins as a new machine. Neither command removes the machine from the control server; an administrator has to do that.

## Devices and decisions

### devices

Lists devices on all connected networks, or on one network:

```console
$ flavorctl devices 01J9X4T6K8M2Q7R3V5W0YBZCDE
NETWORK                     NODE  HOSTNAME     ADDRESSES                     ONLINE
01J9X4T6K8M2Q7R3V5W0YBZCDE  1     workstation  100.64.0.1,fd7a:115c:a1e0::1  true
01J9X4T6K8M2Q7R3V5W0YBZCDE  2     nas          100.64.0.2,fd7a:115c:a1e0::2  true
01J9X4T6K8M2Q7R3V5W0YBZCDE  3     pi-hole      100.64.0.3,fd7a:115c:a1e0::3  true
01J9X4T6K8M2Q7R3V5W0YBZCDE  6     printer      100.64.0.6,fd7a:115c:a1e0::6  false
```

### explain

Shows which network an address or name belongs to, and why. Also available as `flavorctl inspect`. The destination can be an address, an address with a port, a MagicDNS name, a device name or a Flavor name. It does not connect to anything.

```console
$ flavorctl explain 10.20.30.40
destination  10.20.30.40 (address)
decision     unique: office-router on Work
reason       longest prefix

NETWORK       DEVICE         MATCH                       STATUS     ADDRESSES
Work          office-router  subnet route 10.20.30.0/24  selected   100.77.5.2,fd7a:115c:a1e0::4d01:502
Acme staging  vpc-router     subnet route 10.20.0.0/16   outranked  100.64.0.4,fd7a:115c:a1e0::4

not checked (not connected): Studio
```

The decision is `unique`, `ambiguous` or `no match`. The rules behind it are described in [How Flavor decides](networking.md#how-flavor-decides).

### conflicts

Lists every address, DNS name, device name and subnet route that exists more than once across your connected networks, and whether it is an expected overlap or an ambiguity. See [Conflict Center](networking.md#conflict-center) for an example.

## Connections

### forward

```text
flavorctl forward [--network N] [--listen ADDR] <destination:port>
```

Listens on a loopback port and forwards every connection to the destination through the network Flavor chooses for it. Each new connection is resolved again first, and refused if the destination has become ambiguous or unreachable. Runs until Ctrl+C.

- `--network N` limits the decision to one network, by ID or exact name.
- `--listen ADDR` sets the loopback address and port. By default a free port on `127.0.0.1` is used.

```console
$ flavorctl forward --network "Acme staging" postgres:5432
```

A full example with output is in [Forward a local port](networking.md#forward-a-local-port).

### socks

```text
flavorctl socks [--listen ADDR]
```

Starts a SOCKS5 proxy, by default on `127.0.0.1:1080`, that reaches devices and routes on your connected networks. Use remote name resolution (`socks5h://` in curl) so names reach Flavor. Ambiguous destinations are refused. Also available as `flavorctl proxy`. Runs until Ctrl+C. See [SOCKS5 proxy](networking.md#socks5-proxy).

Forwarding and the proxy accept connections only from loopback and only from your own user.

## Preferences

```console
$ flavorctl preference set 100.64.0.3 --network "Home lab"
100.64.0.3 now prefers Home lab (a Flavor preference; system routing is not changed)

$ flavorctl preference list
DESTINATION  KIND     PREFERRED NETWORK
100.64.0.3   address  Home lab

$ flavorctl preference remove 100.64.0.3
```

`<network>` is a network ID or its exact display name. The command is also available as `flavorctl preferences` or `flavorctl prefer`; `list` also answers to `ls`, and `remove` to `rm` or `delete`. `flavorctl preference` with no subcommand lists preferences. How preferences are applied is described in [Destination preferences](networking.md#destination-preferences).

## Workspaces

```console
$ flavorctl workspace create --name "On call" --description "Production and monitoring" 01J9X4T6K8N3R8S4W6X1ZCA2FG 01J9X4T6K8M2Q7R3V5W0YBZCDE
01J9X4V2M3N4P5Q6R7S8T9V0W2

$ flavorctl workspace activate --disconnect-others "On call"
activated On call
  Home lab      already active
  Work          already active
  Acme staging  disconnected
  Studio        disconnected

$ flavorctl workspace list
ID                          NAME     ACTIVE  NETWORKS
01J9X4V2M3N4P5Q6R7S8T9V0W3  Home             Home lab
01J9X4V2M3N4P5Q6R7S8T9V0W2  On call  yes     Home lab, Work
01J9X4V2M3N4P5Q6R7S8T9V0W1  Work             Work, Acme staging
```

- `<workspace>` is a workspace ID or its exact name.
- `workspace edit --networks id,id` replaces the member list.
- `workspace activate` connects the workspace's networks. With `--disconnect-others` it also disconnects every other network that is connected or connecting, including one waiting for sign-in.
- `workspace deactivate` clears the active workspace without disconnecting anything.
- `workspace delete` removes the workspace only. Its networks are not touched.

`flavorctl workspace` with no subcommand lists workspaces. `list` also answers to `ls`, and `delete` to `rm`.

## JSON output

`--json` prints the daemon's response using the standard protobuf JSON mapping: field names in lowerCamelCase and enum values as their full names, matching the API definitions in [`proto/flavor/v1`](../proto/flavor/v1).

```console
$ flavorctl --json explain 100.64.0.3
{
  "daemonInstanceId": "9f2c41d07a5e4b6c8d13e27f50a9b3c6",
  "snapshotSequence": "42",
  "query": "100.64.0.3",
  "kind": "DESTINATION_KIND_ADDRESS",
  "normalized": "100.64.0.3",
  "decision": "RESOLUTION_DECISION_AMBIGUOUS",
  "reason": "DECISION_REASON_MULTIPLE_MATCHES",
  "decidedBy": "MATCH_KIND_DEVICE_ADDRESS",
  "candidates": [
    …
  ]
}
```
