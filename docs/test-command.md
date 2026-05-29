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
- `--compact` enables compact native xTest text reporting for run output: passing tests are hidden, failed and skipped tests remain visible, and the summary is always printed. Compact mode does not emit dots, spinners, ANSI control, or other terminal-control output.
- `--watch` enables native xTest watch mode: TSPack runs the selected tests once immediately, watches relevant project source files, debounces changes, and reruns the same selected command until Ctrl+C or SIGTERM.
- `--xtest-bridge <path>` overrides the native xTest JavaScript bridge path for local development, installed layouts, or CI. `--bridge <path>` is accepted as a compatibility alias.

## Native xTest watch mode

- `tspack test --watch` is supported for the native xTest backend only. Use `--xtest` when a project also has Vitest installed and you want native watch mode.
- Watch mode runs the selected native xTest command once immediately, then polls the project root for `.ts`, `.tsx`, `.js`, and `.jsx` file changes. It ignores generated/vendor directories including `node_modules`, `.git`, `.tspack`, `dist`, `build`, `coverage`, `tspack-artifacts`, `tmp`, and `temp`.
- Each rerun uses the same selection and output flags as the initial run. `--filter`, `--compact`, `--root`, and `--xtest-bridge` compose with `--watch`.
- Watch progress messages are written to stderr. Test output stays on the existing backend output path. Watch mode does not clear the screen, use ANSI control, show spinners, or provide interactive keyboard commands.
- Changes are debounced, and TSPack never starts overlapping native xTest runs. If files change while a run is in flight, the next poll observes the dirty files and schedules one follow-up rerun.
- `tspack test --watch --list` and `tspack test --watch --json` are rejected with `TSPACK_TEST_WATCH_INVALID_MODE` because list mode is static discovery and repeated JSON documents are not a stable automation protocol.
- Selecting Vitest with `--watch` is rejected with `TSPACK_TEST_WATCH_UNSUPPORTED_BACKEND`; M34d does not proxy Vitest's own watch mode.
- Watch mode is dirty-key tracking only. It does not implement affected-test graph selection, HMR, rerun-failed-only behavior, snapshots beyond composing with the selected command, affected parallelism beyond `--batch`, or an interactive terminal UI.

## Native xTest compact output

- `tspack test --compact` is output-only and does not change discovery, filtering, or test execution.
- Compact output applies to native xTest run reports. It hides individual `PASS` entries, prints every `FAIL` with the same assertion detail as normal text output, prints every `SKIP` with its reason, prints diagnostics, and always prints summary counts.
- `tspack test --list --compact` leaves list output unchanged because list mode is discovery output, not run reporting.
- The native bridge JSON report ignores compact formatting when `--json --compact` are supplied directly to the bridge, so JSON consumers keep the same structure.
- Vitest output is not reformatted. If the Vitest backend is selected with `--compact`, TSPack emits `TSPACK_TEST_COMPACT_UNSUPPORTED_BACKEND` as a warning and runs Vitest unchanged.

## Native xTest IDs and bridge resolution

- Native xTest public IDs are root-relative, use `/` path separators, and never intentionally include absolute project paths.
- `tspack test --list --root <project>` prints the same IDs that normal runs and reports use. A listed ID can be copied into `tspack test --filter <listed-id>`.
- Theory case suffixes remain part of the ID, so filters such as `--filter "[2]"` still select matching cases.
- The xTest bridge is resolved in this order: `--xtest-bridge`, `TSPACK_XTEST_BRIDGE`, installed/executable-relative candidates, repository development layout, then the existing current-working-directory fallback.
- Missing bridge diagnostics use `TSPACK_TEST_XTEST_BRIDGE_MISSING` and include searched paths, current working directory, and executable path when available.

## Current M18 limitations

- Vitest `--list` is not supported (`TSPACK_TEST_BACKEND_LIST_UNSUPPORTED`).
- No coverage, fixture helpers, command helpers, filesystem helpers, structured Vitest reporting, affected-test watch selection, or interactive watch UI.

## Diagnostics

- `TSPACK_TEST_NO_BACKENDS`
- `TSPACK_TEST_NO_XTESTS`
- `TSPACK_TEST_XTEST_BRIDGE_MISSING`
- `TSPACK_TEST_XTEST_FAILED`
- `TSPACK_TEST_VITEST_NOT_AVAILABLE`
- `TSPACK_TEST_VITEST_FAILED_TO_START`
- `TSPACK_TEST_BACKEND_LIST_UNSUPPORTED`
- `TSPACK_TEST_BACKEND_FILTER_UNSUPPORTED`
- `TSPACK_TEST_COMPACT_UNSUPPORTED_BACKEND`
- `TSPACK_TEST_BATCH_UNSUPPORTED_BACKEND`
- `TSPACK_TEST_WATCH_UNSUPPORTED_BACKEND`
- `TSPACK_TEST_WATCH_INVALID_MODE`
- `TSPACK_TEST_WATCH_FAILED`

- `--list -xtest` includes Fact, Theory case, Valid, and Invalid entries.
- `tspack artifact` continues to use standalone suite-level `<Artifact>` declarations and does not list/run Valid/Invalid entries.

## Snapshot update mode

`tspack test --update-snapshots` enables native xTest snapshot/golden update mode. It applies only to the native xTest backend: missing snapshots are written, mismatched snapshots are overwritten, matching snapshots are left unchanged, and successful writes are reported in text/JSON run reports.

Without `--update-snapshots`, `tspack test` is read-only for snapshots. Missing snapshots fail with `TSPACK_SNAPSHOT_MISSING`, and mismatches fail with `TSPACK_SNAPSHOT_MISMATCH`.

`--filter` limits update scope to selected native xTest cases. `--list` remains static discovery and does not forward snapshot update mode to the native bridge. Compact mode keeps hiding passing tests while still showing snapshot failures and the snapshot update summary. The Vitest backend does not support this flag; selecting Vitest with `--update-snapshots` reports `TSPACK_SNAPSHOT_UNSUPPORTED_BACKEND`.

## Native xTest batch execution

`tspack test --batch` enables native xTest file-level parallelism. The scheduling unit is the test source file: test files may run concurrently, but each file still runs its own Fact, Theory case, Valid, or Invalid entries sequentially in existing declaration order.

Batch worker count is automatic. TSPack chooses the smaller of available host parallelism and the number of scheduled test files, with an internal conservative cap of eight workers. M34f intentionally exposes no public `--jobs`, `--workers`, or `--max-workers` flag.

Reporting remains deterministic. Discovery uses the normal sorted native file order, batch results are stored at the original file index, and text/compact/JSON reports are reconstructed in that order after scheduled files finish. Per-file output is not streamed while tests are running, so concurrently completing files cannot interleave report text.

Supported native compositions:

- `tspack test --batch --filter <text>` applies native filters before scheduling files where possible. Files with no selected tests are not imported or run.
- `tspack test --batch --compact` keeps compact output semantics: passing tests are hidden, failures/skips/diagnostics and the summary remain deterministic.
- `tspack test --batch --update-snapshots` may update snapshots from multiple files concurrently. Snapshot paths remain anchored by the owning source file, and tests within one file remain sequential.
- `tspack test --batch --root <project>` and `tspack test --batch --xtest-bridge <path>` compose normally.
- `tspack test --watch --batch` is allowed for native xTest and each watch rerun invokes the same batch-capable native command. Existing watch no-overlap behavior still prevents overlapping reruns.
- `tspack test --list --batch` treats batch as an execution-only option and does not forward it to the native bridge for list mode.
- `tspack test --json --batch` emits one parseable deterministic JSON report with no progress text on stdout.

Vitest does not support M34f batch proxying. Selecting Vitest with `--batch` is rejected with `TSPACK_TEST_BATCH_UNSUPPORTED_BACKEND`.
