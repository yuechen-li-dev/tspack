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
- Human CLI output prints diagnostic detail lines under `CODE: message` with indentation for easier resolver/store debugging.
- Severity is preserved (`error`, `warning`, `info`), and warning diagnostics are included in the report summary.
- `ok` is `false` when one or more `error` diagnostics exist, otherwise `true`.
- `TSPACK_WHY_NOT_FOUND` may include detail lines with matching lock package IDs and a suggested `tspack why npm:<name>@<version>` query when a bare package name only matches transitive lock entries.

## Harness diagnostics

- `TSPACK_TEST_*`
- `TSPACK_ARTIFACT_*`
- `TSPACK_BENCH_*`
- `TSPACK_DOOM_*`

## Run diagnostics

- `TSPACK_RUN_*` (target resolution, launch, readiness, timeout, process lifecycle)

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
