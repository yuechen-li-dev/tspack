# Incremental adoption design

## Problem

TSPack should not require an all-or-nothing migration before users get value.
Existing TypeScript projects already have a working `package.json`, lockfile,
`tsconfig.json`, Vite config, scripts, editor behavior, CI assumptions, and a
package-manager workflow. Replacing all of that up front creates an adoption
cliff.

The incremental adoption property is: get value from the first manifest file,
with everything else unchanged. M62a starts one step earlier by allowing TSPack
to observe a package.json-native project before a manifest exists.

## Package.json as compatibility substrate

`package.json` is substrate, not enemy. TSPack should work alongside it the way
React worked alongside the DOM: the compatibility surface remains in place while
a typed, explicit TSPack contract can grow over time.

The long-term direction is:

- `package.json` remains the compatibility substrate.
- `manifest.tsx` becomes an increasing source of truth over time.
- TSPack value scales with how much intent the user declares.
- Full migration is optional.

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

## Non-goals

M62a intentionally does not add:

- package.json script fallback for `tspack run`;
- package.json mutation;
- manifest generation;
- `init --alongside` writes;
- projection writes;
- package.json deletion;
- npm install execution from TSPack;
- changes to update/sync semantics.

## Future milestones

Likely follow-up work includes:

- `init --alongside` dry-run and then explicit writes;
- `ts-lock.toml` generation from observed package.json evidence;
- security lifecycle warnings for observed package.json dependencies;
- `why` and `explain` support for observed package graph evidence;
- partial per-package `package.manifest.tsx` annotation;
- projection preview before any compatibility-output writes.
