# TSPack

TSPack is a TypeScript lifecycle tool centered on deterministic intent (`manifest.tsx`) and deterministic resolved truth (`ts-lock.toml`).

Core thesis: **Declare targets. Resolve sources. Enforce boundaries. Lock reality. Pack exactly.**

## Command surface

| Group | Command | Purpose | Mutation / Stability |
|---|---|---|---|
| Core package | `tspack init` | Scaffold a starter `manifest.tsx` + entry source. | No install/update/sync/build side effects. |
| Core package | `tspack check` | Validate manifest/frontend, graph, boundaries, and lock consistency. Supports `--json` for structured stdout diagnostics. | Does not mutate lock. |
| Core package | `tspack update` | Resolve and write deterministic `ts-lock.toml`. | Mutates lock. |
| Core package | `tspack sync` | Materialize compatibility `node_modules` from lock/store. | Does not mutate lock. |
| Core package | `tspack why` | Explain dependency/target reachability and presence. | Does not mutate lock. |
| Core package | `tspack pack` | Build deterministic package archives. | Does not mutate lock. |
| Native harness | `tspack test` | Run native xTest/Vitest command loop. | May write test outputs; no lock/manifest mutation. |
| Native harness | `tspack artifact` | Run standalone suite-level native artifact units. | May write artifact output; no lock/manifest mutation. |
| Native harness | `tspack bench` | Run native benchmark units (`*.benchmark.tsx`). | May write benchmark outputs; no lock/manifest mutation. |
| Native harness | `tspack doom` | Run quarantined prophecy/doom units (`*.prophecy.tsx`). | May write doom outputs; no lock/manifest mutation. |
| Runtime / inspection | `tspack run [target]` | Start declared manifest `RunTargets` and wait for readiness. | **Not npm scripts**; no lock/manifest mutation. |
| Runtime / inspection | `tspack inspect <url\|target>` | Structural UI inspection; supports declared run target inspection. | **Experimental**; backend surface may evolve. |

## Core contracts

- `manifest.tsx` and `package.manifest.tsx` are restricted documents; they are **not executed**.
- `ts-lock.toml` is resolved truth.
- `node_modules` is a generated compatibility artifact, not source of truth.
- Fetch is not execute: dependency lifecycle scripts are not run by default.

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
