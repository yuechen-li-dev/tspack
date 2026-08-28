# Command inventory (M32)

Registry-aware lifecycle commands read project `<RegistryPolicy>` before
network work. `update` and `add` fail offline; `sync` is network-free when the
lock/store are complete; `check` validates locked source/endpoint evidence; and
`why` displays recorded provenance. There is no global registry-management
command.

| Command | Purpose | Mutates manifest/lock? | Notable non-goals | Details |
|---|---|---|---|---|
| `tspack init` | Scaffold a starter manifest and entry source for `library` or `app`. | **Yes (files)** / No | Does not install, update lock, sync, or build outputs. | `docs/init.md` |
| `tspack add <package>` | Author an explicit npm or JSR dependency through the dependency tape, project it into the owned manifest island, and run normal mixed-source update resolution. | **Yes (manifest and lock)** | Does not mutate package.json authority, search registries automatically, accept arbitrary install specs, or execute lifecycle scripts. | `docs/dev/m70b-jsr-add.md` |
| `tspack remove <package>` | Remove one editable native dependency declaration, reveal any lower-precedence winner, project the owned source edit, and run normal update resolution. | **Yes (manifest and lock)** | Does not delete concept/template declarations, mutate package.json authority, or force a package out of the resolved graph. | `docs/dev/m69d-remove.md` |
| `tspack migrate` | Convert package.json metadata into a reviewable `manifest.migrated.tsx` draft and `tspack-migration.md` report, including package-lock evidence, source scan evidence, script classification, report-only RunTarget suggestions, and optional `--check` structural validation. Dry-run by default; `--write` creates files. | **Yes with `--write` (migration outputs only)** / No | `--check` validates the generated draft with the manifest frontend and Go IR validator, but does not overwrite `manifest.tsx`, mutate package.json, translate lockfiles to TSPack locks, mutate source imports, install, execute scripts, migrate npm scripts into active RunTargets, generate `ts-lock.toml`, or run update/sync/check. | `docs/migrate.md` |
| `tspack adopt --check-annotations` | Check package.manifest.tsx annotation consistency against package.json and exit nonzero on errors or warnings. | No | CI-friendly read-only drift check; unannotated dependencies are notices and do not fail. | `docs/design/incremental-adoption.md` |
| `tspack adopt --suggest-package <package-root>` | Print a dry-run `package.manifest.tsx` annotation suggestion for an existing package.json package. | No | Advisory only; writes no manifest, mutates no package.json, and generates no lockfile. | `docs/design/incremental-adoption.md` |
| `tspack adopt --security` | Read observed npm lifecycle/security metadata from `package.json`, `package-lock.json`, and installed package metadata when available, including lifecycle capability warnings and why chains. | No | Read-only visibility only. Does not run npm, execute lifecycle scripts, fetch registry metadata, generate lockfiles, or make manifest policy decisions. | `docs/design/incremental-adoption.md` |
| `tspack check [--json] [--format]` | Validate manifest/frontend, graph, boundaries, type surfaces, and lock consistency when lock exists. Supports `--explain <file>` for boundary debugging and optional read-only format validation with `--format`. | No / No | Does not resolve, install packages, or mutate project state in explain mode. | `docs/contract.md`, `docs/boundaries.md` |
| `tspack update` | Resolve sources, fetch required package artifacts into the content-addressed store, and then write deterministic `ts-lock.toml`. Supports `--dry-run` plan mode and `--quiet` progress suppression. | No / **Yes (lock)** | Does not execute lifecycle scripts or run npm/npx; prepares lock+store for sync. | `docs/lockfile.md`, `docs/source-resolvers.md` |
| `tspack sync` | Materialize compatibility `node_modules` from `ts-lock.toml`, hydrating missing local store artifacts from locked sources when needed. | No / No | Does not re-resolve versions or mutate the lockfile. | `docs/materialization.md` |
| `tspack build [target]` | Topologically build declared compiler targets through tsc, Copeland, bounded ScriptC, or bounded Perry adapters; compiler descriptors preserve tool/config/source/package/artifact provenance. | No / **Yes (build artifacts and metadata)** | Does not mix compiler source semantics or load native compiler artifacts directly into Node. | `docs/dev/m71-compiler-target-boundary.md`, `docs/dev/m71a-scriptc-hotpath.md`, `docs/dev/m71b-perry-performance.md` |
| `tspack why` | Explain why a dependency, target, lock package, or observed npm package is present, with package-lock-backed observed npm chains before migration, `--reverse` for TSPack lock reverse paths, and `--json` for structured reports. | No / No | Not a resolver/editor command. | `docs/why.md` |
| `tspack how` | Explain diagnostic codes and remediation guidance. | No / No | Does not mutate project state or resolve packages. | `docs/how.md` |
| `tspack audit` | Check exact locked npm versions against OSV and report coverage for the whole mixed-source lock. | No / No | Does not treat JSR compatibility names as npm vulnerability identity or claim full coverage for unmapped sources. | `docs/audit.md` |
| `tspack outdated` | Report declared dependencies with current/wanted/latest npm freshness data (`--json` supported). | No / No | Read-only query; no lock/store/node_modules mutation. | `docs/outdated.md` |
| `tspack pack` | Create deterministic package archives all-or-nothing for the selected package set; `--dry-run` validates and prints the plan without writing; `--verify` structurally verifies produced npm artifacts before finalizing them. | No / No | Not a build pipeline or publish command; include patterns that match nothing are errors. | `docs/pack.md` |
| `tspack run [target]` | Start declared manifest `RunTargets`, treat targets with `ready` as server targets and targets without `ready` as finite commands, apply repeatable child env overlays with `--env KEY=VALUE`, list targets with `--list [--json]`, scope selection with `--package <name>`, run readiness proofs with `--once`, and support explicit manifests with `--manifest <path>`; status stays on stderr. | No / No | Not `npm run`; no package.json script inference; declared cwd controls workspace-root vs package-root command resolution. | `docs/run.md` |
| `tspack workflow list\|inspect\|run\|export` | Normalize provider-neutral workflow intent, inspect or execute one deterministic DAG locally, or explicitly export a GitHub thin runner with drift checking. | No, except explicit provider export | Does not import YAML, mirror GitHub fields, shell back into TSPack for native operations, expose secret values, or own deployment topology. | `docs/workflows.md` |
| `tspack test` | Run test backends (native xTest and/or Vitest); supports `--list`, `--filter <text>`, `--json`, native `--compact`, native `--watch`, native `--batch`, native `--update-snapshots`, and `--xtest-bridge <path>` for explicit native bridge resolution. | No / No | Not a generic task runner, affected-test engine, HMR server, or interactive watch UI. | `docs/test-command.md`, `docs/native-test-harness.md` |
| `tspack artifact` | Run standalone native suite artifacts. | No / No | Not package artifact packing. | `docs/artifacts.md` |
| `tspack bench` | Run native benchmark units (`*.benchmark.tsx`). | No / No | Not a general profiling framework. | `docs/benchmarks.md` |
| `tspack doom` | Run quarantined prophecy/doom units (`*.prophecy.tsx`). | No / No | Not a generic chaos platform. | `docs/doom.md` |
| `tspack inspect <url\|target>` | Structural UI inspection and run-target inspection (experimental backends: platform-webview scaffold, CDP, host-path, Playwright Chromium/WebKit). | No / No | Not screenshot diffing/visual testing; not auto-attach. | `docs/inspect.md` (**experimental**) |
| `tspack scenario <scenario.json> --run <target>` | Start a declared target, run bounded browser assertions, capture declared screenshots, then close the browser and target. | No / No | Not an unrestricted browser-script runner or a second test harness. | `docs/scenario.md` |
| `tspack doctor runtime` | Report the selected workspace runtime profile (`nodejs`, `bun`, or `deno`), selected executable availability, TSPack lifecycle ownership, and `packageManagerDelegated: false`. | No / No | Does not install runtimes, switch package managers, or rewrite RunTargets. | `docs/doctor.md`, `docs/runtime-profiles.md` |

## `tspack update`

- `tspack update` resolves dependency intent, fetches required npm and JSR artifacts into the shared content-addressed store, and writes deterministic `ts-lock.toml` only after required store population succeeds.
- Registry selection is per dependency edge. A project and its transitive graph may mix npm and JSR; JSR access is direct and does not require Deno or a package-manager subprocess.
- It does not materialize `node_modules`; `tspack sync` consumes the lock/store state after update.
- Text-mode progress is written to **stderr** so stdout remains reserved for human diff output or JSON payloads, depending on mode.
- Cold store population prints deterministic plain-text lines such as `fetching npm artifacts [3/20] vite@7.1.0`.
- `tspack update --quiet` suppresses progress/status lines while leaving diagnostics and errors on stderr.

## `tspack add`

- `tspack add lodash` selects the newest stable npm release, authors a caret constraint such as `^4.17.21`, and records the exact selected release in `ts-lock.toml` through the normal update path.
- `tspack add lodash@^4` and `tspack add lodash@4.17.21` preserve the explicit constraint exactly.
- `tspack add lodash --source npm` is equivalent to the default and makes the source-qualified identity explicit. `tspack add @std/path --source jsr` selects JSR. Omitting `--source` always means npm; a failed npm lookup is never retried against JSR.
- A successful JSR add reports the stable Node/TypeScript import spelling, such as `@jsr/std__path`, and JSON includes semantic/materialization/import usage. npm adds omit redundant guidance when the spelling is unchanged. The command does not rewrite application imports.
- Scoped forms such as `@scope/pkg` and `@scope/pkg@^3` remain intact for either source. Git, URL, file, workspace, and general install-spec syntax are rejected explicitly.
- `--optional` is orthogonal to the normal TSPack `dep` kind. `--kind peer` authors peer intent. Package selection accepts a stable package name or an exact workspace-relative package root; when run inside one package directory, that package is inferred if the mapping is unambiguous.
- `--dry-run` performs metadata selection, semantic editing, and source projection planning without writing the manifest, lockfile, or store. `--json` exposes stable semantic result fields.
- Repeating an unqualified or textually equivalent explicit add of the same editable declaration is a zero-registry, byte-for-byte no-op. A changed explicit spec replaces the matching editable declaration and is reported as a constraint change; a derived or concept-owned declaration is retained and shadowed by a new explicit declaration.
- `--dev` is intentionally rejected: TSPack's `test` dependency kind remains reserved and has no native manifest helper or execution contract. `--tool` is also rejected for add because a usable tool requires both `tool(...)` dependency intent and a `<Tools>` selection, while the M69 projector owns only dependency islands. This avoids creating apparently installed but unusable tooling.
- npm and JSR declarations with the same logical name remain source-distinct. Replacement matches source-qualified identity, and removal requires `--source` when the name is ambiguous.
- Registry npm aliases and transitive registry peers are normalized by the M70x requirement IR. `tspack add` does not add new direct-alias syntax; aliases encountered in registry metadata retain local reference spelling while resolution uses semantic target identity.
- package.json-native incremental projects receive an authority diagnostic and should use `tspack npm install ...` for npm dependencies until ownership is migrated. TSPack does not invent a package.json representation for native JSR intent.

## `tspack remove`

- `tspack remove lodash` removes one owned, editable declaration from the selected native package. It does not interpret `lodash@^4` as an uninstall query and does not force matching lock entries out of the graph.
- The command rebuilds the authoring tape before source projection. If an explicit override shadowed a concept or template declaration, that lower declaration becomes effective again and is reported with its provenance.
- Package-local direct truth and resolved truth are separate. A declaration can disappear while the artifact remains locked transitively or because another workspace package still requires it.
- Several editable matches are an error. `--optional`, `--source npm|jsr`, and `--kind dep|peer|tool|test` narrow declarations; `--dev` and `--tool` are remove aliases for the corresponding kinds. If npm and JSR declarations share a package name, an unqualified removal reports source ambiguity and requires `--source`. Package selection accepts a stable name or exact workspace-relative package root, with unambiguous current-directory inference.
- Derived/concept-only and repeated removals are no-op operations. Incremental package.json authority is denied with guidance to use `tspack npm uninstall`.
- `--dry-run` performs selection, semantic removal, tape rebuilding, and source projection without writing manifest, lock, or store state. Resolved status is reported only after a committed update or when reading unchanged no-op state. `--json` exposes the semantic fields without raw tape or AST data.

## `tspack sync`

- `tspack sync` materializes the dependency reality already recorded in `ts-lock.toml`.
- On fresh machines or CI runners, if a required local store artifact is missing, sync hydrates it from the locked source before materialization.
- Hydration is lock-driven, not resolver-driven: sync uses the locked package identity and verification data, does not pick newer versions, and does not rewrite `ts-lock.toml`.
- If the artifact is already present and verifies locally, sync does not refetch it.
- `tspack sync` may need network access when the local store is empty and a locked npm or JSR artifact must be downloaded.
- Cold materialization may print deterministic plain-text lines such as `materializing packages [12/20] react@19.2.7`.
- Before writing `node_modules`, sync rejects semantic packages that map to the same destination with `TSPACK_MATERIALIZE_IMPORT_COLLISION`; it never silently chooses or overwrites one source.

## `tspack why` and package usage

`why <reference|package|source:package>` also prints the deterministic
requirement tape for a shared environment slot: origin, semantic target,
constraint, classification, controlling state, and selected version. Alias
queries explain that the local reference points to a different semantic npm
target. `why --json` exposes explicit requirement DTO fields.

A source-qualified registry query retains semantic package identity. For JSR
lock packages, human and JSON output also explain the npm-compat materialization
name and Node/TypeScript import specifier. Mixed pull chains retain the real
parent source, including JSR parents of npm transitives. `tspack how` remains
diagnostic-code documentation and does not duplicate package usage lookup.

## Runtime versions

- TSPack delegates npm package operations to the real npm CLI.
- TSPack does not manage Node.js runtime versions.
- Commands that need Node.js expect an existing runtime on `PATH`.
- Recommended runtime manager: `mise`, https://mise.jdx.dev/


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

`tspack update` keeps resolution and lockfile output deterministic, then populates missing `.tspack/store` artifacts with a bounded worker pool. The default worker count is `24`, which better fits registry I/O than CPU-count-based defaults; set `TSPACK_STORE_JOBS=1` to force sequential store population for debugging or regression comparisons, or set another positive integer such as `TSPACK_STORE_JOBS=32` for local tuning. Invalid non-positive or non-integer values fail the update with a clear diagnostic before store work starts. Dry-run and policy dry-run paths remain read-only and do not populate the store.

When resolution already had to fetch an npm tarball to inspect it, `tspack update` now writes that artifact into the content-addressed store immediately. The later store-population phase verifies what is already present and skips refetching those tarballs.

### RunTarget environment contracts

`tspack run` honors RunTarget `env` declarations from `manifest.tsx`: required variables are validated before execution, defaults are injected when host values are missing, and secret defaults/values are redacted in diagnostics plus `tspack run --list --json`. The command still runs manifest RunTargets only; it does not execute `package.json` scripts or load `.env` files.


M59b adds RunTarget service requirements with TCP and HTTP preflight checks before `tspack run` exec. `tspack check` validates only declaration shape; future work may add runtime doctor/preflight-only commands, service orchestration, Docker Compose integration, package kind service, NestJS migration, strict env scrubbing, and runtime access tracing.

### `tspack run --preflight-only`

`tspack run <target> --preflight-only` resolves the manifest RunTarget, validates declared `Env(...)` requirements, applies defaults and `--env` overlays, validates readiness URL `${NAME}` placeholders, and checks external `Service(...)` requirements without starting the RunTarget command. It does not poll the target's own readiness URL because no process has been launched.
