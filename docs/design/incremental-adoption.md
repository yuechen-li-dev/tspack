# Incremental adoption design

## Problem

TSPack should not require an all-or-nothing migration before users get value.
Existing TypeScript projects already have a working `package.json`, lockfile,
`tsconfig.json`, Vite config, scripts, editor behavior, CI assumptions, and a
package-manager workflow. Replacing all of that up front creates an adoption
cliff.

The incremental adoption property is: get value from the first manifest file,
with everything else unchanged. M62a starts one step earlier by allowing TSPack
to observe a package.json-native project before a manifest exists. M62b adds
`tspack init --alongside`, which creates only a small root `manifest.tsx` beside
an existing npm project. M63c adds an explicit npm delegation bridge so
package.json-native projects can keep using real npm behavior while adoption
proceeds.

## Package.json as compatibility substrate

`package.json` is substrate, not enemy. TSPack should work alongside it the way
React worked alongside the DOM: the compatibility surface remains in place while
a typed, explicit TSPack contract can grow over time.

The long-term direction is:

- `package.json` remains the compatibility substrate.
- `manifest.tsx` becomes an increasing source of truth over time.
- TSPack value scales with how much intent the user declares.
- Full migration is optional.

## Explicit npm delegation

TSPack does not reimplement npm compatibility. npm remains the package
materializer for package.json/package-lock workflows, including registry access,
lifecycle behavior, lockfile updates, script execution, and npm edge cases.

`tspack npm <npm-args...>` is an explicit bridge to the real npm CLI. It runs
npm in the selected project root, passes the provided npm arguments through, and
returns npm's exit code. Examples include:

- `tspack npm install`
- `tspack npm ci`
- `tspack npm install -D vite`
- `tspack npm exec vite -- --version`
- `tspack npm run build`

This is not package-manager emulation and it is intentionally not `tspack
install`. TSPack does not mutate `package.json`, project package lockfiles,
`manifest.tsx`, or `ts-lock.toml` on behalf of this bridge; any package.json or
lockfile changes are npm's own changes from the user's explicit npm command.
`TSPACK_NPM` may point at a specific npm executable when PATH resolution is not
appropriate.

## Adoption modes

- **Mode 0: package-json-only observation** — read package.json and nearby
  lockfiles, report what exists, and write nothing.
- **Mode 1: observe with root manifest** — add a root `manifest.tsx` beside
  package.json and use it to describe intent without taking over every workflow.
- **Mode 2: annotate per package** — add package-level manifest annotations for
  selected workspaces or packages.
- **Mode 3: govern with targets/runTargets** — move selected build/test/dev
  actions into explicit TSPack targets and RunTargets.
- **Mode 4: project package.json as compatibility output** — preview and later
  write package.json projections from TSPack intent where that is useful.

## M62a scope

M62a adds the foundation only:

- read-only package.json observation IR;
- read-only adoption report API and `tspack adopt --report` CLI;
- lockfile presence detection for npm, pnpm, Yarn, and Bun lockfiles;
- a package.json-native Vite/React/TypeScript dogfood project at
  `examples/incremental-existing-react/`;
- tests that assert reporting does not write `manifest.tsx` or `ts-lock.toml`.


## M62b alongside init flow

`tspack init --alongside` requires an existing `package.json`, observes it with
the same package.json observation path used by `tspack adopt --report`, and
writes a minimal root `manifest.tsx`. The command does not mutate
`package.json`, does not touch npm lockfiles, does not run npm install, does not
run TSPack update or sync, and does not generate `ts-lock.toml`.

The generated manifest declares workspace identity from `package.json` `name`
(or the directory basename when the name is absent) and managed editor
compatibility files using helper presets:

```tsx
import {
  CompatFiles,
  JsonFile,
  TsConfig,
  VSCode,
  Workspace,
  defineWorkspace,
} from "tspack/manifest";

export default defineWorkspace(
  <Workspace name="existing-project-name">
    <CompatFiles>
      <JsonFile
        path="tsconfig.tspack.json"
        value={TsConfig.manifestEditor()}
      />
      <JsonFile path=".vscode/settings.json" value={VSCode.settings()} />
      <JsonFile path=".vscode/extensions.json" value={VSCode.extensions()} />
    </CompatFiles>
  </Workspace>,
);
```

By default, `init --alongside` writes only `manifest.tsx`. The declared compat
files are inspectable intent until the user explicitly runs:

```bash
tspack compat diff
tspack compat write
```

`--dry-run` previews the manifest and writes nothing. `--force` replaces an
existing root `manifest.tsx` but still leaves `package.json`, lockfiles,
`ts-lock.toml`, and compat files untouched. After the manifest exists,
`tspack adopt --report` transitions from `package-json-only` to `observe`: npm
scripts remain npm scripts, dependencies remain observed package.json evidence,
and package.json scripts are not TSPack RunTargets. Use `tspack npm run ...` for
real npm script delegation; `tspack run ...` remains manifest-only.

## Non-goals

M62a intentionally does not add:

- package.json script fallback for `tspack run`;
- package.json mutation;
- package manifest inference;
- package.json projection or migration;
- projection writes;
- package.json deletion;
- implicit npm install execution from TSPack commands;
- changes to update/sync semantics.

## Future milestones

Likely follow-up work includes:

- observed `ts-lock.toml` generation only if explicitly designed later;
- security lifecycle warnings for observed package.json dependencies;
- `why` and `explain` support for observed package graph evidence;
- partial per-package `package.manifest.tsx` annotation;
- projection preview before any compatibility-output writes;
- security lifecycle reporting around the npm-observed graph;
- broader package manager support only if deliberately scoped later.

## M62c observed npm why

M62c adds `tspack why <package>` value for package.json-native npm projects before a full TSPack migration. When no `ts-lock.toml` is present but `package.json` exists, `why` automatically reads observed npm metadata and labels the source as `observed npm package.json/package-lock`.

The observed explanation reads only:

- `package.json` direct dependency sections (`dependencies`, `devDependencies`, `peerDependencies`, and `optionalDependencies`);
- npm `package-lock.json` v2/v3 `packages` entries when the lockfile is present.

This path does not resolve packages from the registry, run `npm install`, generate lockfiles, crawl `node_modules`, infer package manifests, or mutate `package.json`. npm remains the package materializer; TSPack explains the compatibility metadata already on disk.

With a package lock, direct packages report the package.json section and requested range, plus the observed locked version and location when available. Transitive packages are explained by a simple observed lockfile graph: root direct dependencies are traversed through each package-lock entry's `dependencies` object, and `why` prints one or more shortest chains such as `root -> vite -> esbuild`. This is intentionally an observed lockfile explanation rather than a complete npm hoisting resolver.

Without `package-lock.json`, `why` still explains direct package.json dependencies. Transitive explanations require npm's lockfile, so misses report that limitation and suggest running `tspack npm install` if the user wants npm to create the lockfile explicitly.

Observed why currently supports npm package-lock only. If pnpm, Yarn, or Bun lockfiles are present, TSPack reports that limitation instead of attempting to parse those formats.
