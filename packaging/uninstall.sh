#!/bin/sh
set -u

[ "$(id -u)" = 0 ] || { echo "uninstall must run as root: sudo $0" >&2; exit 1; }
manifest=/usr/share/flavor/manifest
[ -r "$manifest" ] || { echo "no Flavor installation found ($manifest is missing)" >&2; exit 1; }
files=$(cat "$manifest")

for u in $(loginctl list-users --no-legend 2>/dev/null | awk '{print $2}'); do
	systemctl --user -M "$u@" disable --now flavord.service >/dev/null 2>&1 || true
done
systemctl disable --now flavor-netd.socket flavor-netd.service >/dev/null 2>&1 || true

for l in $(ip -o link show 2>/dev/null | awk -F': ' '{print $2}' | grep '^flv-u'); do
	ip link delete "$l" 2>/dev/null || true
done

if command -v semodule >/dev/null && semodule -l 2>/dev/null | grep -qx flavor_netd; then
	semodule -r flavor_netd
fi
if [ -r /sys/kernel/security/apparmor/profiles ] && grep -q '^flavor-netd ' /sys/kernel/security/apparmor/profiles && command -v apparmor_parser >/dev/null; then
	apparmor_parser -R /etc/apparmor.d/flavor-netd
fi

for f in $files; do
	case "$f" in
	usr/*|etc/*) ;;
	*) continue ;;
	esac
	case "$f" in *..*) continue ;; esac
	rm -f "/$f"
done
rmdir /usr/libexec/flavor /usr/share/flavor/frontend /usr/share/flavor /usr/share/selinux/packages/flavor 2>/dev/null || true
rm -rf /run/flavor /run/flavor-netd
systemctl daemon-reload

echo "Flavor removed. Per-user data in ~/.local/share/flavor and ~/.config/flavor was kept; deleting it removes this machine's device identities."
