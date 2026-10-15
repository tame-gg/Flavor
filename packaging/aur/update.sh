#!/usr/bin/env bash
set -euo pipefail

usage="usage: packaging/aur/update.sh VERSION ASSETS_DIR"
version="${1:?$usage}"
assets="${2:?$usage}"
aur="$(cd "$(dirname "$0")" && pwd)"

pkgver="${version/-beta./beta}"
if [[ ! "$pkgver" =~ ^[0-9][0-9A-Za-z.]*$ ]] || [ "${pkgver/beta/-beta.}" != "$version" ]; then
	echo "version $version has no AUR pkgver form" >&2
	exit 1
fi

for f in SHA256SUMS SHA256SUMS.asc "flavor-$version-linux-amd64.tar.gz" "flavor-$version-linux-arm64.tar.gz"; do
	[ -f "$assets/$f" ] || { echo "$assets is missing $f" >&2; exit 1; }
done
listed="$(awk -v a="flavor-$version-linux-amd64.tar.gz" -v b="flavor-$version-linux-arm64.tar.gz" '$2 == a || $2 == b' "$assets/SHA256SUMS")"
[ "$(printf '%s\n' "$listed" | grep -c .)" -eq 2 ] || { echo "SHA256SUMS does not list both Linux tarballs" >&2; exit 1; }
(cd "$assets" && printf '%s\n' "$listed" | sha256sum --check --strict) >&2

sum() { sha256sum "$assets/$1" | cut -d' ' -f1; }

for pkg in flavor flavor-bin; do
	pkgbuild="$aur/$pkg/PKGBUILD"
	old="$(sed -n 's/^pkgver=//p' "$pkgbuild")"
	pkgrel="$(sed -n 's/^pkgrel=//p' "$pkgbuild")"
	[ "$old" = "$pkgver" ] || pkgrel=1
	sed -i -e "s/^pkgver=.*/pkgver=$pkgver/" -e "s/^pkgrel=.*/pkgrel=$pkgrel/" "$pkgbuild"
done

bin="$aur/flavor-bin/PKGBUILD"
sed -i \
	-e "s/^sha256sums=('[0-9a-f]\{64\}'$/sha256sums=('$(sum SHA256SUMS)'/" \
	-e "s/^sha256sums_x86_64=('[0-9a-f]\{64\}')$/sha256sums_x86_64=('$(sum "flavor-$version-linux-amd64.tar.gz")')/" \
	-e "s/^sha256sums_aarch64=('[0-9a-f]\{64\}')$/sha256sums_aarch64=('$(sum "flavor-$version-linux-arm64.tar.gz")')/" \
	"$bin"
for want in "sha256sums=('$(sum SHA256SUMS)'" \
	"sha256sums_x86_64=('$(sum "flavor-$version-linux-amd64.tar.gz")')" \
	"sha256sums_aarch64=('$(sum "flavor-$version-linux-arm64.tar.gz")')"; do
	grep -qxF "$want" "$bin" || { echo "could not set $want in $bin" >&2; exit 1; }
done
