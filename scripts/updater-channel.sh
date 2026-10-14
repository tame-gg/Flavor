#!/usr/bin/env bash
set -euo pipefail

version="${1:?usage: scripts/updater-channel.sh VERSION}"
channel="${UPDATER_CHANNEL:-flavor-desktop}"

case "$channel:$version" in
	flavor-desktop:* | flavor-desktop-test:*-rc* | flavor-desktop-test:*-beta* | flavor-desktop-test:*-alpha*) echo "$channel" ;;
	*) echo "updater channel $channel is not allowed for version $version" >&2; exit 1 ;;
esac
