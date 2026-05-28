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

Import-chain traces:
- Boundary diagnostics may include an import chain from the target entry to the external dependency.
- These traces explain source reachability, but boundary `from` rows still match the physical importing file where the import statement appears.
- Boundary `transitiveFrom` rows intentionally use scanner reachability: TSPack finds seed files, walks local relative runtime imports from those seeds, and applies the rule to every reachable file.
- `transitiveFrom` traversal uses the same local relative resolver as normal import scanning, including TypeScript ESM `.js`/`.jsx` source aliasing. It does not resolve through `node_modules`.

Relative resolution:
- Exact existing files win before aliases.
- Extensionless relative imports use the supported extension order and directory `index` fallback.
- TypeScript ESM `.js` specifiers are also resolved against same-stem `.ts`, `.tsx`, and `.jsx` source files when the exact `.js` file is absent.
- TypeScript ESM `.jsx` specifiers are also resolved against same-stem `.tsx`, `.ts`, and `.js` source files when the exact `.jsx` file is absent.

Non-goals in M4: no package resolution, no type-checking, no full type leakage analysis.

## Type-only import and re-export detection

The scanner classifies obvious type-level edges as `type-only`:

- `import type { Foo } from "pkg"`
- `export type { Foo } from "pkg"`
- `export type * from "pkg"`
- mixed named clauses such as `import { type Foo, value } from "pkg"`, which produce both a runtime edge and a type-only edge for the same specifier
- named clauses containing only `type` specifiers, such as `export { type Foo } from "pkg"`, which produce only a type-only edge

For `.d.ts`, `.d.mts`, and `.d.cts` files, scanner-visible imports and re-exports are treated as type-level edges by default, including `import("pkg")` type references. The scanner does not trace symbols through TypeScript's type checker; it only extracts module specifiers.
