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

## Pack changelog warning

`TSPACK_PACK_CHANGELOG_NOT_INCLUDED` means a package root contains `CHANGELOG.md`, but the final publish policy does not place it in the archive. TSPack warns instead of adding the file automatically because package contents must remain explicit and auditable. Add `CHANGELOG.md` to `<Publish include={[...]} />` if consumers should receive it, remove an exclude pattern if one filters it out, or intentionally ignore the warning when the omission is deliberate.

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


## Native xTest type assertion diagnostics

`TSPACK_TYPE_ASSERTION_FAILED` means an `assert.type<TExpected>(value, reason)` call did not satisfy TypeScript assignability. Read the attached TypeScript diagnostic, compare the actual expression type with the expected type argument, and either fix the implementation return type or change the expected type if the assertion was too narrow. The assertion is static: runtime execution is not evidence that the type proposition is true.

`TSPACK_TYPE_ASSERTION_REASON_REQUIRED` means an `assert.type` call omitted its reason or used an empty literal. Add a concise non-empty reason that explains why the type proposition matters.

`TSPACK_TEST_TYPECHECK_FAILED` means the native xTest typecheck lane could not construct the source program for static assertions. Check that the native test file and local relative imports exist and can be parsed by TypeScript without relying on unsupported M34g features such as tsconfig path aliases.
