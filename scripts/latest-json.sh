#!/usr/bin/env bash
set -euo pipefail

# Writes DIR/latest.json, the Tauri updater manifest for the signed desktop bundles in DIR:
# flavor-VERSION-darwin-ARCH.app.tar.gz and flavor-VERSION-windows-ARCH-setup.exe, each with its .sig.
# The URLs point at https://cdn.tame.gg/flavor-desktop/VERSION/<file>, where cdn-publish.yml
# uploads the files once the release is published. Linux has no updater bundle.

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

version="${1:?usage: scripts/latest-json.sh VERSION DIR}"
dir="${2:?usage: scripts/latest-json.sh VERSION DIR}"
cdn="https://cdn.tame.gg/flavor-desktop/$version"

# The updater compares this against the running app's version; a mismatch would offer the same build forever.
conf="$(jq -r .version "$ROOT/desktop/src-tauri/tauri.conf.json")"
[ "$conf" = "$version" ] || { echo "version $version does not match tauri.conf.json ($conf)" >&2; exit 1; }

protocol="$(sed -n 's/^pub const PROTOCOL_MAJOR: u32 = \([0-9][0-9]*\);$/\1/p' "$ROOT/crates/flavor-ipc/src/lib.rs")"
[ -n "$protocol" ] || { echo "no PROTOCOL_MAJOR in crates/flavor-ipc/src/lib.rs" >&2; exit 1; }

platforms='{}'
shopt -s nullglob
for sig in "$dir"/*.sig; do
	file="$(basename "${sig%.sig}")"
	[ -f "$dir/$file" ] || { echo "$sig has no $file next to it" >&2; exit 1; }
	case "$file" in
		"flavor-$version-darwin-"*.app.tar.gz) os=darwin installer=app arch="${file#"flavor-$version-darwin-"}" arch="${arch%.app.tar.gz}" ;;
		"flavor-$version-windows-"*-setup.exe) os=windows installer=nsis arch="${file#"flavor-$version-windows-"}" arch="${arch%-setup.exe}" ;;
		*) echo "unknown updater bundle $file" >&2; exit 1 ;;
	esac
	case "$arch" in
		amd64) arch=x86_64 ;;
		arm64) arch=aarch64 ;;
		*) echo "unknown arch in $file" >&2; exit 1 ;;
	esac
	# Served under {os}-{arch}-{installer}, and under the bare {os}-{arch} for clients that don't name one.
	platforms="$(jq --arg os "$os-$arch" --arg key "$os-$arch-$installer" --arg url "$cdn/$file" --arg signature "$(tr -d '[:space:]' < "$sig")" '
		if has($key) or has($os) then error("two updater bundles for \($os)") else . end
		| .[$key] = {signature: $signature, url: $url} | .[$os] = .[$key]' <<<"$platforms")"
	echo "$os-$arch-$installer: $cdn/$file"
done
[ "$platforms" != '{}' ] || { echo "no signed updater bundles in $dir" >&2; exit 1; }

jq -n --arg version "$version" --arg pub_date "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --argjson protocol_major "$protocol" --argjson platforms "$platforms" \
	'{version: $version, notes: "Flavor \($version)", pub_date: $pub_date, protocol_major: $protocol_major, platforms: $platforms}' > "$dir/latest.json"
echo "$dir/latest.json"
