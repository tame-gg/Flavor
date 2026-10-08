#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

version="${1:?usage: scripts/release.sh VERSION [amd64|arm64]}"
arch="${2:-$(./scripts/go.sh env GOARCH)}"
commit="$(git rev-parse HEAD)"
export SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-$(git log -1 --format=%ct)}"
name="flavor-$version-linux-$arch"
stage="dist/$name"

rm -rf "$stage" "dist/$name.tar.gz"
mkdir -p "$stage"

pkg=git.lunarlabs.dev/flavor/flavor/internal/version
ldflags="-s -w -buildid= -X $pkg.daemonVersion=$version -X $pkg.buildCommit=$commit"
gobuild() {
	CGO_ENABLED=0 GOOS=linux GOARCH="$arch" ./scripts/go.sh build -trimpath -buildvcs=false -ldflags "$ldflags" -o "$stage/$1" "$2"
}
gobuild usr/bin/flavord ./cmd/flavord
gobuild usr/bin/flavorctl ./cmd/flavorctl
gobuild usr/libexec/flavor/flavor-netd ./cmd/flavor-netd

if [ "${DESKTOP:-1}" = 1 ]; then
	[ "$arch" = "$(./scripts/go.sh env GOHOSTARCH)" ] || { echo "the desktop app builds natively only; use DESKTOP=0 to cross-build" >&2; exit 1; }
	sysroot="$(rustc --print sysroot)"
	export RUSTFLAGS="${RUSTFLAGS:-} --remap-path-prefix=$ROOT=/build --remap-path-prefix=${CARGO_HOME:-$HOME/.cargo}=/cargo --remap-path-prefix=$sysroot=/rust"
	(cd desktop && npm ci && npm run tauri build -- --runner "$ROOT/scripts/cargo-auditable.sh")
	install -Dm755 target/release/flavor-desktop "$stage/usr/bin/flavor-desktop"
	install -Dm644 packaging/desktop/dev.lunarlabs.flavor.desktop "$stage/usr/share/applications/dev.lunarlabs.flavor.desktop"
	install -Dm644 desktop/src-tauri/icons/128x128.png "$stage/usr/share/icons/hicolor/128x128/apps/dev.lunarlabs.flavor.png"
	mkdir -p "$stage/usr/share/flavor/frontend"
	jq '.packages |= with_entries(select(.value.dev != true)) | del(.packages[""].devDependencies)' desktop/package-lock.json > "$stage/usr/share/flavor/frontend/package-lock.json"
fi

p=packaging
mkdir -p "$stage/usr/lib/systemd/user"
sed 's|^ExecStart=%h/.local/bin/flavord$|ExecStart=/usr/bin/flavord|' "$p/systemd/user/flavord.service" > "$stage/usr/lib/systemd/user/flavord.service"
grep -q '^ExecStart=/usr/bin/flavord$' "$stage/usr/lib/systemd/user/flavord.service"
install -Dm644 "$p/systemd/system/flavor-netd.socket" "$stage/usr/lib/systemd/system/flavor-netd.socket"
install -Dm644 "$p/systemd/system/flavor-netd.service" "$stage/usr/lib/systemd/system/flavor-netd.service"
install -Dm644 "$p/polkit/actions/dev.lunarlabs.flavor.netd.policy" "$stage/usr/share/polkit-1/actions/dev.lunarlabs.flavor.netd.policy"
install -Dm644 "$p/apparmor/flavor-netd" "$stage/etc/apparmor.d/flavor-netd"
for f in flavor_netd.te flavor_netd.fc flavor_netd.if; do
	install -Dm644 "$p/selinux/$f" "$stage/usr/share/selinux/packages/flavor/$f"
done
if [ -n "${SELINUX_PP:-}" ]; then
	install -Dm644 "$SELINUX_PP" "$stage/usr/share/selinux/packages/flavor/flavor_netd.pp"
fi
install -Dm755 "$p/uninstall.sh" "$stage/usr/libexec/flavor/uninstall"
install -Dm755 "$p/install.sh" "$stage/install.sh"
install -Dm755 "$p/uninstall.sh" "$stage/uninstall.sh"
install -Dm644 LICENSE "$stage/LICENSE"
install -Dm644 README.md "$stage/README.md"

mkdir -p "$stage/usr/share/flavor"
(cd "$stage" && { find usr etc -type f; echo usr/share/flavor/manifest; } | sort -u > usr/share/flavor/manifest)
chmod 644 "$stage/usr/share/flavor/manifest"
find "$stage" -type d -exec chmod 755 {} +

tar --sort=name --mtime="@$SOURCE_DATE_EPOCH" --owner=0 --group=0 --numeric-owner --format=gnu -C dist -cf - "$name" | gzip -n -9 > "dist/$name.tar.gz"
echo "dist/$name.tar.gz"
