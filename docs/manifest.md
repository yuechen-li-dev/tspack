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
- Spread syntax is forbidden (`...` in object literals, arrays/call arguments, and JSX props) to keep manifests fully inspectable and non-opaque.

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


## M6b workspace split mode

TSPack now supports split workspaces:
- Root `manifest.tsx` defines workspace topology via `<Packages rows={...} />`.
- Each package contract lives in `package.manifest.tsx`.
- Use `defineWorkspace(...)` for root split workspaces and `definePackage(...)` for package manifests.

Path rules:
- `row.root` and `row.manifest` must be safe relative paths.
- `row.manifest` must be located under `row.root`.
- Paths inside `package.manifest.tsx` (for example target `entry` and `types`) are relative to the package root.


## RunTargets

Use `<RunTargets rows={[{ name, runtime, command, url, ready }]} />` inside a `Package`.
- `runtime`: `system` or `node` in M22.
- `command`: argv array; no shell string execution.
- `url`: required HTTP/HTTPS URL.
- `ready`: optional `{ kind: "http", path: "/" }`.
