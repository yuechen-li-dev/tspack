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

For boundary diagnostics such as `TSPACK_BOUNDARY_EXPLICIT_DENY` and `TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION`, `from` matches the importing file where the import statement appears. It does not mean every file transitively reachable from an entry file. Use a file-set pattern such as `from: "src/**"` when the restriction should apply to imports written anywhere under `src/`. If violation details show `transitiveFrom`, the deny came from an explicit graph-reachable boundary; inspect the `seed` and `path` detail values to see how the importing file was reached.

## Scope

M31d intentionally ships curated diagnostic help entries, not full diagnostic coverage.

## Non-goals

- No automatic fixes
- No online lookup
- No editor/LSP integration


## allowOnly boundary diagnostics

`TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION` means a matching boundary row restricted external runtime imports to `allowOnly`, and the imported package was not in that row. Relative/internal imports are still allowed. `allowOnly` is not a dependency declaration, so listed packages must also be declared/allowed by the target dependency model. Tool-runtime and `denyDeps` diagnostics take precedence.

## Type boundary diagnostics

`TSPACK_BOUNDARY_TYPE_EXPLICIT_DENY` means a type-only external import or re-export matched `denyTypeDeps`. Move the type behind the correct public target, remove the public type leakage, split internal and public type definitions, or adjust the boundary row if the type exposure is intentional. Runtime-only remediation for `denyDeps` is not enough: `denyTypeDeps` is about the public/type surface and scanner-visible type edges.
