# Manifest (M1 subset)

`manifest.tsx` is a **typed document**, not an executable TypeScript program.
TSPack parses/analyzes AST and never executes user manifest code.

## Authoring type surface

TSPack provides a local TypeScript authoring surface at `tspack/manifest`.
- Import helpers, policy types, and JSX manifest elements directly from `tspack/manifest`.
- `tspack init` writes project-local declaration support (`.tspack/types/tspack-manifest.d.ts` and `tspack-env.d.ts`) so standard TypeScript tooling resolves the module without an npm package or editor extension.
- This surface is for editor autocomplete/typechecking only.
- Parser, normalized IR, and Go validation remain authoritative.
- Manifests are still statically parsed; helper functions are not runtime-executed.

## M1 constraints

- File must be root `manifest.tsx`.
- Imports allowed only from `tspack/manifest` (including `import type`).
- Default export must be `define(<Workspace>...</Workspace>)`.
- Approved helpers: `define`, `defineDeps`, `npm`, `git`, `path`, `workspace`, `dep`, `peer`, `tool`.
- Approved JSX elements: `Workspace`, `Packages`, `Package`, `Policies`, `Targets`, `RunTargets`, `Tools`, `Boundaries`, `Publish`.
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



## Publish include conventions

`<Publish include={...} />` is the complete explicit source of package contents before `exclude` filters apply. TSPack does not silently add conventional files. Include optional package documents, such as `README.md`, `LICENSE`, and `CHANGELOG.md`, only when those files exist and should be published:

```tsx
<Publish include={["dist/**", "README.md", "LICENSE", "CHANGELOG.md"]} exclude={[]} />
```

If `CHANGELOG.md` exists but the final publish policy omits it, `tspack pack` warns with `TSPACK_PACK_CHANGELOG_NOT_INCLUDED`; add the file to `include` or intentionally ignore the warning.

## Package publish metadata

A package `license` is carried into generated package artifacts. When `tspack pack` generates `package/package.json`, a manifest package with `license="MIT"` emits `"license": "MIT"`; missing or empty licenses are not invented.

Manifest peer dependencies affect generated npm `peerDependencies` for packed library artifacts when the peer source is `npm(name, range)`. Optional peer dependencies are represented in generated `peerDependenciesMeta` when the IR marks the peer dependency optional. Non-npm peer sources cannot be represented as npm `peerDependencies` and fail packing with `TSPACK_PACK_UNPUBLISHABLE_PEER_DEPENDENCY`. Tool dependencies remain build/tooling metadata and are not emitted as runtime package dependencies by pack.

For npm interoperability, the root target (`export: "."`) provides generated package entry metadata: its `runtime` becomes `main` and its `types` output becomes `types`, both normalized to package-relative paths such as `./dist/index.js`. Non-root targets continue to be represented through `exports`; pack does not guess `main` from a non-root target.

## M6b workspace split mode

TSPack now supports split workspaces:
- Root `manifest.tsx` defines workspace topology via `<Packages rows={...} />`.
- Each package contract lives in `package.manifest.tsx`.
- Use `defineWorkspace(...)` for root split workspaces and `definePackage(...)` for package manifests.

Path rules:
- `row.root` and `row.manifest` must be safe relative paths.
- `row.manifest` must be located under `row.root`.
- Paths inside `package.manifest.tsx` (for example target `entry` and `types`) are relative to the package root.

Type output rules:
- `library` targets should declare a non-empty safe `types` output path (for example `dist/index.d.ts`).
- `app` targets may set `types: ""` to explicitly indicate there is no public type output for that target.


## Boundary rows

Use `<Boundaries rows={[...]}/>` inside a `Package` to declare runtime boundary rows. A row can scope by physical importing file with `from`, or by local import-graph reachability with `transitiveFrom`.

Supported boundary scope fields:
- `from?: string` — exact file or `/**` pattern matched against the file where the import statement is physically written.
- `transitiveFrom?: string` — exact file or `/**` pattern used to find seed files; the row applies to the seed and every file reachable through local relative runtime imports.

A boundary row cannot specify both `from` and `transitiveFrom`. Scope paths must be safe relative package paths or safe relative glob-like patterns such as `src/**`.


## RunTargets

Use `<RunTargets rows={[{ name, runtime, command, url, cwd, ready }]} />` inside a `Package`.
- `runtime`: `system` or `node` in M22.
- `command`: argv array; no shell string execution.
- `url`: HTTP/HTTPS URL for status and tools. It is required for HTTP readiness and URL-only readiness; TCP and stdout-match readiness can omit it.
- `cwd`: optional `"workspace"` or `"package"`; omitted means `"workspace"`.
- `ready`: optional readiness policy:
  - `{ kind: "http", path: "/" }` polls `url + path` for a `200-399` response.
  - `{ kind: "tcp", host?: "127.0.0.1", port: 5432 }` connects to TCP until successful; `host` defaults to `127.0.0.1`.
  - `{ kind: "stdout-match", pattern: "Local:", stream?: "stdout" | "stderr" | "both" }` watches child output for a literal substring; `stream` defaults to `both`.


## Boundary row `allowOnly`

Boundary rows may include `allowOnly?: string[]` with either `from` or `transitiveFrom`. The list contains exact external package identifiers permitted for matching runtime imports. An empty list is valid and means no external runtime packages are permitted in that scope. `allowOnly` does not declare dependencies; targets still need the normal `deps` or `peers` allowance for any listed package.

## Boundary row `denyTypeDeps`

Boundary rows may include `denyTypeDeps?: string[]` with either `from` or `transitiveFrom`. The list contains exact external package identifiers denied for scanner-visible type-only imports and re-exports. Empty arrays are valid and have no effect. Entries must be non-empty strings.

`denyDeps` and `allowOnly` apply to runtime external imports. `denyTypeDeps` applies only to type-level external imports/re-exports and does not deny runtime imports by itself.
