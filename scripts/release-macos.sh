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
rm -rf "$stage" "dist/$name.tar.gz" "dist/$name.app.tar.gz" "dist/$name.app.tar.gz.sig"
mkdir -p "$stage/bin" "$stage/launchd"

pkg=git.lunarlabs.dev/flavor/flavor/internal/version
ldflags="-s -w -buildid= -X $pkg.daemonVersion=$version -X $pkg.buildCommit=$commit"
for cmd in flavord flavorctl; do
	CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" ./scripts/go.sh build -trimpath -buildvcs=false -ldflags "$ldflags" -o "$stage/bin/$cmd" "./cmd/$cmd"
done

# UPDATER=1 also signs Flavor.app.tar.gz for the Tauri updater, using TAURI_SIGNING_PRIVATE_KEY.
updater=false
[ "${UPDATER:-0}" = 1 ] && updater=true

sysroot="$(rustc --print sysroot)"
export RUSTFLAGS="${RUSTFLAGS:-} --remap-path-prefix=$ROOT=/build --remap-path-prefix=${CARGO_HOME:-$HOME/.cargo}=/cargo --remap-path-prefix=$sysroot=/rust"
(cd desktop && npm ci && npm run tauri build -- --runner "$ROOT/scripts/cargo-auditable.sh" --bundles app \
	--config '{"bundle":{"active":true,"createUpdaterArtifacts":'"$updater"',"macOS":{"signingIdentity":"-"}}}')
cp -R target/release/bundle/macos/Flavor.app "$stage/Flavor.app"
codesign --verify --deep --strict "$stage/Flavor.app"
if [ "$updater" = true ]; then
	cp target/release/bundle/macos/Flavor.app.tar.gz "dist/$name.app.tar.gz"
	cp target/release/bundle/macos/Flavor.app.tar.gz.sig "dist/$name.app.tar.gz.sig"
fi

cp packaging/launchd/dev.lunarlabs.flavor.flavord.plist "$stage/launchd/"
cp LICENSE README.md "$stage/"
find "$stage" -type d -exec chmod 755 {} +

COPYFILE_DISABLE=1 tar --uid 0 --gid 0 --uname root --gname wheel -C dist -czf "dist/$name.tar.gz" "$name"
echo "dist/$name.tar.gz"
