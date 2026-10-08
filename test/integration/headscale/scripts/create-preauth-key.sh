#!/usr/bin/env bash
set -euo pipefail

INSTANCE="${1:-a}"
USER_NAME="${2:-flavor}"
COMPOSE_FILE="$(cd "$(dirname "$0")/.." && pwd)/compose.yml"
SERVICE="headscale-${INSTANCE}"

docker compose -f "$COMPOSE_FILE" exec -T "$SERVICE" \
  headscale users create "$USER_NAME" >/dev/null 2>&1 || true

USER_ID="$(docker compose -f "$COMPOSE_FILE" exec -T "$SERVICE" \
  headscale users list -o json | python3 -c '
import json,sys
name=sys.argv[1]
users=json.load(sys.stdin)
for u in users:
    if u.get("name")==name:
        print(u["id"]); raise SystemExit(0)
raise SystemExit("user not found: "+name)
' "$USER_NAME")"

docker compose -f "$COMPOSE_FILE" exec -T "$SERVICE" \
  headscale preauthkeys create --user "$USER_ID" --expiration 24h -o json | python3 -c '
import json,sys
obj=json.load(sys.stdin)
key=obj.get("key") or obj.get("pre_auth_key") or ""
if not key:
    raise SystemExit("no key in response: "+json.dumps(obj))
print(key)
'
