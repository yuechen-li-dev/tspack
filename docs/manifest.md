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




## Dependency aliases and explicit keys

`defineDeps({ ... })` attaches each object property name as a dependency-reference alias. When no explicit dependency `key` is present, references such as `<Tools values={[deps.typescript]} />` use that alias, so this common unscoped dependency shape keeps emitting `"typescript"`:

```tsx
const deps = defineDeps({
  typescript: tool(npm("typescript", "^5.0.0")),
});
```

Use an explicit `key` when the dependency identity differs from the local property alias. The explicit key overrides the `defineDeps` alias for target dependency refs, peer refs, and tool refs. This is especially important for scoped npm packages or dashed package names when the local alias must be a TypeScript-safe identifier:

```tsx
const deps = defineDeps({
  biomejsBiome: tool(npm("@biomejs/biome", "^1.9.4"), {
    key: "@biomejs/biome",
  }),
  reactDom: peer(npm("react-dom", "^18.3.1"), {
    key: "react-dom",
  }),
});

<Tools values={[deps.biomejsBiome]} />
```

Without the explicit key in this example, the manifest frontend preserves the property alias fallback. Because the dependency identity derived from the npm package is `@biomejs/biome` or `react-dom`, Go IR validation reports `TSPACK_IR_UNKNOWN_DEPENDENCY_REF` if the alias and dependency identity do not match. `tspack migrate` follows this rule automatically: generated declarations get `key` whenever the TypeScript identifier differs from the npm package name, while packages such as `typescript` do not get noisy key options.

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

## Security lifecycle capability acknowledgments

Workspace manifests may include a top-level `<Security />` section under `<Workspace />`:

```tsx
<Workspace name="ws">
  <Security
    acknowledgedCapabilities={[
      {
        package: "npm:@biomejs/biome@1.9.4",
        kind: "lifecycleScript",
        script: "postinstall",
        command: "node scripts/postinstall.js",
        reason: "Known lifecycle capability; execution remains blocked by TSPack.",
      },
    ]}
  />
  <Package name="app" version="1.0.0" kind="library">...</Package>
</Workspace>
```

Each acknowledged capability requires `package`, `kind: "lifecycleScript"`, one supported lifecycle `script`, the exact lockfile `command`, and a non-empty `reason`. Acknowledgments suppress the default lifecycle warning only when all fields match. They do not run scripts and are not written to the lockfile.

Optional evidence fields may be added to a row:

- `behaviorFixture?: string` — safe project-relative path to a source-controlled `.xtest.ts` or `.xtest.tsx` lifecycle behavior fixture.
- `behaviorReport?: string` — safe project-relative path to a `.json` behavior report from a previous explicit probe.

These fields are evidence metadata, not execution permission. `tspack check`, `tspack doctor security`, and `tspack why` validate and surface the references without running fixtures or lifecycle scripts. Missing references warn; omitted evidence is allowed.

## Workspace runtime profile

`<Workspace>` accepts an optional `runtime` prop:

```tsx
<Workspace name="demo" runtime="nodejs">
  ...
</Workspace>
```

Allowed values are `nodejs`, `bun`, and `deno`. Omitted runtime defaults to `nodejs` in the normalized IR and Go model.

The runtime profile selects the JavaScript runtime profile for future runtime-portability seams. It is not package-manager switching: `npm`, `pnpm`, `yarn`, `bun-npm`, and `deno-npm` are invalid runtime profiles. TSPack continues to own dependency resolution, `ts-lock.toml`, materialization, checks, packing, and lifecycle security policy.

## UpdatePolicy

`<UpdatePolicy />` is a root-level manifest declaration for dependency update intent. It is a sibling of `<Workspace />` children such as `<Security />` and package declarations, and appears in IR as `updatePolicy`.

```tsx
<UpdatePolicy
  rows={[
    {
      name: "typescript",
      kind: "tool",
      strategy: "rolling",
      level: "minor",
      reason: "Tooling can roll within compatible compiler minor updates.",
    },
    {
      name: "vite",
      kind: "tool",
      strategy: "rolling",
      level: "major",
      reason: "Frontend build tooling is reviewed through CI.",
    },
    {
      name: "react",
      kind: "dep",
      strategy: "manual",
      reason: "React runtime upgrades are coordinated manually.",
    },
  ]}
/>
```

Rows match by exact dependency `name` and `kind` (`tool`, `dep`, `peer`, or `any`). `manual` and `pinned` rows must not set `level`; `rolling` rows must set `patch`, `minor`, `major`, or `latest`. Optional `reason` text is carried into reports. Optional `packages` scopes a row to declarations from named workspace packages.

M50a does not apply policy-driven updates. The policy annotates `tspack outdated` so CI can see declared intent without outsourcing update motion to external bot churn.

### UpdatePolicy planning command

The declared `<UpdatePolicy />` can be inspected with `tspack outdated` and planned with `tspack update --policy --dry-run`. The policy planning command is read-only in M50b: it classifies candidates into allowed, blocked, unclassified, and not-applicable buckets, but it does not apply rolling updates, enforce security gates, or mutate lockfiles/stores/materialization.
