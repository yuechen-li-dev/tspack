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
- `tspack how --list`

## Scope

M31d intentionally ships curated diagnostic help entries, not full diagnostic coverage.

## Non-goals

- No automatic fixes
- No online lookup
- No editor/LSP integration
