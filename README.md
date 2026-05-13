# TSPack

TSPack is a TypeScript-first package manager prototype centered on deterministic intent (`manifest.tsx`) and deterministic resolved truth (`ts-lock.toml`).

## v1 command loop (M15b release-gate)

- `tspack check`
- `tspack update`
- `tspack sync`
- `tspack why <query>`
- `tspack pack`
- `tspack test`

## Core contracts

- `manifest.tsx` and `package.manifest.tsx` are parsed as restricted documents; they are **not executed**.
- `ts-lock.toml` is resolved truth.
- `node_modules` is a generated compatibility artifact, not source of truth.
- Fetch is not execute: dependency lifecycle scripts are not run.

## v1 non-goals

TSPack v1 does **not** implement package-manager commands for `build`, `dev`, `publish`, `add`, or `remove`, and does not run package scripts.

`tspack test` is an orchestrator for native xTest and Vitest backends. See `docs/test-command.md`.

## Testing

```bash
go test ./...
cd manifest-frontend && npm test
```

See `docs/release-checklist.md` for release smoke commands and audit checklist.


## Standalone artifacts

- `tspack artifact
- `tspack bench` runs native benchmark units (`*.benchmark.tsx`).
` runs standalone native xTest `<Artifact>` units declared directly under `<Suite>`.
- `tspack pack` creates package `.tgz` archives; it is unrelated to native test artifacts.


See also `docs/artifacts.md` for standalone artifact mode details.

- `tspack doom` runs quarantined abnormal-termination Prophecy tests (`*.prophecy.tsx`) in subprocesses and writes doom artifacts.

See `docs/doom.md` for quarantine Doom/Prophecy execution details.


- `tspack inspect <url>`: **experimental** structural UI inspection for rendered browser targets; backend support is still being refined.
