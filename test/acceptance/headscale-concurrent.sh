#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
HS="$ROOT/test/integration/headscale"
WORK="$(mktemp -d)"
BIN="$WORK/bin"
mkdir -p "$BIN" "$WORK/run"
chmod 700 "$WORK/run"
export XDG_RUNTIME_DIR="$WORK/run" XDG_DATA_HOME="$WORK/data" XDG_CONFIG_HOME="$WORK/config"
unset TS_AUTHKEY TS_AUTH_KEY
DAEMON_PID=""

cleanup() {
  [ -n "$DAEMON_PID" ] && kill "$DAEMON_PID" 2>/dev/null && wait "$DAEMON_PID" 2>/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT

"$ROOT/scripts/go.sh" build -o "$BIN/flavord" ./cmd/flavord
"$ROOT/scripts/go.sh" build -o "$BIN/flavorctl" ./cmd/flavorctl
ctl() { "$BIN/flavorctl" "$@"; }

start_daemon() {
  "$BIN/flavord" --secret-store=memory --log-level=debug >>"$WORK/flavord.log" 2>&1 &
  DAEMON_PID=$!
  for _ in $(seq 1 100); do
    [ -S "$XDG_RUNTIME_DIR/flavor/flavord.sock" ] && ctl info >/dev/null 2>&1 && return
    sleep 0.1
  done
  echo "flavord did not become ready" >&2
  exit 1
}

stop_daemon() {
  kill "$DAEMON_PID"
  wait "$DAEMON_PID" || true
  DAEMON_PID=""
}

wait_connected() {
  for _ in $(seq 1 120); do
    if ctl list | grep "^$1 " | grep -q " connected "; then return; fi
    sleep 0.5
  done
  echo "network $1 did not connect" >&2
  ctl list >&2
  exit 1
}

KEY_A="$("$HS/scripts/create-preauth-key.sh" a flavor)"
KEY_B="$("$HS/scripts/create-preauth-key.sh" b flavor)"

start_daemon
INSTANCE_1="$(ctl info | awk '/^instance/ {print $2}')"
A="$(ctl add --name "Headscale A" --headscale http://127.0.0.1:18080 --auto-connect)"
B="$(ctl add --name "Headscale B" --headscale http://127.0.0.1:18081 --auto-connect)"
printf '%s\n' "$KEY_A" | ctl enroll "$A"
printf '%s\n' "$KEY_B" | ctl enroll "$B"
wait_connected "$A"
wait_connected "$B"
echo "== both sessions connected in one flavord"
ctl list
ctl devices

ADDR_A="$(ctl devices "$A" | awk 'NR>1 {print $4}' | sort | head -1)"
ADDR_B="$(ctl devices "$B" | awk 'NR>1 {print $4}' | sort | head -1)"
echo "== first address A=$ADDR_A B=$ADDR_B"

stop_daemon
start_daemon
INSTANCE_2="$(ctl info | awk '/^instance/ {print $2}')"
[ "$INSTANCE_1" != "$INSTANCE_2" ] || { echo "instance id did not change" >&2; exit 1; }
wait_connected "$A"
wait_connected "$B"
echo "== restart: new instance $INSTANCE_2, both reconnected from persisted identity without keys"

ctl remove "$A"
[ -d "$XDG_DATA_HOME/flavor/networks/$A" ] || { echo "soft remove deleted identity" >&2; exit 1; }
ctl remove --delete-identity "$B"
[ ! -e "$XDG_DATA_HOME/flavor/networks/$B" ] || { echo "hard delete left identity" >&2; exit 1; }
ctl remove --delete-identity "$A"
echo "== soft remove kept identity; hard delete removed it"
stop_daemon

if grep -rqaF -e "$KEY_A" -e "$KEY_B" "$WORK/flavord.log" "$XDG_DATA_HOME" "$XDG_CONFIG_HOME" 2>/dev/null; then
  echo "enrollment key found in logs or state" >&2
  exit 1
fi
if grep -qE "https?://[^ ]*/register/" "$WORK/flavord.log"; then
  echo "auth url found in flavord log" >&2
  exit 1
fi
echo "== no enrollment key in logs, data or config; no auth URL in daemon log"
echo "PASS"
