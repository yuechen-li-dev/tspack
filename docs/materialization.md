# Materialization (M10)

`node_modules` in TSPack is a generated compatibility artifact.

## Source of truth

TSPack dependency truth remains:

- manifest IR / validated graph
- `ts-lock.toml`
- content-addressed store artifacts

`node_modules` is disposable output. It may be deleted and recreated from lockfile + store.

## Materializer seam

M10 adds `internal/materialize` with a `Materializer` interface and a `NodeModulesMaterializer` implementation.

This keeps layout strategy separate from resolver/checker/lock/store and leaves a seam for future materializers:

- strict `node_modules` (current)
- virtual filesystem
- import-map based materialization
- PnP-like/runtime loaders

## Strict node_modules behavior

The current implementation is intentionally strict and deterministic:

- no semantic hoisting
- root `node_modules` gets only lock edges from `*:target:*` and `*:tool`
- package-to-package lock edges are nested under parent package `node_modules`
- transitive packages are not promoted to workspace root unless directly referenced

This reduces phantom dependency exposure compared with npm hoisting.

## Safety and cleaning

Generated root includes marker:

- `node_modules/.tspack-materialized`

When `Clean=true`, materializer only deletes `node_modules` if marker exists. Without marker it refuses clean to avoid deleting unmanaged user directories.

## Store verification

Before writing package content, materializer:

1. derives package store hash from lock package (`hash`, fallback `tree_hash`)
2. verifies artifact in store
3. copies extracted content into destination

No fetch or resolve happens in materialization.

## Link modes (M10)

Supported:

- `copy`
- `auto` (currently resolves to `copy`)

Not yet implemented in M10:

- `symlink`
- `hardlink`

Unsupported modes return diagnostics.

## Non-goals in M10

- no sync/update CLI behavior
- no package fetching
- no dependency resolution expansion
- no script execution
- no pack/why commands
- no build/test/dev/publish commands
