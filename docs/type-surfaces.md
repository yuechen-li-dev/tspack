# Type surfaces (M5)

M5 adds a first-pass type surface checker in `internal/typesurface`.

- Verifies declared target `types` outputs exist when declarations are required.
- Scans public `.d.ts`, `.d.mts`, `.d.cts` surfaces for import/export references.
- Traverses relative declaration references recursively with cycle protection.
- Detects obvious scope leakage (`optional peer`, `peer scope`, `undeclared`, `tool`).
- Node builtins are currently classified and ignored.

Out of scope in M5:
- declaration generation
- running `tsc`
- full TypeScript symbol graph analysis

Supported type policy keys and values:
- `declarations`: `required|optional|none`
- `missingTypes`: `error|warn|ignore`
- `publicTypeLeakage`: `error|warn|ignore`
- `typeOnlyRuntimeLeakage`: `error|warn|ignore` (runtime checker code path)

Defaults:
- library packages: declarations required
- otherwise: declarations optional
- missing types: error
- public type leakage: error
