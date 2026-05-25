# Command inventory (M24)

| Command | Purpose | Mutates manifest/lock? | Notable non-goals | Details |
|---|---|---|---|---|
| `tspack init` | Scaffold a starter manifest and entry source for `library` or `app`. | **Yes (files)** / No | Does not install, update lock, sync, or build outputs. | `docs/init.md` |
| `tspack check` | Validate manifest/frontend, graph, boundaries, type surfaces, and lock consistency when lock exists. | No / No | Does not resolve or install packages. | `docs/contract.md` |
| `tspack update` | Resolve sources, fetch required package artifacts into the content-addressed store, and then write deterministic `ts-lock.toml`. Supports `--dry-run` plan mode. | No / **Yes (lock)** | Does not execute lifecycle scripts or run npm/npx; prepares lock+store for sync. | `docs/lockfile.md`, `docs/source-resolvers.md` |
| `tspack sync` | Materialize compatibility `node_modules` from lock/store. | No / No | Does not re-resolve versions. | `docs/materialization.md` |
| `tspack why`
- `tspack how` | Explain why dependency/target/lock package is present. | No / No | Not a resolver/editor command. | `docs/why.md` |
| `tspack outdated` | Report declared dependencies with current/wanted/latest npm freshness data (`--json` supported). | No / No | Read-only query; no lock/store/node_modules mutation. | `docs/outdated.md` |
| `tspack pack` | Create deterministic package archives. | No / No | Not a build pipeline or publish command. | `docs/pack.md` |
| `tspack run [target]` | Start declared manifest `RunTargets` and wait for readiness. | No / No | Not `npm run`; no package.json script inference. | `docs/run.md` |
| `tspack test` | Run test backends (native xTest and/or Vitest). | No / No | Not a generic task runner. | `docs/test-command.md`, `docs/native-test-harness.md` |
| `tspack artifact` | Run standalone native suite artifacts. | No / No | Not package artifact packing. | `docs/artifacts.md` |
| `tspack bench` | Run native benchmark units (`*.benchmark.tsx`). | No / No | Not a general profiling framework. | `docs/benchmarks.md` |
| `tspack doom` | Run quarantined prophecy/doom units (`*.prophecy.tsx`). | No / No | Not a generic chaos platform. | `docs/doom.md` |
| `tspack inspect <url\|target>` | Structural UI inspection and run-target inspection (experimental backends: platform-webview scaffold, CDP, host-path, Playwright Chromium). | No / No | Not screenshot diffing/visual testing; not auto-attach. | `docs/inspect.md` (**experimental**) |

## `tspack check --json`

- `tspack check --json` writes a machine-readable JSON report to **stdout**.
- The JSON report includes command metadata, `ok`, summary counts (`errors`, `warnings`, `info`, `total`), and ordered diagnostics.
- In JSON mode, human-readable diagnostics are not mixed into stdout.
- Exit behavior is unchanged:
  - warning diagnostics (including lock version conflicts) keep exit `0` when no errors exist;
  - exit `0` when there are no error diagnostics (warnings-only remains `0`);
  - nonzero when one or more error diagnostics exist, or on fatal runtime/command failure.

## `tspack update --dry-run`

- `tspack update --dry-run` resolves the would-be lockfile state and prints a deterministic package-level diff.
- It may fetch registry metadata to resolve versions, but does **not** write `ts-lock.toml`, does **not** populate store artifacts, and does **not** materialize `node_modules`.
- It exits `0` on successful planning regardless of whether changes are present; resolver/runtime errors remain non-zero.

## Stability

- Stable core package surface: `check`, `update`, `sync`, `why`, `pack`.
- Stable native harness surface: `test`, `artifact`, `bench`, `doom`.
- Experimental surface: `inspect`.


- `tspack format` and `tspack lint` are Biome-backed lifecycle UX commands. See `docs/format-lint.md`.

- `tspack doctor` adds non-mutating environment diagnostics. See `docs/doctor.md`.

## `tspack update <query>`

- Targets declared dependency intents only (not arbitrary transitive lock packages).
- Query matching order:
  1. dependency key exact match
  2. npm package name exact match
  3. `npm:<name>` exact match
- Non-selected declared npm roots are preserved from the existing lock when possible.
- `--dry-run` and `--json` compose with targeted update.
