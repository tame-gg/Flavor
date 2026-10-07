#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export PATH="$(go env GOPATH)/bin:${PATH:-}"
cd "$ROOT/proto"
buf dep update
buf generate
cd "$ROOT"
go mod tidy
