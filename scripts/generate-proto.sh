#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export GOENV="${GOENV:-$ROOT/go.env}"
export PATH="$(go env GOPATH)/bin:${PATH:-}"
cd "$ROOT/proto"
buf dep update
buf generate
buf build --exclude-source-info -o "$ROOT/crates/lattice-proto/lattice.binpb"
cd "$ROOT"
go mod tidy
