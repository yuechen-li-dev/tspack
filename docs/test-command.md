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
- `--filter <text>` applies backend filter where supported. Native xTest filters continue to match ID substrings and test-name substrings.
- `--xtest-bridge <path>` overrides the native xTest JavaScript bridge path for local development, installed layouts, or CI. `--bridge <path>` is accepted as a compatibility alias.

## Native xTest IDs and bridge resolution

- Native xTest public IDs are root-relative, use `/` path separators, and never intentionally include absolute project paths.
- `tspack test --list --root <project>` prints the same IDs that normal runs and reports use. A listed ID can be copied into `tspack test --filter <listed-id>`.
- Theory case suffixes remain part of the ID, so filters such as `--filter "[2]"` still select matching cases.
- The xTest bridge is resolved in this order: `--xtest-bridge`, `TSPACK_XTEST_BRIDGE`, installed/executable-relative candidates, repository development layout, then the existing current-working-directory fallback.
- Missing bridge diagnostics use `TSPACK_TEST_XTEST_BRIDGE_MISSING` and include searched paths, current working directory, and executable path when available.

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
