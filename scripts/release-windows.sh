#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

version="${1:?usage: scripts/release-windows.sh VERSION [amd64|arm64]}"
arch="${2:-$(./scripts/go.sh env GOARCH)}"
commit="$(git rev-parse HEAD)"
name="flavor-$version-windows-$arch"
stage="dist/$name"
case "$arch" in
	amd64) triple=x86_64-pc-windows-msvc ;;
	arm64) triple=aarch64-pc-windows-msvc ;;
	*) echo "unsupported arch $arch" >&2; exit 1 ;;
esac

[ "$arch" = "$(./scripts/go.sh env GOHOSTARCH)" ] || { echo "the desktop app builds natively only" >&2; exit 1; }
rm -rf "$stage" "dist/$name.zip" "dist/$name-setup.exe" desktop/src-tauri/binaries
mkdir -p "$stage/bin" desktop/src-tauri/binaries

pkg=git.lunarlabs.dev/flavor/flavor/internal/version
ldflags="-s -w -buildid= -X $pkg.daemonVersion=$version -X $pkg.buildCommit=$commit"
CGO_ENABLED=0 GOOS=windows GOARCH="$arch" ./scripts/go.sh build -trimpath -buildvcs=false -ldflags "$ldflags -H=windowsgui" -o "$stage/bin/flavord.exe" ./cmd/flavord
CGO_ENABLED=0 GOOS=windows GOARCH="$arch" ./scripts/go.sh build -trimpath -buildvcs=false -ldflags "$ldflags" -o "$stage/bin/flavorctl.exe" ./cmd/flavorctl
cp "$stage/bin/flavord.exe" "desktop/src-tauri/binaries/flavord-$triple.exe"
cp "$stage/bin/flavorctl.exe" "desktop/src-tauri/binaries/flavorctl-$triple.exe"

(cd desktop && npm ci && npm run tauri build -- --runner "$ROOT/scripts/cargo-auditable.cmd" --bundles nsis \
	--config '{"bundle":{"active":true,"externalBin":["binaries/flavord","binaries/flavorctl"],"windows":{"nsis":{"installMode":"currentUser","installerHooks":"../../packaging/windows/hooks.nsh"}}}}')
cp target/release/flavor-desktop.exe "$stage/Flavor.exe"
cp target/release/bundle/nsis/*-setup.exe "dist/$name-setup.exe"
cp LICENSE README.md "$stage/"

(cd dist && powershell.exe -NoProfile -Command "Compress-Archive -Path '$name' -DestinationPath '$name.zip'")
echo "dist/$name.zip"
echo "dist/$name-setup.exe"
