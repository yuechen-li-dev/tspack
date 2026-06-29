# Command inventory (M32)

| Command | Purpose | Mutates manifest/lock? | Notable non-goals | Details |
|---|---|---|---|---|
| `tspack init` | Scaffold a starter manifest and entry source for `library` or `app`. | **Yes (files)** / No | Does not install, update lock, sync, or build outputs. | `docs/init.md` |
| `tspack migrate` | Convert package.json metadata into a reviewable `manifest.migrated.tsx` draft and `tspack-migration.md` report, including package-lock evidence, source scan evidence, script classification, report-only RunTarget suggestions, and optional `--check` structural validation. Dry-run by default; `--write` creates files. | **Yes with `--write` (migration outputs only)** / No | `--check` validates the generated draft with the manifest frontend and Go IR validator, but does not overwrite `manifest.tsx`, mutate package.json, translate lockfiles to TSPack locks, mutate source imports, install, execute scripts, migrate npm scripts into active RunTargets, generate `ts-lock.toml`, or run update/sync/check. | `docs/migrate.md` |
| `tspack adopt --security` | Read observed npm lifecycle/security metadata from `package.json`, `package-lock.json`, and installed package metadata when available. | No | Read-only visibility only. Does not run npm, execute lifecycle scripts, fetch registry metadata, generate lockfiles, or make manifest policy decisions. | `docs/design/incremental-adoption.md` |
| `tspack check [--json] [--format]` | Validate manifest/frontend, graph, boundaries, type surfaces, and lock consistency when lock exists. Supports `--explain <file>` for boundary debugging and optional read-only format validation with `--format`. | No / No | Does not resolve, install packages, or mutate project state in explain mode. | `docs/contract.md`, `docs/boundaries.md` |
| `tspack update` | Resolve sources, fetch required package artifacts into the content-addressed store, and then write deterministic `ts-lock.toml`. Supports `--dry-run` plan mode and `--quiet` progress suppression. | No / **Yes (lock)** | Does not execute lifecycle scripts or run npm/npx; prepares lock+store for sync. | `docs/lockfile.md`, `docs/source-resolvers.md` |
| `tspack sync` | Materialize compatibility `node_modules` from `ts-lock.toml`, hydrating missing local store artifacts from locked sources when needed. | No / No | Does not re-resolve versions or mutate the lockfile. | `docs/materialization.md` |
| `tspack why` | Explain why a dependency, target, lock package, or observed npm package is present, with package-lock-backed observed npm chains before migration, `--reverse` for TSPack lock reverse paths, and `--json` for structured reports. | No / No | Not a resolver/editor command. | `docs/why.md` |
| `tspack how` | Explain diagnostic codes and remediation guidance. | No / No | Does not mutate project state or resolve packages. | `docs/how.md` |
| `tspack outdated` | Report declared dependencies with current/wanted/latest npm freshness data (`--json` supported). | No / No | Read-only query; no lock/store/node_modules mutation. | `docs/outdated.md` |
| `tspack pack` | Create deterministic package archives all-or-nothing for the selected package set; `--dry-run` validates and prints the plan without writing; `--verify` structurally verifies produced npm artifacts before finalizing them. | No / No | Not a build pipeline or publish command; include patterns that match nothing are errors. | `docs/pack.md` |
| `tspack run [target]` | Start declared manifest `RunTargets`, treat targets with `ready` as server targets and targets without `ready` as finite commands, apply repeatable child env overlays with `--env KEY=VALUE`, list targets with `--list [--json]`, scope selection with `--package <name>`, run readiness proofs with `--once`, and support explicit manifests with `--manifest <path>`; status stays on stderr. | No / No | Not `npm run`; no package.json script inference; declared cwd controls workspace-root vs package-root command resolution. | `docs/run.md` |
| `tspack test` | Run test backends (native xTest and/or Vitest); supports `--list`, `--filter <text>`, `--json`, native `--compact`, native `--watch`, native `--batch`, native `--update-snapshots`, and `--xtest-bridge <path>` for explicit native bridge resolution. | No / No | Not a generic task runner, affected-test engine, HMR server, or interactive watch UI. | `docs/test-command.md`, `docs/native-test-harness.md` |
| `tspack artifact` | Run standalone native suite artifacts. | No / No | Not package artifact packing. | `docs/artifacts.md` |
| `tspack bench` | Run native benchmark units (`*.benchmark.tsx`). | No / No | Not a general profiling framework. | `docs/benchmarks.md` |
| `tspack doom` | Run quarantined prophecy/doom units (`*.prophecy.tsx`). | No / No | Not a generic chaos platform. | `docs/doom.md` |
| `tspack inspect <url\|target>` | Structural UI inspection and run-target inspection (experimental backends: platform-webview scaffold, CDP, host-path, Playwright Chromium/WebKit). | No / No | Not screenshot diffing/visual testing; not auto-attach. | `docs/inspect.md` (**experimental**) |
| `tspack doctor runtime` | Report the selected workspace runtime profile (`nodejs`, `bun`, or `deno`), selected executable availability, TSPack lifecycle ownership, and `packageManagerDelegated: false`. | No / No | Does not install runtimes, switch package managers, or rewrite RunTargets. | `docs/doctor.md`, `docs/runtime-profiles.md` |

## `tspack update`

- `tspack update` resolves dependency intent, fetches required npm artifacts into the content-addressed store, and writes deterministic `ts-lock.toml` only after required store population succeeds.
- It does not materialize `node_modules`; `tspack sync` consumes the lock/store state after update.
- Text-mode progress is written to **stderr** so stdout remains reserved for human diff output or JSON payloads, depending on mode.
- `tspack update --quiet` suppresses progress/status lines while leaving diagnostics and errors on stderr.

## `tspack sync`

- `tspack sync` materializes the dependency reality already recorded in `ts-lock.toml`.
- On fresh machines or CI runners, if a required local store artifact is missing, sync hydrates it from the locked source before materialization.
- Hydration is lock-driven, not resolver-driven: sync uses the locked package identity and verification data, does not pick newer versions, and does not rewrite `ts-lock.toml`.
- If the artifact is already present and verifies locally, sync does not refetch it.
- `tspack sync` may need network access when the local store is empty and a locked npm artifact must be downloaded.


## `tspack check --explain <file>`

- `tspack check --explain <file>` explains how check-time boundary analysis sees one source file.
- Explain mode is read-only: it reads the manifest and source files, scans imports, and prints an explanation without mutating the manifest, lockfile, store, or `node_modules`.
- The output lists target reachability paths, boundary rules whose `from` pattern matches the physical source file, relative imports with resolved paths when available, and external import allow/deny decisions.
- `--explain` requires exactly one file path under the project root and supports `.ts`, `.tsx`, `.js`, and `.jsx` files.
- `tspack check --explain <file> --json` writes only the explain JSON payload to stdout using two-space indentation.

## `tspack check --format`

- `tspack check --format` runs normal project validation, then runs a read-only Biome format validation scoped to project source paths such as manifests, declared target entry directories, and package `src/` directories.
- The format scope is `.` relative to the selected project root; `check` does not accept format path arguments.
- `--root <root>` controls both normal project validation paths and Biome backend/config discovery.
- `--manifest <path>` controls normal manifest loading only; format validation still checks the selected root directory rather than only the manifest file.
- The command never writes formatting changes. Run `tspack format` to apply formatting.
- A source format failure is reported as `TSPACK_FORMAT_CHECK_FAILED` and makes the overall check exit nonzero. Missing formatter backend under `check --format` is reported as `TSPACK_FORMAT_BACKEND_MISSING` with the underlying backend diagnostic in details.
- `tspack doctor format` can be used separately to inspect Biome backend and config readiness.

## `tspack check --json`

- `tspack check --json` writes a machine-readable JSON report to **stdout**.
- The JSON report includes command metadata, `ok`, summary counts (`errors`, `warnings`, `info`, `total`), and ordered diagnostics.
- In JSON mode, human-readable diagnostics are not mixed into stdout. With `--format`, Biome output is captured and ANSI/control escape sequences are stripped before diagnostic details are emitted, so stdout remains JSON-only; format failures are represented as normal check diagnostics.
- Exit behavior is unchanged:
  - warning diagnostics (including lock version conflicts) keep exit `0` when no errors exist;
  - exit `0` when there are no error diagnostics (warnings-only remains `0`);
  - nonzero when one or more error diagnostics exist, or on fatal runtime/command failure.

## `tspack update` progress

- Text-mode `tspack update` writes simple status lines to **stderr** for resolve, bounded-parallel store population, lockfile write, and completion phases. Workers never print per-package output directly, so human output remains deterministic.
- Progress is deliberately plain text: no spinner, progress bar, ANSI control sequence, terminal-width logic, or interactive UI.
- Normal human stdout remains the existing lockfile diff output; progress is not written to stdout.
- `tspack update <query>` starts with targeted context such as `updating target dependency: react` before the shared update phases.
- `tspack update --quiet` suppresses progress/status lines while leaving diagnostics and errors on stderr.

## `tspack update --dry-run`

- `tspack update --dry-run` resolves the would-be lockfile state and prints a deterministic package-level diff.
- Text-mode dry runs write progress to **stderr** for resolve, lockfile-diff computation, and dry-run completion.
- `tspack update <query> --dry-run` starts with targeted planning context such as `planning targeted update: react`.
- `tspack update --dry-run --json` keeps stdout as structured JSON only and suppresses progress by default. Dry-run JSON includes `dryRun.enabled`, boolean `dryRun.changed`, and numeric `dryRun.summary` counts for full and targeted updates; the top-level `changed` and `summary` fields remain available for compatibility.
- It may fetch registry metadata to resolve versions, but does **not** write `ts-lock.toml`, does **not** populate store artifacts, and does **not** materialize `node_modules`.
- It exits `0` on successful planning regardless of whether changes are present; resolver/runtime errors remain non-zero.

## `tspack outdated`

- `tspack outdated` reports declared dependency freshness using registry metadata only.
- Default human output groups identical declarations and shows package count/list to keep monorepos readable.
- `--per-package` restores the expanded one-row-per-declaration view.
- `--json` writes structured freshness results to stdout; grouped `entries` are the default, with declaration-level `dependencies` also present for compatibility.
- Non-registry/workspace dependencies are `not_applicable`, not errors.
- It does not fetch package tarballs, populate the store, write the lockfile, or materialize `node_modules`.

### Future update policy

A future declared update policy or rolling-track model may make dependency update intent more explicit. M49a only documents the bounded targeted-update behavior and does not add a policy DSL.

## Stability

- Stable core package surface: `check`, `update`, `sync`, `outdated`, `why`, `how`, `pack`.
- Stable native harness surface: `test`, `artifact`, `bench`, `doom`.
- Experimental surface: `inspect`.


- `tspack format` and `tspack lint` are Biome-backed lifecycle UX commands. Format/lint diagnostics distinguish check findings (`TSPACK_FORMAT_CHECK_FAILED`, `TSPACK_LINT_CHECK_FAILED`), format write failures (`TSPACK_FORMAT_WRITE_FAILED`), and incomplete safe-fix attempts (`TSPACK_LINT_FIX_INCOMPLETE`). `tspack lint --fix --unsafe` explicitly forwards Biome unsafe fixes; `--unsafe` requires `--fix` and is not supported for format. See `docs/format-lint.md`.

- `tspack doctor` adds non-mutating environment diagnostics, and `tspack doctor security` summarizes lifecycle capability security posture from the manifest and lockfile without executing scripts. See `docs/doctor.md`.

## `tspack update <query>`

- Targets declared dependency intents only (not arbitrary transitive lock packages).
- Query matching order:
  1. dependency key exact match
  2. npm package name exact match
  3. `npm:<name>` exact match
- Targeted updates are bounded to the selected dependency and its resolver closure. Existing package entries outside that selected update closure are preserved semantically, including unrelated multi-version and peer-resolved entries.
- If the selected dependency is already at the wanted locked version and the targeted package diff is empty, the command exits successfully without rewriting `ts-lock.toml`.
- Non-selected declared npm roots are preserved from the existing lock when possible.
- `--dry-run` and `--json` compose with targeted update.

### `tspack test --update-snapshots`

Native xTest snapshot update mode writes missing golden files and replaces mismatched golden files for selected tests. It is explicit, native-xTest-only, not forwarded in `--list` mode, and unsupported for the Vitest backend.

### `tspack test --batch`

Native xTest batch mode runs test files concurrently with an automatic worker count, preserves deterministic report order, and keeps tests inside each file sequential. It composes with native `--filter`, `--compact`, `--update-snapshots`, `--json`, `--root`, `--watch`, and `--xtest-bridge`; `--list --batch` remains static discovery, and Vitest rejects batch with `TSPACK_TEST_BATCH_UNSUPPORTED_BACKEND`.

### Lifecycle script visibility

`update` records supported npm lifecycle scripts (`preinstall`, `install`, `postinstall`, `prepare`, and related pack/publish hooks) as lockfile package capabilities without executing them. `check` warns with `TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT` for each recorded lifecycle script, and `why`/`why --json` include package capabilities in explanations. `sync` materializes packages without running lifecycle commands.

## Inspect CDP target listing JSON

`inspect` supports machine-readable CDP target listing for tools such as the VS Code extension proof of concept:

```sh
tspack inspect --cdp http://127.0.0.1:9229 --list-targets --json
```

The JSON output identifies the command and mode, echoes the CDP endpoint, and includes inspectable targets plus diagnostics:

```json
{
  "command": "inspect",
  "mode": "list-targets",
  "cdp": "http://127.0.0.1:9229",
  "endpoint": "http://127.0.0.1:9229",
  "targets": [
    {
      "index": 0,
      "type": "page",
      "url": "vscode-file://...",
      "title": "...",
      "id": "..."
    }
  ],
  "diagnostics": []
}
```

## Runtime profile doctor

`tspack doctor runtime [--root .] [--json]` reports the workspace runtime profile selected by `<Workspace runtime="...">`, the executable that would represent that profile (`node`, `bun`, or `deno`), PATH availability for the selected runtime only, and the TSPack lifecycle ownership note. It does not install dependencies or delegate to package managers. Explicit `RunTarget.runtime: "bun"` and `RunTarget.runtime: "deno"` are separate run-command launch adapters that invoke `bun <declared argv>` or `deno <declared argv>`; RunTargets without explicit runtime inherit the workspace runtime profile, while explicit RunTarget runtime still wins.

### Policy update planning

`tspack update --policy --dry-run` produces a read-only, security-gated update plan from the declared `<UpdatePolicy />`. It reuses outdated metadata and policy evaluation, reports ready, needs-review, security-blocked, policy-blocked, unclassified, and not-applicable buckets, and never writes the lockfile. Security gates use existing TSPack lifecycle capability and acknowledgment policy: unacknowledged consumer-install lifecycle scripts block readiness, unacknowledged maintainer-publish lifecycle scripts require review, and exact or lifecycle-category acknowledgments can pass while lifecycle execution remains blocked. `--json` emits a stable `policyPlan` object plus the normalized `dryRun` object with `securityGatesEvaluated: true`, candidate security statuses, `wouldApply`, and ready/securityBlocked/reviewRequired counts. `--policy` requires `--dry-run`; mutation and targeted policy planning remain future work.

### Store population parallelism

`tspack update` keeps resolution and lockfile output deterministic, then populates missing `.tspack/store` artifacts with a bounded worker pool. The default worker count is conservative and bounded by CPU count; set `TSPACK_STORE_JOBS=1` to force sequential store population for debugging or regression comparisons, or set a positive integer such as `TSPACK_STORE_JOBS=4` for local tuning. Invalid non-positive or non-integer values fail the update with a clear diagnostic before store work starts. Dry-run and policy dry-run paths remain read-only and do not populate the store.

### RunTarget environment contracts

`tspack run` honors RunTarget `env` declarations from `manifest.tsx`: required variables are validated before execution, defaults are injected when host values are missing, and secret defaults/values are redacted in diagnostics plus `tspack run --list --json`. The command still runs manifest RunTargets only; it does not execute `package.json` scripts or load `.env` files.


M59b adds RunTarget service requirements with TCP and HTTP preflight checks before `tspack run` exec. `tspack check` validates only declaration shape; future work may add runtime doctor/preflight-only commands, service orchestration, Docker Compose integration, package kind service, NestJS migration, strict env scrubbing, and runtime access tracing.

### `tspack run --preflight-only`

`tspack run <target> --preflight-only` resolves the manifest RunTarget, validates declared `Env(...)` requirements, applies defaults and `--env` overlays, validates readiness URL `${NAME}` placeholders, and checks external `Service(...)` requirements without starting the RunTarget command. It does not poll the target's own readiness URL because no process has been launched.
