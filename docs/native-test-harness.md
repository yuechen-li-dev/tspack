# Native TSX Test Harness (M16)

M16 introduces an opt-in, TSPack-native unit test substrate.

## Scope

- Native test file naming: `*.xtest.tsx`, `*.valid.tsx`, `*.invalid.tsx`
- TSX tags are used for static discovery and metadata
- Test callback bodies are ordinary TypeScript code
- Assertions require explicit human-readable reasons
- `assert.*` is the primary assertion API
- `expect(...).matcher(...).because(reason)` is available
- Fixture, command, filesystem, golden, archive, and diagnostics helpers are intentionally out of scope in M16
- Vitest remains separate and unchanged for existing test suites

## TSX discovery model

Supported tags:

- `<Suite name="...">`
- `<Fact name="...">{() => {}}</Fact>`
- `<Theory name="..."> ... </Theory>`
- `<Case ... />`

Discovery is static and deterministic:

- names must be string literals
- case values must be literal string/number/boolean/null
- spread syntax is rejected
- dynamic test generation is not supported

## Result IDs

Example IDs:

- `math/addition works`
- `math/string lengths[0]`
- `math/string lengths[1]`


## M17 file discovery

Native test files use the `*.xtest.tsx` suffix. Here, `x` means extended/xUnit style, not skipped.
Native tests can live next to production source files, but native test tags are only valid in native test files.
Conventional `*.test.ts`, `*.test.tsx`, `*.spec.ts`, and `*.spec.tsx` are not claimed by the native harness.
Discovery/listing is static and does not execute callback bodies.

Non-goals remain unchanged for M17: no fixture runner, no command helpers, no filesystem assertions, no Vitest orchestration, and no `tspack test` CLI command.

## M17b assertion and control-flow additions

### `assert.near(actual, expected, tolerance, reason)`

- Explicit tolerance is required; there is no implicit default.
- Reason is mandatory and must be non-empty.
- Assertion passes when `Math.abs(actual - expected) <= tolerance`.
- Assertion fails when:
  - `actual`, `expected`, or `tolerance` is non-finite.
  - `tolerance` is negative.
  - absolute difference is greater than tolerance.
- Failure uses assertion kind `near` and includes actual, expected, tolerance, absolute difference, and reason.

### `skip(reason)`

- `skip(reason)` is exported by the native test surface for runtime conditional skipping.
- Reason is mandatory and must be non-empty.
- Intended usage is guard-clause style near the top of a Fact or Theory case body.
- Calling `skip(reason)` immediately exits the current test body:
  - In `Fact`, that Fact result is `skipped`.
  - In `Theory`, only the current Case execution is `skipped`; other cases still run.
- Skips are not failures and are reported with `status: "skipped"` and a recorded skip reason.

## M17c native artifacts

`<Artifact />` is supported under `<Fact>` and `<Theory>` declarations.

```tsx
<Artifact name="report" path="report.json" format="json" />
```

Rules:
- `name` and `path` are required string literals.
- `format` is optional string literal.
- `optional={true}` marks non-required artifacts (default is required).
- Artifact paths must be relative and safe (no absolute paths, `..`, or backslashes).

Runner options now include `artifactRoot`:
- `runSuite(..., { artifactRoot })`
- `runNativeTestFiles(..., { artifactRoot })`
- Per-test output directory is `artifactRoot/<sanitized-test-id>/...`.

Test callbacks receive `ctx.artifact` writer:
- `writeText(name, text, reason)`
- `writeJson(name, value, reason)`
- `writeBytes(name, bytes, reason)`

Artifact reasons are mandatory. Required artifacts must be written unless the test is skipped.

Non-goals in M17c remain unchanged: no fixtures, no snapshots/golden assertions, no command helpers, and no filesystem assertion API beyond artifact writing.

## M17d list/filter/report

### Listing APIs

- `listDiscoveredTests(discovery)` expands static discovery into deterministic `ListedTest[]` entries.
- `listNativeTests(options)` performs file discovery and returns `{ tests, diagnostics }`.
- Listing is static-only and does not import test modules or execute callbacks.
- Listed IDs keep the `file.xtest.tsx::suite/test` prefix shape.
- Listed metadata includes Fact and Theory cases plus declared artifacts.

### Filter semantics

- `runNativeTestFiles({ filter })` uses plain substring matching against full test IDs.
- Theory-name matches include all theory cases because case IDs include the theory name.
- Case suffix filtering (for example `[1]`) works because suffix appears in final IDs.
- Filtering is applied before module import; files with no matching discovered IDs are not imported.
- No-match filter emits `TSPACK_TEST_FILTER_NO_MATCH` and returns no test results.

### Report model

- `createNativeTestReport(result)` returns `{ summary, tests, diagnostics }`.
- Summary includes total/passed/failed/skipped/diagnostic counts.
- Test entries include status, optional skip reason, failure normalization, and artifacts.
- Failure normalization surfaces code/message/reason/assertion/actual/expected plus near details when present.

### Text and JSON reports

- `formatNativeTestTextReport(report)` emits deterministic PASS/FAIL/SKIP lines, failure details, artifact lines, diagnostics, and summary counts.
- `formatNativeTestJsonReport(report)` emits two-space indented JSON with trailing newline.

### Exit code semantics

- `nativeTestExitCode(report)` returns `1` when:
  - any test failed, or
  - any diagnostic is severity `error` (default severity if missing).
- Returns `0` when all tests pass/skip and diagnostics are non-error.
- `TSPACK_TEST_FILTER_NO_MATCH` is treated as error and therefore exits `1`.

M17d non-goals remain unchanged: no fixture runner, no command helpers, no filesystem assertion suite, no Vitest orchestration, and no package-manager `tspack test` CLI.


## Standalone artifacts

- `tspack artifact` runs standalone native xTest `<Artifact>` units declared directly under `<Suite>`.
- `tspack pack` creates package `.tgz` archives; it is unrelated to native test artifacts.


See also `docs/artifacts.md` for standalone artifact mode details.


## Valid/Invalid fixture invariants

- `*.valid.tsx` files allow only `<Valid>` children under `<Suite>`.
- `*.invalid.tsx` files allow only `<Invalid>` children under `<Suite>`.
- `<Valid>` and `<Invalid>` execute like facts with sync/async support and `skip(reason)`.
- Invalid invariants pass when expected diagnostics are observed (for example with `expect.error(...)`).
- Use `expect.error(subject, code).because(reason)` to assert an error code exists.
- Use `expect.noErrors(subject).because(reason)` to assert no error diagnostics exist.
- These fixture invariants run via the native xTest backend (`tspack test -xtest`) and are not included in `tspack artifact`.

## Project fixtures (`<Project />`)

Executable native units (`Fact`, `Theory`, `Valid`, `Invalid`, and suite `Artifact`) can include one `<Project />` declaration.

- `from?: string` fixture directory copied into per-execution sandbox.
- `name?: string` label metadata.
- `keepOnFailure?: boolean` preserves sandbox on failure.

The callback context includes `project` with `path`, `readText`, `readJson`, `writeText`, `writeJson`, and `writeBytes`. Paths are sandboxed and unsafe paths are rejected. Project write helpers require a non-empty reason.

Fixture copy skips `node_modules`, `.git`, `.tspack`, `tspack-artifacts`, and `dist-packages`. Symlinks are rejected (`TSPACK_PROJECT_SYMLINK_UNSUPPORTED`).

### Non-goals

- No command execution helpers.
- No filesystem assertion helpers.
- No snapshot/golden tooling.
- No automatic valid/invalid parser runners.

## M19e execution contracts

### `CycleTime`

Use `<CycleTime seconds={numberLiteral} />` to set execution timeout metadata.

Allowed locations:
- Under `<Theory>` (applies per theory case).
- Under suite-level standalone `<Artifact>` (applies to artifact generation unit).

Not allowed:
- Under `<Fact>`, `<Valid>`, `<Invalid>`, `<Case>`, or nested test artifact declarations.

Rules:
- `seconds` is required.
- Must be a positive finite number literal.
- No dynamic expressions.
- No JSX spread attributes.
- Only one `CycleTime` per parent.

Diagnostics:
- `TSPACK_TEST_INVALID_CYCLETIME`
- `TSPACK_TEST_DUPLICATE_CYCLETIME`
- `TSPACK_TEST_CYCLETIME_NOT_ALLOWED`

### Timeout behavior

- Default timeout is **30 seconds** for executable native units.
- Theory `CycleTime` overrides timeout per case.
- Standalone Artifact `CycleTime` overrides timeout for artifact execution.
- Custom Fact/Valid/Invalid timeout controls are intentionally out of scope for M19e.

Limitation:
- Current timeout guards async/awaited operations in the current runner.
- It does not interrupt a hard synchronous infinite loop without future worker/process isolation.

### Meaningful-action requirement

Executable units must perform meaningful action.

For Fact / Theory case / Valid / Invalid, meaningful action is at least one of:
- `assert.*`
- `expect(...).because(...)`
- `skip(reason)`

For standalone Artifact, meaningful action is at least one of:
- `assert.*`
- `expect(...).because(...)`
- `skip(reason)`
- `artifact.writeText`, `artifact.writeJson`, `artifact.writeBytes`

If a unit otherwise completes successfully without meaningful action, it fails with `TSPACK_TEST_NO_ASSERTION`.

### M19e non-goals

- benchmarks
- death tests
- worker/process isolation
- custom Fact timeout controls
