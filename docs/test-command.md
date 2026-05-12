# `tspack test` (M18)

`tspack test` is a backend orchestrator.

## Backends

- xTest backend discovers `**/*.xtest.tsx`, `**/*.valid.tsx`, and `**/*.invalid.tsx` files.
- Vitest backend runs local `node_modules/.bin/vitest`.

## Selection

- `tspack test` auto-detects available backends.
- `tspack test -xtest` / `--xtest` selects only xTest.
- `tspack test -vitest` / `--vitest` selects only Vitest.
- Flags can be combined (xTest runs first, then Vitest).

## Flags

- `--root <path>` defaults to `.`
- `--list` lists xTest cases without executing callbacks.
- `--filter <text>` applies backend filter where supported.

## Current M18 limitations

- Vitest `--list` is not supported (`TSPACK_TEST_BACKEND_LIST_UNSUPPORTED`).
- No coverage, watch mode, fixture helpers, command helpers, filesystem helpers, or structured Vitest reporting.

## Diagnostics

- `TSPACK_TEST_NO_BACKENDS`
- `TSPACK_TEST_NO_XTESTS`
- `TSPACK_TEST_XTEST_BRIDGE_MISSING`
- `TSPACK_TEST_XTEST_FAILED`
- `TSPACK_TEST_VITEST_NOT_AVAILABLE`
- `TSPACK_TEST_VITEST_FAILED_TO_START`
- `TSPACK_TEST_BACKEND_LIST_UNSUPPORTED`
- `TSPACK_TEST_BACKEND_FILTER_UNSUPPORTED`

- `--list -xtest` includes Fact, Theory case, Valid, and Invalid entries.
- `tspack artifact` continues to use standalone suite-level `<Artifact>` declarations and does not list/run Valid/Invalid entries.
