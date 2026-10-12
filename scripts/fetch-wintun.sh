#!/usr/bin/env bash
set -euo pipefail

arch="${1:?usage: scripts/fetch-wintun.sh amd64|arm64 DEST}"
dest="${2:?usage: scripts/fetch-wintun.sh amd64|arm64 DEST}"
sha=07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

curl -fsSLo "$work/wintun.zip" https://www.wintun.net/builds/wintun-0.14.1.zip
echo "$sha  $work/wintun.zip" | sha256sum -c - >/dev/null
"$(cygpath -u "$SYSTEMROOT")/System32/tar.exe" -xf "$(cygpath -w "$work/wintun.zip")" -C "$(cygpath -w "$work")" "wintun/bin/$arch/wintun.dll"
cp "$work/wintun/bin/$arch/wintun.dll" "$dest"
