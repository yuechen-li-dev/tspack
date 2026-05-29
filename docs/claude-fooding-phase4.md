# Claude-Fooding Phase 4 Remediation

Claude-fooding Phase 4 focused on TSPack's native xTest harness. The closeout state is **Success**: the original motivating test-authoring flow now has documented remediation milestones, release-gate smoke coverage expectations, and a clear native xTest development/debugging path without adding new behavior in this closeout milestone.

## Original findings

Phase 4 found that the native xTest shape was promising, especially:

- Native TSX `Suite` / `Fact` / `Theory` / `Case` syntax was comfortable to author and review.
- Mandatory assertion reasons and the failure format were strong.
- Theory case suffix filtering was useful for narrowing failures.
- Static discovery before execution was strong.

It also identified gaps and follow-up requests:

- Source TypeScript imports were missing from native xTest execution.
- List/filter IDs were inconsistent enough that copied IDs did not always work.
- xTest bridge resolution was sensitive to the current working directory.
- Report output was too noisy for normal development loops.
- Theory callback / `Case` structure could silently pass zero-case theories.
- Watch mode was requested for local development.
- Snapshot/golden assertions were requested for deterministic expected output.
- Parallel execution was requested for larger suites.
- Static TypeScript assertions were requested for type-surface checks inside native xTest.

## Remediation summary

| Finding | Milestone | Fix | Status |
|---|---|---|---|
| Native xTest could not import nearby TypeScript source modules. | M29 | Native runtime execution materializes a local relative TypeScript/TSX import closure before importing test entries. | Done |
| Listed IDs and executed IDs were not consistently copy/paste friendly. | M34a | Native xTest IDs are root-relative, list/run/filter-consistent, and keep Theory case suffixes. | Done |
| Bridge lookup depended too much on process CWD. | M34a | Added explicit `--xtest-bridge` / `--bridge` override paths and more robust bridge resolution diagnostics. | Done |
| Theory callback placement was too rigid and bad Theory shapes could pass vacuously. | M34b | Theory callbacks may appear before, between, or after cases; zero-case, missing-body, and duplicate-body theories fail deterministically. | Done |
| Normal reports were too noisy. | M34c | `--compact` hides passing tests while preserving failures, skips, diagnostics, and summary counts. | Done |
| Local development needed reruns after edits. | M34d | Native `tspack test --watch` provides a boring dirty-key rerun loop. | Done |
| Golden output comparisons needed deterministic first-class assertions. | M34e | Added `expect.snapshotText(...)`, `expect.snapshotJson(...)`, and explicit `--update-snapshots`. | Done |
| Larger native suites needed file-level concurrency. | M34f | Added `--batch` file-level parallelism with automatic worker selection and deterministic report ordering. | Done |
| Type-surface expectations needed static checks. | M34g | Added `assert.type<TExpected>(value, reason)` with semantic TypeScript assignability diagnostics, compact/JSON reporting, and meaningful-action integration. | Done |

## Current native xTest model

Native xTest now supports:

- TSX `Suite` / `Fact` / `Theory` / `Case` structure for readable test declarations.
- Static discovery and `--list` before executing callback bodies.
- Root-relative stable test IDs that can be copied from list output into `--filter`.
- Source import closure support for local relative TypeScript/TSX/JavaScript/JSX imports.
- Mandatory human-readable reasons for assertions and expectation chains.
- No-assertion enforcement through `TSPACK_TEST_NO_ASSERTION` when a test takes no meaningful action.
- Runtime conditional `skip(reason)` with required skip reasons.
- Compact output that hides passing tests but keeps failures, skips, diagnostics, and summaries visible.
- Native watch mode through `tspack test --watch` for dirty-key reruns.
- Snapshot/golden assertions through `expect.snapshotText(...)` and `expect.snapshotJson(...)`.
- Explicit snapshot update mode through `tspack test --update-snapshots`.
- File-level parallelism through `tspack test --batch` with deterministic report ordering.
- Static TypeScript assignability assertions through `assert.type<TExpected>(value, reason)`.

## Current xTest golden debugging/development flow

Use the native harness in small, explicit steps:

```sh
tspack test --list
tspack test --filter <id>
tspack test --compact
tspack test --watch
tspack test --batch
tspack test --update-snapshots
tspack test --json
tspack how TSPACK_TEST_THEORY_NO_CASES
tspack how TSPACK_SNAPSHOT_MISMATCH
tspack how TSPACK_TYPE_ASSERTION_FAILED
```

Suggested loop:

1. Start with `tspack test --list` to confirm static discovery and copy a root-relative ID.
2. Use `tspack test --filter <id>` to reproduce the narrow motivating failure.
3. Use `tspack test --compact` for routine runs where passing tests are noise.
4. Use `tspack test --watch` for local reruns while editing.
5. Use `tspack test --batch` before committing larger native-suite changes.
6. Use `tspack test --update-snapshots` only when intentionally accepting golden output changes.
7. Use `tspack test --json` for automation and report consumers.
8. Use `tspack how <diagnostic-code>` for focused remediation guidance.

## Explicit non-goals / deferred work

- No Vitest watch proxy.
- No affected-test graph watch mode yet.
- No public worker-count knob for `--batch`.
- No intra-file / Theory-case parallelism.
- No inline snapshots.
- No custom snapshot serializers.
- No exact type equality assertion yet.
- No negative type assertion yet.
- No tsconfig/path-alias integration for type assertions yet.
- No full TypeScript symbol graph analyzer.
- No editor/LSP integration.
