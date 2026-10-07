#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export GOENV="${GOENV:-$ROOT/go.env}"
exec go "$@"
