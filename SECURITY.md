# Security policy

## Reporting a vulnerability

Please report security problems privately through GitHub: open the [Security tab](https://github.com/tame-gg/Flavor/security) and choose **Report a vulnerability**, or go directly to [the advisory form](https://github.com/tame-gg/Flavor/security/advisories/new). Do not open a public issue for a vulnerability.

A useful report includes:

- the Flavor version (`flavorctl info`) and your distribution,
- which part is affected: `flavord`, `flavorctl`, the desktop app, `flavor-netd` or its security policies, the installer, or the release artifacts,
- steps to reproduce, and what an attacker gains.

Do not include real pre-auth keys, sign-in links or private hostnames. The maintainer will reply in the advisory and coordinate a fix and disclosure with you.

## Supported versions

Flavor is in beta. Security fixes go into the next release; older releases, including those published under the previous name Lattice, are not updated.

| Version | Supported |
| --- | --- |
| 0.1.0-beta.2 (latest) | yes |
| older releases | no |

## Scope

In scope: everything in this repository and the release artifacts built from it, including the privileged helper `flavor-netd`, its systemd, polkit, AppArmor and SELinux configuration, and the install and uninstall scripts.

Out of scope: vulnerabilities in Tailscale, Headscale or other dependencies themselves. Please report those to their projects, unless the way Flavor uses them makes the problem worse.

## Security model

The [security model](https://github.com/tame-gg/Flavor/wiki/Security-Model) in the wiki describes what each component may do, how local access is controlled, how credentials are handled and what Flavor does not protect against. To check that a download is genuine, see [Verify the download](https://github.com/tame-gg/Flavor/wiki/Install#verify-the-download).
