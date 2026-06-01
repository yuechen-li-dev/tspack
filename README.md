# TSPack

TSPack is a TypeScript lifecycle tool centered on deterministic intent (`manifest.tsx`) and deterministic resolved truth (`ts-lock.toml`).

Core thesis: **Declare targets. Resolve sources. Enforce boundaries. Lock reality. Pack exactly.**

## Command surface

| Group | Command | Purpose | Mutation / Stability |
|---|---|---|---|
| Core package | `tspack init` | Scaffold a starter `manifest.tsx` + entry source. | No install/update/sync/build side effects. |
| Core package | `tspack migrate` | Generate a package.json-based `manifest.migrated.tsx` draft and migration report for human/LLM review; package-lock, source import scanning, script classification, RunTarget suggestions, and `--check` validation are report-only evidence. | Dry-run by default; `--check` validates without writing, `--write --check` writes only after validation passes, and migration never runs scripts or writes `ts-lock.toml`. |
| Core package | `tspack check` | Validate manifest/frontend, graph, boundaries, and lock consistency. Supports `--json`, `--explain <file>`, and optional `--format` read-only formatting validation. | Does not mutate lock. |
| Core package | `tspack update` | Resolve and write deterministic `ts-lock.toml` (or plan-only with `--dry-run`); text mode reports plain progress on stderr and supports `--quiet`. | Mutates lock (except `--dry-run`). |
| Core package | `tspack sync` | Materialize compatibility `node_modules` from lock/store. | Does not mutate lock. |
| Core package | `tspack why` `tspack how` | Explain dependency/target reachability and presence; `why --json` emits structured explanations and diagnostics; `why --reverse` shows which roots pull a locked package in. | Does not mutate lock. |
| Core package | `tspack pack` | Build deterministic package archives. | Does not mutate lock. |
| Native harness | `tspack test` | Run native xTest/Vitest command loop. | May write test outputs; no lock/manifest mutation. |
| Native harness | `tspack artifact` | Run standalone suite-level native artifact units. | May write artifact output; no lock/manifest mutation. |
| Native harness | `tspack bench` | Run native benchmark units (`*.benchmark.tsx`). | May write benchmark outputs; no lock/manifest mutation. |
| Native harness | `tspack doom` | Run quarantined prophecy/doom units (`*.prophecy.tsx`). | May write doom outputs; no lock/manifest mutation. |
| Runtime / inspection | `tspack run [target]` | Start, list (`--list [--json]`), or package-scope (`--package <name>`) declared manifest `RunTargets`. | **Not npm scripts**; no lock/manifest mutation. |
| Runtime / inspection | `tspack inspect <url\|target>` | Structural UI inspection; supports declared run target inspection. | **Experimental**; backend surface may evolve. |
| Runtime / inspection | `tspack doctor runtime` | Report the selected workspace runtime profile and executable availability. | Read-only; no package-manager delegation. |

## Core contracts

- `manifest.tsx` and `package.manifest.tsx` are restricted documents; they are **not executed**.
- `ts-lock.toml` is resolved truth.
- `node_modules` is a generated compatibility artifact, not source of truth.
- Fetch is not execute: dependency lifecycle scripts are not run by default.
- Workspace `runtime` selects `nodejs`, `bun`, or `deno` as a runtime profile; TSPack still owns dependency resolution, lockfiles, materialization, checks, packing, and lifecycle policy. See `docs/runtime-switch-demo.md` for the one-line runtime switch fixture.

## Non-goals (current)

TSPack does **not** aim to become:
- npm-script compatibility mode
- arbitrary task runner
- build/bundler tool
- publish pipeline

`inspect` remains experimental and should not be treated as a stable contract yet.

## Testing

```bash
go test ./...
cd manifest-frontend && npm test
```

See:
- `docs/product-contract.md`
- `docs/commands.md`
- `docs/design-non-goals.md`
- `docs/release-gate.md`


- `tspack format` and `tspack lint` are Biome-backed lifecycle UX commands. See `docs/format-lint.md`.

- `tspack doctor` adds non-mutating environment diagnostics. See `docs/doctor.md`.
