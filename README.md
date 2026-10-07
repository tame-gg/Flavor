# Lattice

Join and manage multiple Tailscale and Headscale networks from one desktop app.

Architecture: Tauri + React GUI → Connect-RPC over a local Unix socket → per-user Go daemon (`latticed`) → embedded `tsnet` NetworkSessions.

## Docs

- [Architecture](docs/architecture.md)
- [Security](docs/security.md)
- [Roadmap / slice-1 freeze](docs/roadmap.md)
- [HANDOFF](HANDOFF.md)

## Slice 1 status

In progress. Slice 1 proves multi-session Tailscale + Headscale connectivity with typed IPC. It does **not** include TUN, synthetic DNS, system routing, or a privileged helper.

## Development (target)

```bash
./scripts/go.sh run ./cmd/latticed
```

Desktop (`desktop/`) lands in later milestones.

## License

See [LICENSE](LICENSE).
