#!/bin/sh
set -u

[ "$(id -u)" = 0 ] || { echo "uninstall must run as root: sudo $0" >&2; exit 1; }
manifest=/usr/share/lattice/manifest
[ -r "$manifest" ] || { echo "no Lattice installation found ($manifest is missing)" >&2; exit 1; }
files=$(cat "$manifest")

for u in $(loginctl list-users --no-legend 2>/dev/null | awk '{print $2}'); do
	systemctl --user -M "$u@" disable --now latticed.service >/dev/null 2>&1 || true
done
systemctl disable --now lattice-netd.socket lattice-netd.service >/dev/null 2>&1 || true

for l in $(ip -o link show 2>/dev/null | awk -F': ' '{print $2}' | grep '^lat-u'); do
	ip link delete "$l" 2>/dev/null || true
done

if command -v semodule >/dev/null && semodule -l 2>/dev/null | grep -qx lattice_netd; then
	semodule -r lattice_netd
fi
if [ -r /sys/kernel/security/apparmor/profiles ] && grep -q '^lattice-netd ' /sys/kernel/security/apparmor/profiles && command -v apparmor_parser >/dev/null; then
	apparmor_parser -R /etc/apparmor.d/lattice-netd
fi

for f in $files; do
	case "$f" in
	usr/*|etc/*) ;;
	*) continue ;;
	esac
	case "$f" in *..*) continue ;; esac
	rm -f "/$f"
done
rmdir /usr/libexec/lattice /usr/share/lattice /usr/share/selinux/packages/lattice 2>/dev/null || true
rm -rf /run/lattice /run/lattice-netd
systemctl daemon-reload

echo "Lattice removed. Per-user data in ~/.local/share/lattice and ~/.config/lattice was kept; deleting it removes this machine's device identities."
