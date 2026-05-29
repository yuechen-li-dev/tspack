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

## Package/core diagnostics

- `TSPACK_FRONTEND_*`
- `TSPACK_IR_*`
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
- Human CLI output prints diagnostic detail lines under `CODE: message` with indentation for easier resolver/store debugging.
- Severity is preserved (`error`, `warning`, `info`), and warning diagnostics are included in the report summary.
- `ok` is `false` when one or more `error` diagnostics exist, otherwise `true`.
- `TSPACK_LOCK_VERSION_CONFLICT` warns when one source ecosystem/package name appears at multiple locked versions (for example two npm react versions).
- `TSPACK_WHY_NOT_FOUND` may include detail lines with matching lock package IDs and a suggested `tspack why npm:<name>@<version>` query when a bare package name only matches transitive lock entries.

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


- `tspack format` and `tspack lint` are Biome-backed lifecycle UX commands. See `docs/format-lint.md`.

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
