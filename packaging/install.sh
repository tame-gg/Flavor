#!/bin/sh
set -eu

here=$(cd "$(dirname "$0")" && pwd)
[ "$(id -u)" = 0 ] || { echo "install.sh must run as root: sudo $0" >&2; exit 1; }
command -v systemctl >/dev/null || { echo "Flavor needs systemd" >&2; exit 1; }

if [ -x /usr/libexec/lattice/uninstall ]; then
	echo "Removing Lattice first; per-user data is kept and moved to Flavor when flavord first starts"
	/usr/libexec/lattice/uninstall
fi

systemctl stop flavor-netd.service 2>/dev/null || true

cd "$here"
while IFS= read -r f; do
	case "$f" in
	usr/*|etc/*) ;;
	*) echo "refusing manifest entry outside /usr and /etc: $f" >&2; exit 1 ;;
	esac
	case "$f" in *..*) echo "refusing manifest entry: $f" >&2; exit 1 ;; esac
	install -D -m "$(stat -c %a "$f")" "$f" "/$f"
done < usr/share/flavor/manifest

if [ -r /sys/module/apparmor/parameters/enabled ] && grep -q Y /sys/module/apparmor/parameters/enabled && command -v apparmor_parser >/dev/null; then
	apparmor_parser -r /etc/apparmor.d/flavor-netd
	echo "AppArmor profile flavor-netd loaded"
fi

if command -v selinuxenabled >/dev/null && selinuxenabled; then
	if [ -f /usr/share/selinux/packages/flavor/flavor_netd.pp ]; then
		semodule -i /usr/share/selinux/packages/flavor/flavor_netd.pp
		restorecon -RF /usr/libexec/flavor
		echo "SELinux module flavor_netd installed"
	else
		echo "warning: SELinux is enabled but this build has no flavor_netd.pp; flavor-netd runs as unconfined_service_t" >&2
	fi
fi

systemctl daemon-reload
systemctl enable flavor-netd.socket >/dev/null 2>&1
systemctl restart flavor-netd.socket

for u in $(loginctl list-users --no-legend 2>/dev/null | awk '{print $2}'); do
	systemctl --user -M "$u@" daemon-reload >/dev/null 2>&1 || true
	systemctl --user -M "$u@" try-restart flavord.service >/dev/null 2>&1 || true
done

echo "Flavor installed. Each user enables the daemon once with: systemctl --user enable --now flavord"
echo "Remove everything later with: sudo /usr/libexec/flavor/uninstall"
