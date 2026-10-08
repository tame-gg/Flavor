#!/bin/sh
set -eu

here=$(cd "$(dirname "$0")" && pwd)
[ "$(id -u)" = 0 ] || { echo "install.sh must run as root: sudo $0" >&2; exit 1; }
command -v systemctl >/dev/null || { echo "Lattice needs systemd" >&2; exit 1; }

systemctl stop lattice-netd.service 2>/dev/null || true

cd "$here"
while IFS= read -r f; do
	case "$f" in
	usr/*|etc/*) ;;
	*) echo "refusing manifest entry outside /usr and /etc: $f" >&2; exit 1 ;;
	esac
	case "$f" in *..*) echo "refusing manifest entry: $f" >&2; exit 1 ;; esac
	install -D -m "$(stat -c %a "$f")" "$f" "/$f"
done < usr/share/lattice/manifest

if [ -r /sys/module/apparmor/parameters/enabled ] && grep -q Y /sys/module/apparmor/parameters/enabled && command -v apparmor_parser >/dev/null; then
	apparmor_parser -r /etc/apparmor.d/lattice-netd
	echo "AppArmor profile lattice-netd loaded"
fi

if command -v selinuxenabled >/dev/null && selinuxenabled; then
	if [ -f /usr/share/selinux/packages/lattice/lattice_netd.pp ]; then
		semodule -i /usr/share/selinux/packages/lattice/lattice_netd.pp
		restorecon -RF /usr/libexec/lattice
		echo "SELinux module lattice_netd installed"
	else
		echo "warning: SELinux is enabled but this build has no lattice_netd.pp; lattice-netd runs as unconfined_service_t" >&2
	fi
fi

systemctl daemon-reload
systemctl enable lattice-netd.socket >/dev/null 2>&1
systemctl restart lattice-netd.socket

for u in $(loginctl list-users --no-legend 2>/dev/null | awk '{print $2}'); do
	systemctl --user -M "$u@" daemon-reload >/dev/null 2>&1 || true
	systemctl --user -M "$u@" try-restart latticed.service >/dev/null 2>&1 || true
done

echo "Lattice installed. Each user enables the daemon once with: systemctl --user enable --now latticed"
echo "Remove everything later with: sudo /usr/libexec/lattice/uninstall"
