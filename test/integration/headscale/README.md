# Local Headscale integration harness

Repeatable Headscale environment for Flavor M4+ tsnet session testing.

## Start (single server)

```bash
docker compose -f test/integration/headscale/compose.yml up -d headscale-a
```

Control URL: `http://127.0.0.1:18080`

## Optional second server

```bash
docker compose -f test/integration/headscale/compose.yml --profile dual up -d
```

Second control URL: `http://127.0.0.1:18081`

## Create user + disposable pre-auth key

```bash
./test/integration/headscale/scripts/create-preauth-key.sh a flavor
```

Prints a one-time reusable=false pre-auth key to stdout. Do not commit keys.

## Integration test

```bash
export FLAVOR_INTEGRATION=1
export FLAVOR_HEADSCALE_URL=http://127.0.0.1:18080
export FLAVOR_HEADSCALE_AUTHKEY="$(./test/integration/headscale/scripts/create-preauth-key.sh a flavor)"
./scripts/go.sh test ./internal/session -run TestIntegrationHeadscale -count=1 -v
```

Ordinary `go test ./...` skips this test when `FLAVOR_INTEGRATION` is unset.

Instance a also publishes three `extra_records` under `intra.flavor-a.test`; the integration test checks that they reach `Session.DNSRecords()` and are cleared on stop.

## Notes

- Development HTTP (`http://`) is explicit for local harness only.
- Production Headscale paths keep normal TLS verification; do not set `InsecureSkipVerify`.
- Enrollment keys remain transient process input and must never be persisted by Flavor.
- Headscale 0.26+ requires a non-empty DERP map; this harness fetches Tailscale’s public default DERP map at container start (network required for compose up).
- `preauthkeys create --user` takes a numeric user id; the script resolves the name → id.
