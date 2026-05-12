# Import scanning (M4)

M4 adds source import scanning for `.ts`, `.tsx`, `.js`, `.jsx`, `.mts`, `.cts`.

Supported forms:
- `import ... from "pkg"`
- `import type ... from "pkg"`
- `export ... from "pkg"`
- `export type ... from "pkg"`
- `import "pkg"`
- `require("pkg")`
- `import("pkg")`

Classifications:
- Runtime
- TypeOnly
- UnknownDynamic (non-literal `require(...)` or `import(...)`)

Specifier classifications:
- RelativeInternal (`./`, `../`)
- ExternalPackage (including scoped package root extraction)
- NodeBuiltin (`node:fs` and selected builtin names)
- Unknown

Non-goals in M4: no package resolution, no type-checking, no full type leakage analysis.
