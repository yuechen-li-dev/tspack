# Diagnostic inventory (M24)

This inventory groups primary diagnostic families by subsystem to keep command-surface expectations aligned with product scope.

## Init diagnostics

- `TSPACK_INIT_KIND_REQUIRED`
- `TSPACK_INIT_INVALID_KIND`
- `TSPACK_INIT_NAME_REQUIRED`
- `TSPACK_INIT_INVALID_NAME`
- `TSPACK_INIT_INVALID_VERSION`
- `TSPACK_INIT_FILE_EXISTS`
- `TSPACK_INIT_WRITE_FAILED`
- `TSPACK_INIT_UNSAFE_ROOT`


## Migrate diagnostics

- `TSPACK_MIGRATE_PACKAGE_JSON_MISSING`: the selected package.json path does not exist. Details include `root` and `packageJsonPath`, with a suggested `--root`/`--package-json` fix.
- `TSPACK_MIGRATE_PACKAGE_JSON_INVALID`: package.json could not be read or parsed as JSON. Details include `root`, `packageJsonPath`, and the parse/read error.
- `TSPACK_MIGRATE_OUTPUT_EXISTS`: an output path exists and `--force` was not passed. Details include `root`, `packageJsonPath`, and `outputPath`; no migration output is written.
- `TSPACK_MIGRATE_WRITE_FAILED`: writing a migration output failed. Details include `root`, `packageJsonPath`, `outputPath`, and the filesystem error.
- `TSPACK_MIGRATE_INVALID_ARGS`: migrate flags are invalid, unknown, missing required values, or combine incompatible options such as `--package-lock` with `--no-lock-evidence`.
- `TSPACK_MIGRATE_PACKAGE_LOCK_MISSING`: an explicit `--package-lock` path does not exist. Implicit missing `<root>/package-lock.json` is not an error.
- `TSPACK_MIGRATE_PACKAGE_LOCK_INVALID`: package-lock evidence could not be parsed. An implicit invalid lock is reported as a warning and ignored; an explicit invalid `--package-lock` path fails migration.
- `TSPACK_MIGRATE_PACKAGE_LOCK_UNSUPPORTED_VERSION`: package-lock evidence used a lockfileVersion outside npm v2/v3. Migrate continues best-effort when a `packages` object is readable.
- `TSPACK_MIGRATE_UNSUPPORTED_PACKAGE_SHAPE`: package shapes are unsupported by the package.json draft generator. Prefer TODOs in the generated draft/report when a partial mechanical draft is possible.
- `TSPACK_MIGRATE_CHECK_TEMP_WRITE_FAILED`: `tspack migrate --check` could not create or write the temporary manifest used for validation. No migration outputs are written.
- `TSPACK_MIGRATE_GENERATED_MANIFEST_INVALID`: the generated draft did not pass manifest frontend parsing/API validation. Details include frontend diagnostics and remaining TODO counts. With `--write --check`, no outputs are written.
- `TSPACK_MIGRATE_GENERATED_IR_INVALID`: the generated draft frontend IR did not pass Go manifest/IR validation. Details include IR diagnostics, `remainingTodos`, and `todosAreErrors: false`; TODO comments are review markers and are not the cause by themselves. Unknown dependency refs may include an alias/key mismatch hint when generated dependency refs do not match declared dependency identities. With `--write --check`, no outputs are written.

## Package/core diagnostics

- `TSPACK_FRONTEND_*`
- `TSPACK_IR_*`
- `TSPACK_MANIFEST_INVALID_RUNTIME_PROFILE`: workspace `runtime` was not one of `nodejs`, `bun`, or `deno`; package manager names such as `npm`, `pnpm`, and `yarn` are not runtime profiles.
- `TSPACK_GRAPH_*`
- `TSPACK_IMPORTSCAN_*`
- `TSPACK_BOUNDARY_*`
- `TSPACK_TYPESURFACE_*`
- `TSPACK_LOCK_*`
- `TSPACK_RESOLVE_*`
- `TSPACK_STORE_*`
- `TSPACK_MATERIALIZE_*`
- `TSPACK_PROJECT_*`
- `TSPACK_CHECK_*`
- `TSPACK_SYNC_*`
- `TSPACK_PACK_*`
- `TSPACK_WHY_*`
- `TSPACK_CAPABILITY_*`

### Structured check report

- `tspack check --json` emits a structured diagnostics report on stdout.
- `tspack how <code>` explains a diagnostic code from JSON/CI output, and `tspack how --list` lists curated help entries.
- Each diagnostic includes stable `code`, `severity`, and `message` fields, with optional file/details/fixes when available.
- Boundary diagnostics may include reachable import-chain details in `details`, such as a path from the target entry through the importing file to the external package. `transitiveFrom` explicit-deny diagnostics include `transitiveFrom`, `seed`, and a seed-to-import `path`.
- `tspack check --json` preserves diagnostic details structurally for tooling; human diagnostic text is not mixed into stdout in JSON mode.
- Human CLI output prints diagnostic detail lines under `CODE: message` with indentation for easier resolver/store debugging. By default, human `tspack check` summarizes noisy warning families when there are two or more `TSPACK_LOCK_VERSION_CONFLICT` or `TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT` diagnostics; use `--show-conflicts` or `--show-lifecycle` to reveal the individual human diagnostic blocks.
- Severity is preserved (`error`, `warning`, `info`), and warning diagnostics are included in the report summary.
- `ok` is `false` when one or more `error` diagnostics exist, otherwise `true`.
- `TSPACK_LOCK_VERSION_CONFLICT` warns when one source ecosystem/package name appears at multiple locked versions (for example two npm react versions). Human `tspack check` summarizes this warning family by default when multiple packages conflict, including a deterministic example list and `--show-conflicts` reveal guidance. `tspack check --json` still emits every individual conflict diagnostic.
- `TSPACK_WHY_NOT_FOUND` may include detail lines with matching lock package IDs and suggested `tspack why npm:<name>@<version>` queries when a bare package name only matches transitive lock entries. Multiple suggestions are sorted and scoped package IDs use the same lock ID form, for example `npm:@scope/pkg@1.2.3`. In reverse mode, not-found means no locked package matched the reverse query and the details point users back to normal `tspack why <declared-dep>` for manifest declarations.
- `tspack why --json` preserves why diagnostics, details, and suggestions in the JSON `diagnostics` array; handled diagnostic paths keep stdout parseable and do not print human diagnostic text to stderr.
- `TSPACK_WHY_LOCKFILE_MISSING` is warning-only for normal `tspack why`, which can still answer from manifest declarations. It is an error for `tspack why --reverse` because reverse explanations require lockfile edges; run `tspack update` first.

### Boundary validation diagnostics

- `TSPACK_BOUNDARY_INVALID_SCOPE`: a boundary row specified incompatible scope fields, such as both `from` and `transitiveFrom`.
- `TSPACK_BOUNDARY_INVALID_TRANSITIVE_FROM`: a `transitiveFrom` value was empty, absolute, parent-traversing, or otherwise not a safe relative exact path or glob-like pattern.

## Harness diagnostics

- `TSPACK_TEST_*`
  - `TSPACK_TEST_THEORY_NO_CASES`: a `<Theory>` has a callback body but no direct `<Case />` children. Add at least one direct `<Case />` child.
  - `TSPACK_TEST_THEORY_MISSING_BODY`: a `<Theory>` has cases but no callback body. Add exactly one callback body directly under the theory.
  - `TSPACK_TEST_THEORY_DUPLICATE_BODY`: a `<Theory>` declares more than one callback body. Keep exactly one callback body.
  - `TSPACK_TEST_INVALID_THEORY_STRUCTURE`: a `<Theory>` contains an unsupported direct child or non-callback expression.
  - `TSPACK_TEST_WATCH_UNSUPPORTED_BACKEND`: `tspack test --watch` was requested for a backend other than native xTest, such as Vitest. Native watch mode does not proxy Vitest watch behavior.
  - `TSPACK_TEST_WATCH_INVALID_MODE`: watch mode was combined with a static or automation-oriented mode such as `--list` or `--json`. Run those modes without `--watch`.
  - `TSPACK_TEST_WATCH_FAILED`: watch mode could not scan or poll the project root. Details include the filesystem error.
  - `TSPACK_TEST_BATCH_UNSUPPORTED_BACKEND`: `tspack test --batch` was requested for a backend other than native xTest, such as Vitest. M34f does not proxy Vitest parallelism.
  - `TSPACK_TEST_TYPECHECK_FAILED`: native xTest could not build or load the TypeScript source program for the static type assertion lane.
  - `TSPACK_TYPE_ASSERTION_FAILED`: an `assert.type<TExpected>(value, reason)` call failed TypeScript assignability checking. The diagnostic includes the reason, expected type text, and TypeScript diagnostic details when available.
  - `TSPACK_TYPE_ASSERTION_REASON_REQUIRED`: an `assert.type` call is missing a non-empty string literal reason.
- `TSPACK_ARTIFACT_*`
- `TSPACK_BENCH_*`
- `TSPACK_DOOM_*`

## Run diagnostics

- `TSPACK_RUN_*` (target resolution, package scoping, cwd validation/root resolution, listing argument validation, launch, readiness, timeout, process lifecycle)
  - `TSPACK_RUN_INVALID_CWD`: a RunTarget `cwd` value was not `workspace` or `package`.
  - `TSPACK_RUN_PACKAGE_ROOT_UNKNOWN`: a RunTarget requested `cwd: "package"`, but the declaring package root could not be resolved.
  - `TSPACK_RUN_INVALID_ENV`: `--env` was missing its `KEY=VALUE` value, omitted `=`, used an empty key, or used a key outside `[A-Za-z_][A-Za-z0-9_]*`.

## Inspect diagnostics (experimental)

Target/input:
- `TSPACK_INSPECT_TARGET_REQUIRED`
- `TSPACK_INSPECT_INVALID_TARGET`
- `TSPACK_INSPECT_INVALID_TARGET_OPTIONS`
- `TSPACK_INSPECT_RUN_TARGET_MISSING`
- `TSPACK_INSPECT_RUN_TARGET_NOT_FOUND`

Browser/backend:
- `TSPACK_INSPECT_BROWSER_UNSUPPORTED`
- `TSPACK_INSPECT_BROWSER_LAUNCH_FAILED`
- `TSPACK_INSPECT_BRIDGE_MISSING`
- `TSPACK_INSPECT_FAILED`
- `TSPACK_INSPECT_INVALID_BACKEND_OPTIONS`

CDP:
- `TSPACK_INSPECT_CDP_ENDPOINT_REQUIRED`
- `TSPACK_INSPECT_CDP_ENDPOINT_INVALID`
- `TSPACK_INSPECT_CDP_CONNECT_FAILED`
- `TSPACK_INSPECT_CDP_TARGET_NOT_FOUND`
- `TSPACK_INSPECT_CDP_TARGET_AMBIGUOUS`
- `TSPACK_INSPECT_CDP_TARGET_UNSUPPORTED`
- `TSPACK_INSPECT_CDP_EVALUATION_FAILED`

Platform webview:
- `TSPACK_INSPECT_PLATFORM_WEBVIEW_UNAVAILABLE`
- `TSPACK_INSPECT_PLATFORM_WEBVIEW_INIT_FAILED`
- `TSPACK_INSPECT_PLATFORM_WEBVIEW_EVALUATION_FAILED`
- `TSPACK_INSPECT_PLATFORM_WEBVIEW_UNSUPPORTED_OS`

Installed host:
- `TSPACK_INSPECT_HOST_PATH_NOT_FOUND`
- `TSPACK_INSPECT_HOST_PATH_INVALID`
- `TSPACK_INSPECT_HOST_LAUNCH_FAILED`
- `TSPACK_INSPECT_HOST_CDP_ENDPOINT_FAILED`
- `TSPACK_INSPECT_HOST_CLEANUP_FAILED`

Playwright Core provider:
- `TSPACK_INSPECT_PLAYWRIGHT_CORE_NOT_FOUND`
- `TSPACK_INSPECT_PLAYWRIGHT_CORE_LOAD_FAILED`

Page/analyzer:
- `TSPACK_INSPECT_PAGE_LOAD_FAILED`
- `TSPACK_INSPECT_ANALYSIS_FAILED`
- `TSPACK_INSPECT_SELECTOR_NOT_FOUND`
- `TSPACK_INSPECT_INVALID_VIEWPORT`
- `TSPACK_INSPECT_INVALID_POINT`


## Format and lint diagnostics

`tspack format` and `tspack lint` are Biome-backed lifecycle UX commands. See `docs/format-lint.md`.

- `TSPACK_BIOME_BACKEND_NOT_FOUND`: TSPack could not find a Biome backend in the local `.bin`, direct `@biomejs/biome` package binary, or `PATH`.
- `TSPACK_BIOME_COMMAND_FAILED`: the Biome backend failed to start, was terminated by signal, or failed in an unmapped infrastructure path.
- `TSPACK_FORMAT_INVALID_FLAGS`: `tspack format` received unsupported or malformed flags, including `--unsafe` because format has no unsafe behavior in TSPack.
- `TSPACK_FORMAT_CHECK_FAILED`: `tspack format --check` or `tspack check --format` found files that would change. Run `tspack format` to apply formatting.
- `TSPACK_FORMAT_WRITE_FAILED`: `tspack format` failed while asking Biome to write formatting changes.
- `TSPACK_LINT_INVALID_FLAGS`: `tspack lint` received unsupported or malformed flags, including `--unsafe` without `--fix`.
- `TSPACK_LINT_CHECK_FAILED`: `tspack lint` reported lint violations. Run `tspack lint --fix` to apply safe fixes where possible.
- `TSPACK_LINT_FIX_INCOMPLETE`: `tspack lint --fix` may have applied safe fixes, or safe and unsafe fixes when `--unsafe` was explicitly present, but violations remain. Unsafe fixes are not applied by default.

- `tspack doctor` adds non-mutating environment diagnostics. See `docs/doctor.md`.

## Materializer bin diagnostics (M28)

- `TSPACK_MATERIALIZE_BIN_INVALID`
- `TSPACK_MATERIALIZE_BIN_TARGET_MISSING`
- `TSPACK_MATERIALIZE_BIN_CONFLICT`
- `TSPACK_MATERIALIZE_BIN_WRITE_FAILED`


## Native import loader diagnostics (M29)

- `TSPACK_TEST_IMPORT_NOT_FOUND`
- `TSPACK_TEST_IMPORT_OUTSIDE_ROOT`
- `TSPACK_TEST_UNSUPPORTED_IMPORT`
- `TSPACK_TEST_MODULE_TRANSPILE_FAILED`

## Update target diagnostics

- `TSPACK_UPDATE_TARGET_NOT_FOUND`: no declared dependency key or npm package matched the query.
- `TSPACK_UPDATE_TARGET_AMBIGUOUS`: query matched multiple incompatible declared dependencies.
- `TSPACK_UPDATE_TARGET_UNSUPPORTED_SOURCE`: query selected a non-npm declared dependency kind.
- `TSPACK_UPDATE_TARGET_LOCK_MISSING`: non-selected root dependency had no prior lock pin and may refresh.
- `TSPACK_UPDATE_TARGET_RESOLUTION_FAILED`: targeted resolution failed.

## Check explain diagnostics

- `TSPACK_CHECK_EXPLAIN_FILE_REQUIRED`: `--explain` was supplied without exactly one file path.
- `TSPACK_CHECK_EXPLAIN_FILE_NOT_FOUND`: the requested explain file does not exist.
- `TSPACK_CHECK_EXPLAIN_FILE_OUTSIDE_ROOT`: the requested explain path resolves outside the project root.
- `TSPACK_CHECK_EXPLAIN_UNSUPPORTED_FILE`: the requested explain path is not a supported `.ts`, `.tsx`, `.js`, or `.jsx` source file.
- `TSPACK_CHECK_EXPLAIN_FAILED`: explain mode failed before it could produce a stable explanation.

## M33e allowOnly boundary diagnostics

- `TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION`: an external runtime import matched a boundary row with `allowOnly`, but the package was not listed in that row. Details include `package`, `import`, `boundary`, `allowOnly`, and `path`; transitive rows also include `transitiveFrom` and `seed`.
- `TSPACK_BOUNDARY_INVALID_ALLOW_ONLY`: a manifest IR boundary row contained an invalid `allowOnly` value, such as an empty string entry.

## M33f type boundary diagnostics

- `TSPACK_BOUNDARY_TYPE_EXPLICIT_DENY`: a scanner-visible type-only external import or re-export matched a boundary row's `denyTypeDeps`. Details include `package`, `import`, `boundary`, `denyTypeDeps`, and `path`; transitive rows also include `transitiveFrom` and `seed`.
- `TSPACK_BOUNDARY_INVALID_DENY_TYPE_DEPS`: a manifest IR boundary row contained an invalid `denyTypeDeps` value, such as an empty string entry.

## Native xTest inspect assertions

- `TSPACK_ASSERT_INSPECT_EXISTS_FAILED`: `assert.inspect.exists` received a null or undefined node.
- `TSPACK_ASSERT_INSPECT_VISIBLE_FAILED`: `assert.inspect.visible` received a missing node or a node whose browser-computed `visible` field is not `true`.
- `TSPACK_ASSERT_INSPECT_HIDDEN_FAILED`: `assert.inspect.hidden` received a missing node or a node whose browser-computed `visible` field is not `false`.
- `TSPACK_ASSERT_INSPECT_ROLE_FAILED`: `assert.inspect.role` found a role different from the expected role.
- `TSPACK_ASSERT_INSPECT_NAME_FAILED`: `assert.inspect.name` found an accessible name different from the expected name.
- `TSPACK_ASSERT_INSPECT_BOUNDS_FAILED`: `assert.inspect.boundsWithin` received missing bounds or bounds that violate one or more supplied min/max `x`, `y`, `width`, or `height` constraints.
- `TSPACK_ASSERT_INSPECT_HIT_FAILED`: `assert.inspect.hitIncludes` did not find an element in the collected hit-test stack matching all supplied `role`, `name`, and `tag` fields.
- `TSPACK_ASSERT_INSPECT_SOURCE_FAILED`: `assert.inspect.source` received missing source metadata or source fields that differ from the expected `file`, `component`, or `symbol` fields. This assertion does not validate files on disk.

Inspect assertion diagnostics include the assertion reason, expected condition, compact actual node or hit-test facts, and details such as failed bounds constraints or source fields without dumping the full inspect subtree.

## Native xTest snapshots

- `TSPACK_SNAPSHOT_INVALID_NAME`: a snapshot name is empty, path-like, traversal-like, starts with `.`, contains separators, or contains characters outside letters, numbers, `_`, `-`, and `.`.
- `TSPACK_SNAPSHOT_MISSING`: a snapshot assertion ran in read-only mode and the expected `.snap.txt` or `.snap.json` file does not exist. Run `tspack test --update-snapshots` to create it.
- `TSPACK_SNAPSHOT_MISMATCH`: a snapshot file exists but differs from the normalized actual value. Details include the snapshot path, expected and actual hashes, and the first differing line.
- `TSPACK_SNAPSHOT_WRITE_DISABLED`: a snapshot assertion ran without native xTest file execution context.
- `TSPACK_SNAPSHOT_WRITE_FAILED`: update mode attempted to write a snapshot but the filesystem write failed.
- `TSPACK_SNAPSHOT_TEXT_VALUE_INVALID`: `expect.snapshotText` received a non-string value.
- `TSPACK_SNAPSHOT_JSON_UNSUPPORTED`: `expect.snapshotJson` received an unsupported value such as `undefined`, a function, bigint, non-finite number, circular reference, or non-plain object.
- `TSPACK_SNAPSHOT_UNSUPPORTED_BACKEND`: `--update-snapshots` was requested for a backend other than native xTest.


## Run target diagnostics

- `TSPACK_RUN_PACKAGE_NOT_FOUND`: `--package <name>` did not match a package in the loaded manifest; output includes known packages.
- `TSPACK_RUN_TARGET_AMBIGUOUS`: the requested target name or default selection matched multiple package-qualified run targets; output includes candidates such as `@prisma-ui/demo:dev` and hints to use `--package <name>`.
- `TSPACK_RUN_TARGET_NOT_FOUND`: the requested run target does not exist. With `--package`, output includes the selected package and its known targets.
- `TSPACK_RUN_INVALID_ARGS`: run flags were combined in a contradictory or incomplete way, such as `--list dev`, `--list --once`, `--list --env KEY=VALUE`, or `--package` without a value.
- `TSPACK_RUN_INVALID_ENV`: an explicit CLI environment overlay was malformed. `--env` accepts only `KEY=VALUE`; keys must be non-empty and match `[A-Za-z_][A-Za-z0-9_]*`, while values may be empty or contain `=`.
- `TSPACK_RUN_INVALID_READY`: a RunTarget readiness policy has an invalid shape, such as an unknown `kind`, HTTP readiness without an absolute `path`, TCP readiness without a `1..65535` port, or stdout-match readiness without a non-empty literal `pattern` / with an invalid `stream`.
- `TSPACK_RUN_READY_TIMEOUT`: the selected HTTP, TCP, or stdout-match readiness check did not succeed before `--ready-timeout`.
- `TSPACK_RUN_PROCESS_EXITED_EARLY`: the child process exited before the selected readiness check succeeded.

## Pack diagnostics

- `TSPACK_PACK_INVALID_ARGS`: pack flags were combined in a contradictory way; currently `--dry-run --verify` is rejected because dry-run intentionally produces no archive for verification to inspect.
- `TSPACK_PACK_INCLUDE_MATCHED_NOTHING`: an explicit `publish.include` pattern matched no files. This is an error by default because it usually means build outputs such as `dist/**` are missing. Details include the package name, pattern, package root, and a remediation hint to build outputs first or remove/update the include pattern.
- `TSPACK_PACK_CHANGELOG_NOT_INCLUDED`: `CHANGELOG.md` exists at the package root, but the final publish policy omits it. This is a warning only; add `CHANGELOG.md` to `<Publish include={[...]} />` to publish it, remove an excluding pattern if needed, or intentionally ignore the warning. TSPack does not auto-include changelogs.
- `TSPACK_PACK_WRITE_FAILED`: archive output could not be created, written, or moved into place. Pack writes through temporary files and cleans temporary/final paths on a best-effort basis before reporting this error.
- `TSPACK_PACK_UNPUBLISHABLE_PEER_DEPENDENCY`: a publishable package target declares a peer dependency from a non-npm source such as `path`, `git`, or `workspace`. Generated npm `peerDependencies` require package names with version ranges, so pack fails instead of silently omitting the peer.
- `TSPACK_PACK_VERIFY_FAILED`: `--verify` could not read the produced archive or found an artifact-level structural failure such as an empty/stub package payload.
- `TSPACK_PACK_VERIFY_PACKAGE_JSON_MISSING`: `--verify` could not find `package/package.json` in the produced archive.
- `TSPACK_PACK_VERIFY_PACKAGE_JSON_INVALID`: `--verify` found `package/package.json`, but it was not valid JSON.
- `TSPACK_PACK_VERIFY_MISSING_FILE`: `--verify` found a `package.json` path reference such as `main`, `types`, or an `exports` target that does not exist in the archive. Details include package, archive, field, and referenced path.
- `TSPACK_PACK_VERIFY_INVALID_PACKAGE_PATH`: `--verify` found an unsafe archive entry path or package metadata path reference, such as an absolute path, parent traversal, URL-like target, or backslash-containing path.
- `TSPACK_PACK_VERIFY_METADATA_MISMATCH`: `--verify` found package metadata that does not match the manifest-derived pack plan, including name/version/license/main/types/exports or peer dependency metadata.

## TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT

Severity: warning.

The lockfile records that a package declares an npm lifecycle script such as `preinstall`, `install`, `postinstall`, or `prepare`. Details include the lock package ID, script name, raw command string, `execution: blocked by default`, and any available root path that pulls the package. Use `tspack why <package>` or `tspack why --reverse <package>` to investigate reachability.

Human `tspack check` summarizes this warning family by default when multiple packages declare lifecycle scripts. The summary reports the count, deterministic package examples, the blocked-by-policy posture, and `--show-lifecycle` reveal guidance. This does not disable detection, does not suppress serious security diagnostics, and does not remove script or pull-chain details from `tspack check --json`.

## Lifecycle behavior probe violation codes (M37b)

These codes are returned by the explicit lifecycle behavior harness and are not normal package-manager execution diagnostics:

- `TSPACK_LIFECYCLE_UNSUPPORTED_COMMAND`: the probe rejected a lifecycle command outside the MVP `node <script> [args...]` shape.
- `TSPACK_LIFECYCLE_NETWORK_DENIED`: the guarded script attempted a denied network or DNS API.
- `TSPACK_LIFECYCLE_CHILD_PROCESS_DENIED`: the guarded script attempted `child_process` execution.
- `TSPACK_LIFECYCLE_ENV_DENIED`: the guarded script read or checked a denied environment key.
- `TSPACK_LIFECYCLE_FS_READ_DENIED`: the guarded script attempted to read outside allowed roots.
- `TSPACK_LIFECYCLE_FS_WRITE_DENIED`: the guarded script attempted to write, remove, rename, copy, or stream outside allowed roots.
- `TSPACK_LIFECYCLE_GUARD_FAILED`: the Node preload guard reported an internal instrumentation failure.
- `TSPACK_LIFECYCLE_REPORT_MISSING`: the harness could not read the guard report after process exit.

## TSPACK_SECURITY_INVALID_ACKNOWLEDGED_CAPABILITY

Severity: error.

A manifest `Security.acknowledgedCapabilities` row is malformed. Rows must include a non-empty lock package ID, `kind: "lifecycleScript"`, a supported lifecycle script name, a non-empty command, and a non-empty reason.

## TSPACK_SECURITY_INVALID_BEHAVIOR_FIXTURE

Severity: error.

An acknowledgment `behaviorFixture` is not a safe project-relative `.xtest.ts` or `.xtest.tsx` path. Absolute paths, parent traversal, backslash paths, empty paths, and unrelated extensions are rejected.

## TSPACK_SECURITY_BEHAVIOR_FIXTURE_MISSING

Severity: warning.

An acknowledgment links a behavior fixture that does not exist. The fixture is not run automatically; fix or remove the reference so the acknowledgment points at reviewable evidence.

## TSPACK_SECURITY_INVALID_BEHAVIOR_REPORT

Severity: error.

An acknowledgment `behaviorReport` is not a safe project-relative `.json` path.

## TSPACK_SECURITY_BEHAVIOR_REPORT_MISSING

Severity: warning.

An acknowledgment links a behavior report JSON file that does not exist. TSPack does not generate reports during check or doctor.

## TSPACK_SECURITY_BEHAVIOR_REPORT_INVALID

Severity: warning.

An acknowledgment links a behavior report that exists but cannot be parsed as JSON. The report remains evidence metadata and does not grant execution permission.

## TSPACK_SECURITY_DUPLICATE_ACKNOWLEDGED_CAPABILITY

Severity: error.

The manifest declares the same acknowledged capability more than once. Keep exactly one row for each package/kind/script/command tuple.

## TSPACK_SECURITY_ACKNOWLEDGED_CAPABILITY_STALE

Severity: warning.

A manifest acknowledgment matches a package and lifecycle script, but the lockfile records a different command. The actual lifecycle capability remains blocked and is reported as unacknowledged; review the new command before updating policy.

## TSPACK_SECURITY_ACKNOWLEDGED_CAPABILITY_UNUSED

Severity: warning.

A manifest acknowledgment does not match any lifecycle capability in the lockfile. Remove stale policy after dependency removal or update it to the new exact package/script/command tuple.

## Source scan migration diagnostics

- `TSPACK_MIGRATE_SOURCE_SCAN_TRUNCATED`: `tspack migrate` hit conservative source scan limits. Migration continues with partial evidence; review imports manually if classification evidence is important.
- `TSPACK_MIGRATE_SOURCE_PARSE_WARNING`: a source file or source root could not be read or was skipped by a recoverable scan warning. Migration continues and the report lists the affected path.
- `TSPACK_MIGRATE_SOURCE_SCAN_FAILED`: source roots were discovered but no source files could be read. Migration continues so package.json migration can still produce a draft; use `--no-source-scan` to skip source evidence or review file permissions.
