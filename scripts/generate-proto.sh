#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export GOENV="${GOENV:-$ROOT/go.env}"
gopath="$(go env GOPATH)"
export PATH="$gopath/bin:${PATH:-}"
cd "$ROOT/proto"
buf dep update
buf generate
buf build --exclude-source-info -o "$ROOT/crates/flavor-proto/flavor.binpb"
cd "$ROOT"
go mod tidy
