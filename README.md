# TSPack

TSPack is a TypeScript-first package manager prototype centered on deterministic intent (`manifest.tsx`) and deterministic resolved truth (`ts-lock.toml`).

## v1 command loop (M15b release-gate)

- `tspack check`
- `tspack update`
- `tspack sync`
- `tspack why <query>`
- `tspack pack`

## Core contracts

- `manifest.tsx` and `package.manifest.tsx` are parsed as restricted documents; they are **not executed**.
- `ts-lock.toml` is resolved truth.
- `node_modules` is a generated compatibility artifact, not source of truth.
- Fetch is not execute: dependency lifecycle scripts are not run.

## v1 non-goals

TSPack v1 does **not** implement package-manager commands for `build`, `test`, `dev`, `publish`, `add`, or `remove`, and does not run package scripts.

## Testing

```bash
go test ./...
cd manifest-frontend && npm test
```

See `docs/release-checklist.md` for release smoke commands and audit checklist.
