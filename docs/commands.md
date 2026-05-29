# Command inventory (M32)

| Command | Purpose | Mutates manifest/lock? | Notable non-goals | Details |
|---|---|---|---|---|
| `tspack init` | Scaffold a starter manifest and entry source for `library` or `app`. | **Yes (files)** / No | Does not install, update lock, sync, or build outputs. | `docs/init.md` |
| `tspack check` | Validate manifest/frontend, graph, boundaries, type surfaces, and lock consistency when lock exists. Supports `--explain <file>` for boundary debugging. | No / No | Does not resolve, install packages, or mutate project state in explain mode. | `docs/contract.md`, `docs/boundaries.md` |
| `tspack update` | Resolve sources, fetch required package artifacts into the content-addressed store, and then write deterministic `ts-lock.toml`. Supports `--dry-run` plan mode and `--quiet` progress suppression. | No / **Yes (lock)** | Does not execute lifecycle scripts or run npm/npx; prepares lock+store for sync. | `docs/lockfile.md`, `docs/source-resolvers.md` |
| `tspack sync` | Materialize compatibility `node_modules` from lock/store artifacts prepared by `tspack update`. | No / No | Does not re-resolve versions or mutate the lockfile. | `docs/materialization.md` |
| `tspack why` | Explain why a dependency, target, or lock package is present, with deduplicated lock edges, lock-ID guidance for transitive matches, and `--json` for structured reports. | No / No | Not a resolver/editor command. | `docs/why.md` |
| `tspack how` | Explain diagnostic codes and remediation guidance. | No / No | Does not mutate project state or resolve packages. | `docs/how.md` |
| `tspack outdated` | Report declared dependencies with current/wanted/latest npm freshness data (`--json` supported). | No / No | Read-only query; no lock/store/node_modules mutation. | `docs/outdated.md` |
| `tspack pack` | Create deterministic package archives all-or-nothing for the selected package set; `--dry-run` validates and prints the plan without writing; `--verify` structurally verifies produced npm artifacts before finalizing them. | No / No | Not a build pipeline or publish command; include patterns that match nothing are errors. | `docs/pack.md` |
| `tspack run [target]` | Start declared manifest `RunTargets`, apply repeatable child env overlays with `--env KEY=VALUE`, wait for HTTP/TCP/stdout-match readiness, list targets with `--list [--json]`, scope selection with `--package <name>`, run one-shot checks with `--once`, and support explicit manifests with `--manifest <path>`; status stays on stderr. | No / No | Not `npm run`; no package.json script inference; declared cwd controls workspace-root vs package-root command resolution. | `docs/run.md` |
| `tspack test` | Run test backends (native xTest and/or Vitest); supports `--list`, `--filter <text>`, `--json`, native `--compact`, native `--watch`, native `--batch`, native `--update-snapshots`, and `--xtest-bridge <path>` for explicit native bridge resolution. | No / No | Not a generic task runner, affected-test engine, HMR server, or interactive watch UI. | `docs/test-command.md`, `docs/native-test-harness.md` |
| `tspack artifact` | Run standalone native suite artifacts. | No / No | Not package artifact packing. | `docs/artifacts.md` |
| `tspack bench` | Run native benchmark units (`*.benchmark.tsx`). | No / No | Not a general profiling framework. | `docs/benchmarks.md` |
| `tspack doom` | Run quarantined prophecy/doom units (`*.prophecy.tsx`). | No / No | Not a generic chaos platform. | `docs/doom.md` |
| `tspack inspect <url\|target>` | Structural UI inspection and run-target inspection (experimental backends: platform-webview scaffold, CDP, host-path, Playwright Chromium). | No / No | Not screenshot diffing/visual testing; not auto-attach. | `docs/inspect.md` (**experimental**) |

## `tspack update`

- `tspack update` resolves dependency intent, fetches required npm artifacts into the content-addressed store, and writes deterministic `ts-lock.toml` only after required store population succeeds.
- It does not materialize `node_modules`; `tspack sync` consumes the lock/store state after update.
- Text-mode progress is written to **stderr** so stdout remains reserved for human diff output or JSON payloads, depending on mode.
- `tspack update --quiet` suppresses progress/status lines while leaving diagnostics and errors on stderr.


## `tspack check --explain <file>`

- `tspack check --explain <file>` explains how check-time boundary analysis sees one source file.
- Explain mode is read-only: it reads the manifest and source files, scans imports, and prints an explanation without mutating the manifest, lockfile, store, or `node_modules`.
- The output lists target reachability paths, boundary rules whose `from` pattern matches the physical source file, relative imports with resolved paths when available, and external import allow/deny decisions.
- `--explain` requires exactly one file path under the project root and supports `.ts`, `.tsx`, `.js`, and `.jsx` files.
- `tspack check --explain <file> --json` writes only the explain JSON payload to stdout using two-space indentation.

## `tspack check --json`

- `tspack check --json` writes a machine-readable JSON report to **stdout**.
- The JSON report includes command metadata, `ok`, summary counts (`errors`, `warnings`, `info`, `total`), and ordered diagnostics.
- In JSON mode, human-readable diagnostics are not mixed into stdout.
- Exit behavior is unchanged:
  - warning diagnostics (including lock version conflicts) keep exit `0` when no errors exist;
  - exit `0` when there are no error diagnostics (warnings-only remains `0`);
  - nonzero when one or more error diagnostics exist, or on fatal runtime/command failure.

## `tspack update` progress

- Text-mode `tspack update` writes simple status lines to **stderr** for resolve, store population/fetch, lockfile write, and completion phases.
- Progress is deliberately plain text: no spinner, progress bar, ANSI control sequence, terminal-width logic, or interactive UI.
- Normal human stdout remains the existing lockfile diff output; progress is not written to stdout.
- `tspack update <query>` starts with targeted context such as `updating target dependency: react` before the shared update phases.
- `tspack update --quiet` suppresses progress/status lines while leaving diagnostics and errors on stderr.

## `tspack update --dry-run`

- `tspack update --dry-run` resolves the would-be lockfile state and prints a deterministic package-level diff.
- Text-mode dry runs write progress to **stderr** for resolve, lockfile-diff computation, and dry-run completion.
- `tspack update <query> --dry-run` starts with targeted planning context such as `planning targeted update: react`.
- `tspack update --dry-run --json` keeps stdout as structured JSON only and suppresses progress by default.
- It may fetch registry metadata to resolve versions, but does **not** write `ts-lock.toml`, does **not** populate store artifacts, and does **not** materialize `node_modules`.
- It exits `0` on successful planning regardless of whether changes are present; resolver/runtime errors remain non-zero.

## `tspack outdated`

- `tspack outdated` reports declared dependency freshness using registry metadata only.
- `--json` writes structured freshness results to stdout.
- It does not fetch package tarballs, populate the store, write the lockfile, or materialize `node_modules`.

## Stability

- Stable core package surface: `check`, `update`, `sync`, `outdated`, `why`, `how`, `pack`.
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

### `tspack test --update-snapshots`

Native xTest snapshot update mode writes missing golden files and replaces mismatched golden files for selected tests. It is explicit, native-xTest-only, not forwarded in `--list` mode, and unsupported for the Vitest backend.

### `tspack test --batch`

Native xTest batch mode runs test files concurrently with an automatic worker count, preserves deterministic report order, and keeps tests inside each file sequential. It composes with native `--filter`, `--compact`, `--update-snapshots`, `--json`, `--root`, `--watch`, and `--xtest-bridge`; `--list --batch` remains static discovery, and Vitest rejects batch with `TSPACK_TEST_BATCH_UNSUPPORTED_BACKEND`.
