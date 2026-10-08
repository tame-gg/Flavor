# System-wide names (experimental)

By default, Flavor reaches your networks without touching system networking: the desktop app and `flavorctl` work through the daemon, and other programs use [a forwarded port or the SOCKS5 proxy](networking.md#reach-a-service-without-root).

System-wide names go one step further. Any program on the computer can use a Flavor name such as `nas.home-lab.flavor.internal` directly, with no proxy settings, even when the device's real address also exists on another network. This needs a small privileged helper, `flavor-netd`, so it is **off by default** and **experimental**.

> [!IMPORTANT]
> Use system-wide names only on a computer where you are the only user. Routes and DNS settings apply to the whole host, so other local users could reach your networks through them. See [Single-user machines only](#single-user-machines-only).

## How it works

1. `flavord`, running as you, asks `flavor-netd` for a network interface.
2. The helper checks that you are allowed to (polkit, plus an active local login session), creates a TUN interface named `flv-u<your uid>`, adds routes for Flavor's synthetic address ranges, and hands the interface back to `flavord`.
3. The helper tells systemd-resolved to send queries for `flavor.internal` to Flavor's resolver on that interface.
4. When a program looks up `nas.home-lab.flavor.internal`, Flavor resolves the name with the same rules as the Connection Inspector and answers with a synthetic address that encodes both the network and the real address.
5. Connections to that synthetic address arrive on the TUN interface. `flavord` decodes which network and device they are for and connects through that network's embedded Tailscale node.

Two devices that both use `100.64.0.2` on different networks get different synthetic addresses, so the operating system can tell them apart.

## Requirements

- Flavor installed system-wide from a [release tarball or package](install.md), which places the helper at `/usr/libexec/flavor/flavor-netd` with its systemd socket, polkit policy and security profile.
- systemd-resolved handling DNS on the computer. `resolvectl status` should work, and `/etc/resolv.conf` should point at its stub resolver.
- polkit, and a graphical or console login on the computer itself. SSH sessions are not allowed to turn it on.

## Turn it on

1. Make sure the helper's socket is enabled. The tarball's `install.sh` already does this; the Arch packages leave it to you:

   ```bash
   sudo systemctl enable --now flavor-netd.socket
   ```

2. Start `flavord` with the helper. Open an override for your user service:

   ```bash
   systemctl --user edit flavord
   ```

   Add these lines, save, and restart the daemon:

   ```ini
   [Service]
   ExecStart=
   ExecStart=/usr/bin/flavord --synthetic-helper=/run/flavor/netd.sock
   ```

   ```bash
   systemctl --user restart flavord
   ```

3. Check that a name resolves and connects. Use one of your own devices; the Connection Inspector shows each device's Flavor name.

   ```bash
   resolvectl query nas.home-lab.flavor.internal
   curl http://nas.home-lab.flavor.internal:8080/
   ```

## What can be reached

- **Names under `flavor.internal`.** Flavor names have the form `<device>.<network>.flavor.internal`; see [Names](networking.md#names).
- **TCP and UDP**, including QUIC. Idle UDP flows close after two minutes.
- **Not ICMP.** `ping` gets no reply: Flavor's network stack only carries TCP and UDP, and drops ICMP rather than pretending a host answered.
- **Not raw tailnet addresses.** `100.64.0.2` and advertised subnet routes are not routed to the interface, because they can be ambiguous. Use names.
- **Not MagicDNS names.** Names like `nas.tail1234.ts.net` are not sent to Flavor; only `flavor.internal` is.

How DNS questions are answered:

| Situation | Answer |
| --- | --- |
| The name points at exactly one device | an `A` or `AAAA` record with its synthetic address, TTL 60 seconds |
| The name is under `flavor.internal` but matches nothing | `NXDOMAIN` |
| The name could mean devices on several networks | `SERVFAIL`, with an extended DNS error explaining that it is ambiguous |
| A [destination preference](networking.md#destination-preferences) settles the ambiguity | the preferred network's device |
| Another record type for an existing name | an empty answer |

## What it changes on your computer

| Change | Details |
| --- | --- |
| A TUN interface | `flv-u<uid>`, MTU 1280, not persistent: it disappears when `flavord` closes it |
| Two host addresses on that interface | one in a random private IPv6 `/48` (`fd00::/8`) chosen once and stored in your Flavor database, and one IPv4 address from a `/20` inside `198.18.0.0/15` |
| Two routes | the IPv6 `/48` and the IPv4 `/20`, both through the interface, tagged with route protocol 76 |
| systemd-resolved settings for that interface | Flavor's resolver as the DNS server and `flavor.internal` as a routing-only domain; the interface is never the default DNS route |

Flavor picks an IPv4 range that does not overlap any existing route or address, and the helper refuses to create the interface if either range would overlap an existing address, route or routing rule. If the stored IPv4 range later starts to overlap a local network, Flavor answers only with IPv6 addresses for that run instead of shadowing your network.

It does **not** edit `/etc/resolv.conf`, firewall rules or sysctls, and it does not install routes for your real tailnet ranges.

## Authorization and confinement

- **Who may turn it on.** Creating or changing the interface needs polkit authorization for `dev.lunarlabs.flavor.netd.manage-interface`. The shipped policy allows it, without a password prompt, only for active local sessions. The helper additionally requires the calling user's login session to be active and not remote, so it refuses SSH sessions and sessions running in the background, for example after switching users.
- **How the caller is identified.** The helper takes the caller's identity from the kernel (`SO_PEERCRED` and, on Linux 6.5 or newer, a pidfd), never from the request.
- **One owner per host.** Only one user can own the interface at a time. Other users are refused without learning who owns it.
- **The socket.** Any local program can connect to `/run/flavor/netd.sock`, which starts the helper; only the checks above decide whether anything changes.
- **Privileges.** The helper runs as root with `CAP_NET_ADMIN` as its only capability, started on demand by `flavor-netd.socket`. Its systemd unit sets `NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, `DevicePolicy=closed` with only `/dev/net/tun` allowed, `RestrictAddressFamilies=AF_UNIX AF_NETLINK`, `IPAddressDeny=any`, `MemoryDenyWriteExecute` and the kernel-protection options.
- **Mandatory access control.** `install.sh` loads the [AppArmor profile](../packaging/apparmor/flavor-netd) on AppArmor systems and the [SELinux module](../packaging/selinux) on SELinux systems. Both allow only the operations above.
- **It treats `flavord` as untrusted.** Requests are size-limited and rate-limited, addresses are validated, and the helper checks the host's current addresses, routes and routing rules again right before it creates the interface.

## Lifetime and cleanup

- `flavord` keeps a connection to the helper while system-wide names are active, and checks on it every 5 seconds.
- If `flavord` stops or crashes, the interface, its addresses and routes disappear with it, and the helper reverts the systemd-resolved settings.
- If your session becomes inactive, for example when you switch users, the helper removes everything. `flavord` sets it up again when the session is active again.
- If systemd-resolved restarts, the helper reapplies its settings.
- The helper exits after 60 seconds with nothing to do. When it starts, it removes any leftover `flv-u*` interface.

## Single-user machines only

The interface, routes and DNS settings belong to the whole host, not to your user. On a computer shared with other users, their programs could resolve your Flavor names and open connections to your devices, and `flavord` would carry that traffic with your identity, because packets on the interface do not say which user sent them. Forwarding and the SOCKS5 proxy do not have this problem: they check the user ID of every connection.

## Turn it off

Remove your override and restart the daemon. Flavor removes the interface and DNS settings when it stops using the helper.

```bash
systemctl --user revert flavord
systemctl --user restart flavord
```

To disable the helper for everyone:

```bash
sudo systemctl disable --now flavor-netd.socket
```

## Testing status

The helper was exercised by hand on throwaway virtual machines, before the project was renamed from Lattice, on Ubuntu 26.04 (AppArmor, systemd 259, polkit 127) and Fedora 43 (SELinux enforcing, systemd 258, polkit 126). Those runs covered setup, name resolution and connections over IPv4 and IPv6, ambiguous names, crashes of either process, systemd-resolved restarts, inactive sessions, SSH refusals and cleanup. It has not been tested with AppArmor 4 (for example Ubuntu 24.04). Automated tests use fake kernel, polkit, logind and resolved backends.

Problems: see [Troubleshooting](troubleshooting.md#system-wide-names).
