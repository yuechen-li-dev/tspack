# Release checklist (v1-ish prototype)

## Required automated tests

```bash
go test ./cmd/... ./internal/... ./tools/...
cd manifest-frontend && npm test
```

## Manual smoke loop

```bash
tspack --version
tspack help
tspack check --root <fixture>
tspack update --root <fixture>
tspack sync --root <fixture>
tspack why <query> --root <fixture>
tspack pack --root <fixture> --out <tmp>
```

## Supported commands

- `check`
- `update`
- `sync`
- `why`
- `pack`
- `help`
- `--version`

## v1 non-goals

- No `build`, `test`, `dev`, `publish`, `add`, `remove` commands.
- No script execution/lifecycle execution.
- No vulnerability or license scanning.

## Mutation contract

- `check`: no lockfile/store/node_modules mutation.
- `update`: lockfile-only mutation (`ts-lock.toml`).
- `sync`: mutates `node_modules` compatibility output only.
- `why`: no lockfile/store/node_modules mutation.
- `pack`: writes pack artifacts only; does not mutate lockfile/store/node_modules.

## Security contract

- Fetch is not execute.
- Capabilities are recorded/checked, not executed.
- Restricted manifest parser frontend prevents arbitrary execution.

## Known limitations/follow-ups

- Live network npm behavior is not part of deterministic core tests unless explicitly configured.
- Resolver entrypoint naming debt is addressed with `resolver.Resolve(...)` wrapper over legacy `ResolveNPM(...)`; full rename can happen post-v1.
