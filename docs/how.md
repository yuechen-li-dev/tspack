# tspack how

`tspack how <diagnostic-code>` explains a diagnostic code with remediation guidance.

## Purpose

`how` is for diagnostic remediation text. It does not run checks, mutate manifests, or apply fixes.

## Syntax

- `tspack how <diagnostic-code>`
- `tspack how --list`
- `tspack how <diagnostic-code> --json`
- `tspack how --list --json`

## Relation to `why` and `doctor`

- `tspack why <query>` explains graph presence (dependency/target/lock edges).
- `tspack how <diagnostic-code>` explains a diagnostic and how to fix it.
- `tspack doctor [scope]` reports environment/toolchain readiness.

## Examples

- `tspack check --json`
- `tspack how TSPACK_IR_INVALID_RELATIVE_PATH`
- `tspack how TSPACK_LOCK_VERSION_CONFLICT`
- `tspack how --list`

## Boundary notes

For boundary diagnostics such as `TSPACK_BOUNDARY_EXPLICIT_DENY`, `from` matches the importing file where the import statement appears. It does not mean every file transitively reachable from an entry file. Use a file-set pattern such as `from: "src/**"` when the restriction should apply to imports written anywhere under `src/`. If violation details show `transitiveFrom`, the deny came from an explicit graph-reachable boundary; inspect the `seed` and `path` detail values to see how the importing file was reached.

## Scope

M31d intentionally ships curated diagnostic help entries, not full diagnostic coverage.

## Non-goals

- No automatic fixes
- No online lookup
- No editor/LSP integration
