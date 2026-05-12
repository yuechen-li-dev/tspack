# Native TSX Test Harness (M16)

M16 introduces an opt-in, TSPack-native unit test substrate.

## Scope

- Native test file naming: `*.xtest.tsx`
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
