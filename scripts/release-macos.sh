#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

version="${1:?usage: scripts/release-macos.sh VERSION [amd64|arm64]}"
arch="${2:-$(./scripts/go.sh env GOARCH)}"
commit="$(git rev-parse HEAD)"
name="flavor-$version-darwin-$arch"
stage="dist/$name"

[ "$arch" = "$(./scripts/go.sh env GOHOSTARCH)" ] || { echo "the desktop app builds natively only" >&2; exit 1; }
rm -rf "$stage" "dist/$name.tar.gz"
mkdir -p "$stage/bin" "$stage/launchd"

pkg=git.lunarlabs.dev/flavor/flavor/internal/version
ldflags="-s -w -buildid= -X $pkg.daemonVersion=$version -X $pkg.buildCommit=$commit"
for cmd in flavord flavorctl; do
	CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" ./scripts/go.sh build -trimpath -buildvcs=false -ldflags "$ldflags" -o "$stage/bin/$cmd" "./cmd/$cmd"
done

sysroot="$(rustc --print sysroot)"
export RUSTFLAGS="${RUSTFLAGS:-} --remap-path-prefix=$ROOT=/build --remap-path-prefix=${CARGO_HOME:-$HOME/.cargo}=/cargo --remap-path-prefix=$sysroot=/rust"
(cd desktop && npm ci && npm run tauri build -- --runner "$ROOT/scripts/cargo-auditable.sh" --bundles app \
	--config '{"bundle":{"active":true,"macOS":{"signingIdentity":"-"}}}')
cp -R target/release/bundle/macos/Flavor.app "$stage/Flavor.app"
codesign --verify --deep --strict "$stage/Flavor.app"

cp packaging/launchd/dev.lunarlabs.flavor.flavord.plist "$stage/launchd/"
cp LICENSE README.md "$stage/"
find "$stage" -type d -exec chmod 755 {} +

COPYFILE_DISABLE=1 tar --uid 0 --gid 0 --uname root --gname wheel -C dist -czf "dist/$name.tar.gz" "$name"
echo "dist/$name.tar.gz"
