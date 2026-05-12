# Manifest (M1 subset)

`manifest.tsx` is a **typed document**, not an executable TypeScript program.
TSPack parses/analyzes AST and never executes user manifest code.

## M1 constraints

- File must be root `manifest.tsx`.
- Imports allowed only from `tspack/manifest` (including `import type`).
- Default export must be `define(<Workspace>...</Workspace>)`.
- Approved helpers: `define`, `defineDeps`, `npm`, `git`, `path`, `workspace`, `dep`, `peer`, `tool`.
- Approved JSX elements: `Workspace`, `Package`, `Policies`, `Targets`, `Tools`, `Boundaries`, `Publish`.
- `rows={[...]}` and `values={[...]}` must remain literal/restricted.

## Forbidden constructs

- Any non-`tspack/manifest` import.
- `process.env`, filesystem/network/time/random APIs.
- Functions, classes, async/await.
- Loops or conditionals.
- Dynamic generation (`map`/`filter`/`reduce`, unknown helper calls, arbitrary calls).

## Example

```tsx
import { define, defineDeps, npm, tool } from "tspack/manifest";

const deps = defineDeps({
  typescript: tool(npm("typescript", "^5.9")),
});

export default define(
  <Workspace name="mono">
    <Package name="pkg" version="0.1.0" license="MIT" kind="library" dependencies={{ values: [deps.typescript] }}>
      <Targets rows={[{ name: "core", export: ".", entry: "src/index.ts", runtime: "dist/index.js", types: "dist/index.d.ts", peers: [], deps: [], optional: false }]} />
      <Publish include={["dist/**"]} exclude={[]} />
    </Package>
  </Workspace>,
);
```
