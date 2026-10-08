#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

version="${1:?usage: scripts/release.sh VERSION [amd64|arm64]}"
arch="${2:-$(./scripts/go.sh env GOARCH)}"
commit="$(git rev-parse HEAD)"
export SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-$(git log -1 --format=%ct)}"
name="lattice-$version-linux-$arch"
stage="dist/$name"

rm -rf "$stage" "dist/$name.tar.gz"
mkdir -p "$stage"

pkg=git.lunarlabs.dev/lattice/lattice/internal/version
ldflags="-s -w -buildid= -X $pkg.daemonVersion=$version -X $pkg.buildCommit=$commit"
gobuild() {
	CGO_ENABLED=0 GOOS=linux GOARCH="$arch" ./scripts/go.sh build -trimpath -buildvcs=false -ldflags "$ldflags" -o "$stage/$1" "$2"
}
gobuild usr/bin/latticed ./cmd/latticed
gobuild usr/bin/latticectl ./cmd/latticectl
gobuild usr/libexec/lattice/lattice-netd ./cmd/lattice-netd

if [ "${DESKTOP:-1}" = 1 ]; then
	[ "$arch" = "$(./scripts/go.sh env GOHOSTARCH)" ] || { echo "the desktop app builds natively only; use DESKTOP=0 to cross-build" >&2; exit 1; }
	sysroot="$(rustc --print sysroot)"
	export RUSTFLAGS="${RUSTFLAGS:-} --remap-path-prefix=$ROOT=/build --remap-path-prefix=${CARGO_HOME:-$HOME/.cargo}=/cargo --remap-path-prefix=$sysroot=/rust"
	(cd desktop && npm ci && npm run tauri build -- --runner "$ROOT/scripts/cargo-auditable.sh")
	install -Dm755 target/release/lattice-desktop "$stage/usr/bin/lattice-desktop"
	install -Dm644 packaging/desktop/dev.lunarlabs.lattice.desktop "$stage/usr/share/applications/dev.lunarlabs.lattice.desktop"
	install -Dm644 desktop/src-tauri/icons/128x128.png "$stage/usr/share/icons/hicolor/128x128/apps/dev.lunarlabs.lattice.png"
	mkdir -p "$stage/usr/share/lattice/frontend"
	jq '.packages |= with_entries(select(.value.dev != true)) | del(.packages[""].devDependencies)' desktop/package-lock.json > "$stage/usr/share/lattice/frontend/package-lock.json"
fi

p=packaging
mkdir -p "$stage/usr/lib/systemd/user"
sed 's|^ExecStart=%h/.local/bin/latticed$|ExecStart=/usr/bin/latticed|' "$p/systemd/user/latticed.service" > "$stage/usr/lib/systemd/user/latticed.service"
grep -q '^ExecStart=/usr/bin/latticed$' "$stage/usr/lib/systemd/user/latticed.service"
install -Dm644 "$p/systemd/system/lattice-netd.socket" "$stage/usr/lib/systemd/system/lattice-netd.socket"
install -Dm644 "$p/systemd/system/lattice-netd.service" "$stage/usr/lib/systemd/system/lattice-netd.service"
install -Dm644 "$p/polkit/actions/dev.lunarlabs.lattice.netd.policy" "$stage/usr/share/polkit-1/actions/dev.lunarlabs.lattice.netd.policy"
install -Dm644 "$p/apparmor/lattice-netd" "$stage/etc/apparmor.d/lattice-netd"
for f in lattice_netd.te lattice_netd.fc lattice_netd.if; do
	install -Dm644 "$p/selinux/$f" "$stage/usr/share/selinux/packages/lattice/$f"
done
if [ -n "${SELINUX_PP:-}" ]; then
	install -Dm644 "$SELINUX_PP" "$stage/usr/share/selinux/packages/lattice/lattice_netd.pp"
fi
install -Dm755 "$p/uninstall.sh" "$stage/usr/libexec/lattice/uninstall"
install -Dm755 "$p/install.sh" "$stage/install.sh"
install -Dm755 "$p/uninstall.sh" "$stage/uninstall.sh"
install -Dm644 LICENSE "$stage/LICENSE"
install -Dm644 README.md "$stage/README.md"

mkdir -p "$stage/usr/share/lattice"
(cd "$stage" && { find usr etc -type f; echo usr/share/lattice/manifest; } | sort -u > usr/share/lattice/manifest)
chmod 644 "$stage/usr/share/lattice/manifest"
find "$stage" -type d -exec chmod 755 {} +

tar --sort=name --mtime="@$SOURCE_DATE_EPOCH" --owner=0 --group=0 --numeric-owner --format=gnu -C dist -cf - "$name" | gzip -n -9 > "dist/$name.tar.gz"
echo "dist/$name.tar.gz"
