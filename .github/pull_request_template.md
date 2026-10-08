## What this changes

Closes #

## How it was tested

- [ ] `./scripts/go.sh vet ./...` and `./scripts/go.sh test ./...`
- [ ] `cd desktop && npm run build && npm test`
- [ ] `cargo test --workspace` (after the frontend build)
- [ ] Tried it by hand (describe below)

## Checklist

- [ ] The title is lowercase and starts with `feat:`, `fix:` or `chore:`
- [ ] Tests cover the change, or it does not change behavior
- [ ] Docs (`README.md`, `doc/`) are updated if user-facing behavior changed
- [ ] No keys, sign-in links or other secrets appear in code, tests, logs or screenshots
- [ ] Generated code is regenerated with `./scripts/generate-proto.sh` if `.proto` files changed
