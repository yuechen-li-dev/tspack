# Manifest (M1 subset)

`manifest.tsx` is a **typed document**, not an executable TypeScript program.
TSPack parses/analyzes AST and never executes user manifest code. Manifest TSX is TSPack DSL syntax, not React component code.

## Authoring type surface

TSPack provides a local TypeScript authoring surface at `tspack/manifest`.
- Import helpers, policy types, and JSX manifest elements directly from `tspack/manifest`.
- `tspack init` writes project-local declaration support (`.tspack/types/tspack-manifest.d.ts`, `.tspack/types/tspack-xtest.d.ts`, and `tspack-env.d.ts`) so standard TypeScript tooling resolves the module and native xTest globals without an npm package or editor extension.
- `tspack init --alongside` plus `tspack compat write` materializes the same local declaration support for existing npm projects.
- `tspack init` also writes `tsconfig.tspack.json` for editor and type-surface support of manifest files.
- `tsconfig.tspack.json` uses `jsx: preserve` so manifest TSX and `*.xtest.tsx` do not require React or `react/jsx-runtime`.
- `tsconfig.tspack.json` also sets `"types": []`, so app ambient packages such as `@types/react` do not leak JSX globals into manifest or xTest authoring.
- This surface is for editor autocomplete/typechecking only.
- Parser, normalized IR, and Go validation remain authoritative.
- Manifests are still statically parsed; helper functions are not runtime-executed.

## TypeScript/editor boundary

TSPack owns these TSX/type contexts:

- `manifest.tsx`
- `package.manifest.tsx`
- `**/*.manifest.tsx`
- `**/*.xtest.tsx`
- `.tspack/types/**/*.d.ts`

App and framework tooling owns normal app source such as `src/**/*.ts`, `src/**/*.tsx`, app test files, and framework configs. `tsconfig.tspack.json` intentionally allowlists only manifest files, `*.xtest.tsx`, and `.tspack/types/**/*.d.ts`, so ordinary app files such as `src/App.tsx` do not join the manifest editor project. If an app `tsconfig.json` includes root TSX broadly, exclude TSPack-owned files there and use `tsconfig.tspack.json` when editing manifests. TSPack parses only manifest entrypoints as manifest DSL.

`TsConfig.manifestEditor()` emits that default allowlist unchanged for normal
projects. `TsConfig.manifestEditor({ include, exclude })` is available for
advanced repositories that need a narrower editor project, such as compiler
repos, monorepos with intentionally invalid fixtures, or repositories that keep
`testdata` and template inputs beside real manifests. Most users should not
need it. Override lists must be arrays of safe non-empty relative glob strings,
and an override `include` list is exact, so add `.tspack/types/**/*.d.ts`
explicitly when you still want local manifest and xTest declaration support.

VS Code also needs to discover the manifest editor project. In practice that
means either:

- a root `tsconfig.json` solution file that references `tsconfig.tspack.json`, or
- an existing root app config that already routes manifest files correctly.

If `manifest.tsx` shows `Cannot find module 'tspack/manifest'` or asks for
`react/jsx-runtime`, use `TypeScript: Go to Project Configuration`. If VS Code
reports an inferred project or opens the wrong config, add a solution-style
root config instead of installing React just to satisfy manifest TSX.

TSPack also expects VS Code to use the workspace TypeScript SDK instead of the
bundled editor compiler. TSPack-owned templates and compat helpers point VS
Code at `node_modules/typescript/lib` by default, or another manifest-declared
relative SDK path when the repo layout needs one. If VS Code complains about
`ignoreDeprecations` while CLI typechecking succeeds, switch to `TypeScript:
Select TypeScript Version` and choose the workspace SDK.

## Troubleshooting manifest editor errors

- `Cannot find module "tspack/manifest"` usually means the local editor support files have not been materialized yet. Run `tspack compat write` in alongside projects, or rerun `tspack init --force` in init-owned projects.
- `Cannot find name 'Workspace'` and similar JSX symbol errors usually mean the manifest did not import every JSX component it uses from `tspack/manifest`.
- `Cannot find module 'react/jsx-runtime'` means the manifest is being checked under the wrong tsconfig. Use `tsconfig.tspack.json`, which keeps `jsx: preserve`, and restart the TypeScript server if VS Code had the project open before the file was created or before `tspack compat write` materialized the editor files.
- `Cannot find module 'tspack/manifest'` alongside JSX runtime errors usually means VS Code did not discover the manifest editor project at all. Confirm `tspack compat write` has created `.tspack/types/*`, then run `TypeScript: Go to Project Configuration`. A solution-style root `tsconfig.json` that references `tsconfig.tspack.json` fixes that discovery gap for repos that do not otherwise have a discoverable root TS project for manifests.

## M1 constraints

- File must be root `manifest.tsx`.
- Imports allowed only from `tspack/manifest` (including `import type`).
- Default export must be `define(<Workspace>...</Workspace>)`.
- Approved helpers: `define`, `defineDeps`, `npm`, `jsr`, `git`, `path`, `workspace`, `dep`, `peer`, `tool`.
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

Registry sources are explicit on each dependency and can be mixed without
changing the workspace runtime:

```tsx
const deps = defineDeps({
  colors: dep(npm("picocolors", "^1.1.1")),
  path: dep(jsr("@std/path", "^1.1.6")),
});
```

JSR names use `@scope/package`. JSR resolution does not require Deno. Node and
TypeScript compatibility artifacts use JSR's `@jsr/scope__package` names (for
example `@jsr/std__path`), while manifest, lock, and diagnostic identity remains
`jsr:@std/path`. Application imports currently use the compatibility name, such
as `import { join } from "@jsr/std__path"`; TSPack does not rewrite source files.
`tspack add --source jsr` and source-qualified `tspack why` expose this mapping
as structured package usage and human import guidance. The compatibility
spelling remains a Node/TypeScript detail, not a legal replacement for
`jsr("@std/path", ...)` in manifest truth.

JSR compatibility dependency keys are normalized by source: `@jsr/...` keys
refer to JSR packages and ordinary keys refer to npm packages, including in
optional dependency metadata. Malformed or ambiguous compatibility names fail
instead of becoming authoring identity. npm registry alias constraints such as
`"local-name": "npm:real-package@^1"` are normalized as a local reference plus
the semantic `npm:real-package` target; resolution and audit use the target,
while Node materialization uses the reference. Root `peer(...)` authoring and
transitive registry peers use source-qualified workspace environment slots.

```tsx
import {
  Package,
  Publish,
  Targets,
  Workspace,
  define,
  defineDeps,
  npm,
  tool,
} from "tspack/manifest";

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

## Package kinds

Package `kind` is semantic classification. It is preserved in the manifest IR
and introspection output, but it does not change dependency resolution, sync,
RunTarget execution, or package-manager behavior.

- `app`: browser/application package.
- `library`: reusable package intended for consumption by other packages.
- `service`: deployable backend/runtime unit.

Service packages commonly declare RunTargets with environment contracts and
service requirements:

```tsx
import { Env, Package, RunTargets, Service, Workspace, define } from "tspack/manifest";

export default define(
  <Workspace name="backend">
    <Package name="@app/api" version="0.1.0" kind="service">
      <RunTargets
        rows={[
          {
            name: "dev",
            runtime: "node",
            command: ["tsx", "src/server.ts"],
            url: "http://127.0.0.1:3000",
            env: [
              Env("DATABASE_URL", { required: true, secret: true }),
            ],
            requires: [
              Service("postgres", { tcp: "127.0.0.1:5432" }),
            ],
          },
        ]}
      />
    </Package>
  </Workspace>,
);
```

M59c only adds the `service` vocabulary. It does not add Docker Compose,
service startup/orchestration, containers, deployment artifacts, OpenAPI
generation, or new templates.




## Dependency aliases and explicit keys

`defineDeps({ ... })` attaches each object property name as a dependency-reference alias. When no explicit dependency `key` is present, references such as `<Tools values={[deps.typescript]} />` use that alias, so this common unscoped dependency shape keeps emitting `"typescript"`:

```tsx
const deps = defineDeps({
  typescript: tool(npm("typescript", "^5.9.0")),
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

## Dependency declaration provenance

The normalized manifest now retains an authoring declaration for every
dependency before projecting the effective dependency set used by graph and
resolution. A declaration records its source-qualified package identity,
constraint, semantic kind, optional flag, manifest source path, precedence
layer, authority, and editability. Later declarations for the same identity and
effective dependency key can shadow earlier declarations without deleting them
from the authoring tape. Distinct explicit aliases remain separate declarations.

Ordinary manifest dependencies receive project- or package-manifest provenance
automatically. Generated templates can set package-wide defaults with
`dependencyDeclaration`, and a dependency helper can override those defaults
with its `declaration` option. These fields are primarily for TSPack-owned
generators and future editing commands; normal hand-authored manifests do not
need to specify them.

```tsx
const deps = defineDeps({
  react: dep(npm("react", "^19"), {
    declaration: {
      origin: { kind: "concept", name: "react.app" },
      layer: "concept",
      authority: "generated",
      editability: "concept-owned",
    },
  }),
});
```

The tape is authoring truth, not resolved truth. `ts-lock.toml` continues to
record exact resolved packages and does not contain shadowed authoring history.
During incremental adoption, package.json declarations are recorded as
observed, non-editable compatibility evidence and package.json remains
authoritative.

### Authoring dependencies with `tspack add`

`tspack add <package>` creates or replaces an explicit owned declaration through
the authoring IR and tape, then uses the source-preserving projector for the
selected package's `dependencies.values` island. npm is the default source;
`--source jsr` selects JSR explicitly. TSPack does not search another registry
after a lookup failure. An unqualified package uses the newest stable release
from the selected source and writes a compatible caret constraint; an explicit
constraint is preserved. The subsequent normal mixed-source update writes exact
resolved truth to `ts-lock.toml` and captures required artifacts in the shared
store.

The default kind is TSPack `dep`, not an emulated npm section. `--optional`
sets the independent optional bit, `--kind peer` authors peer intent, and
`--source npm|jsr` explicitly selects the registry source. Package
selection accepts either the stable package name or its exact workspace-relative
root. When invoked below one package root, add/remove infer that package if the
mapping is unambiguous. Annotation manifests and alongside/package.json-native
projects remain non-editable because package.json retains authority.

`--dev` does not emulate npm `devDependencies`. The normalized `test` kind is
reserved but does not yet have a manifest helper or execution contract. A
TSPack tool also requires a `tool(...)` dependency plus selection through
`<Tools>`; because the M69 source projector edits only dependency islands,
`tspack add --tool` fails with guidance instead of authoring an unusable tool.

### Removing authored dependencies

`tspack remove <package>` removes the selected owned, editable authoring
declaration. The dependency tape is rebuilt before any source write. If a
lower-precedence concept or template declaration was shadowed, it becomes the
effective declaration again; the projector removes only the selected element
from the package's owned `dependencies.values` island.

Removal does not mean forced graph deletion. The selected package can stop
declaring a dependency while the same artifact remains in `ts-lock.toml`
through another workspace package or a transitive edge. Repeated removal and a
match that exists only as concept/template/derived provenance are no-op
operations. Use `--package <name>` in multi-package workspaces and
`--optional`, `--source npm|jsr`, or `--kind dep|peer|tool|test` when those semantics
are needed to disambiguate editable declarations. package.json-native projects keep npm authority and must use
`tspack npm uninstall` instead.

Without the explicit keys in the dependency-alias example above, the manifest frontend preserves the property alias fallback. Because the dependency identity derived from the npm package is `@biomejs/biome` or `react-dom`, Go IR validation reports `TSPACK_IR_UNKNOWN_DEPENDENCY_REF` if the alias and dependency identity do not match. `tspack migrate` follows this rule automatically: generated declarations get `key` whenever the TypeScript identifier differs from the npm package name, while packages such as `typescript` do not get noisy key options.

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

## Init templates and manifest authoring

`tspack init` templates generate the initial `manifest.tsx` and the editor boundary files (`tsconfig.tspack.json`, `.tspack/types/tspack-manifest.d.ts`, `.tspack/types/tspack-xtest.d.ts`, and `tspack-env.d.ts`). The generated manifest is the ongoing project contract; generated config files are tooling projections for authoring support.

## RunTarget env schema

`RunTarget` rows accept `env?: RunTargetEnvRow[]`. Use `Env(name, options)` for readable, typed declarations. `name` must match `[A-Za-z_][A-Za-z0-9_]*` and duplicate names on one RunTarget are rejected case-insensitively. Fields are:

- `required?: boolean` — the variable must be present at run time unless `default` is provided.
- `default?: string` — injected by `tspack run` when the variable is missing.
- `secret?: boolean` — values and defaults are redacted in diagnostics and JSON/list output.
- `description?: string` — documentation shown in missing-env diagnostics.

`tspack check` validates declaration shape and duplicates but does not require host secrets to exist.

## RunTarget service requirements

Use `Service(name, options)` inside a RunTarget `requires` array to declare backend service dependencies. Supported M59b options are `tcp`, `http`, `expectStatus`, `timeoutMs`, `optional`, and `description`.

```tsx
requires: [
  Service("redis", { tcp: "127.0.0.1:6379" }),
  Service("health-api", { http: "http://127.0.0.1:8080/health", expectStatus: 200 }),
]
```

Service names must be non-empty and unique per RunTarget. Exactly one endpoint kind is allowed. TCP endpoints use `host:port`; HTTP endpoints must use `http://` or `https://`.

### RunTarget readiness URL placeholders

HTTP RunTargets may include `${NAME}` placeholders in `url` so readiness follows resolved `Env(...)` values such as ports. Placeholder names use the same portable env variable shape as `Env(...)`. Values are resolved at `tspack run` time after host env, `--env` overlays, and RunTarget defaults. Secret env values cannot be interpolated into readiness URLs.

## Incremental `package.manifest.tsx` annotations

`package.manifest.tsx` now has two explicit modes:

- `definePackage(<Package ... />)` is the full TSPack package contract used by native workspace operations.
- `annotatePackage(<PackageAnnotations ... />)` is an incremental annotation file for an existing package.json package.

Annotation mode is intentionally not full package ownership. During incremental adoption, `package.json` remains authoritative and TSPack does not rewrite dependency sections, infer targets, generate run targets, project package.json, or write a lockfile. An annotation manifest can classify selected package.json dependencies as semantic `dep`, `peer`, or `tool` intent so `tspack adopt --report` can point out mismatches.

```tsx
import {
  PackageAnnotations,
  annotatePackage,
  defineDeps,
  dep,
  npm,
  peer,
  tool,
} from "tspack/manifest";

const deps = defineDeps({
  clsx: dep(npm("clsx", "^2.1.1")),
  react: peer(npm("react", "^19.0.0")),
  typescript: tool(npm("typescript", "^5.9.0")),
});

export default annotatePackage(
  <PackageAnnotations dependencies={{ values: [deps.clsx, deps.react, deps.typescript] }} />,
);
```

If an annotation manifest is used where a full split-workspace package contract is required, TSPack rejects it with a diagnostic instead of silently treating partial annotations as a native package.

### Suggested annotation manifests

Existing npm packages can bootstrap annotation mode with `tspack adopt --suggest-package <package-root>`. The command prints advisory `annotatePackage(<PackageAnnotations />)` source that uses the annotation dependency helpers documented above. The generated file is intentionally limited to dependency annotations: it does not declare targets, run targets, policies, or full package ownership, and `package.json` remains authoritative until the package explicitly moves to a full TSPack package contract. Once annotations exist, `tspack adopt --check-annotations` can be used in CI to report classification, range, and missing-dependency drift while keeping unannotated package.json dependencies as non-failing notices.
