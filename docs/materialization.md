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

The marker now stores deterministic JSON metadata for the current materialization plan:

- marker schema/version
- materializer name
- link mode
- materialization plan digest
- package/file/directory counts for the generated package projection

When `Clean=true`, materializer only deletes `node_modules` if marker exists. Without marker it refuses clean to avoid deleting unmanaged user directories.

Repeated `tspack sync` uses this marker as a fast-path contract. If the current lock/store/materialization plan still matches and a small root-level sanity check succeeds, TSPack skips relinking package files entirely.

This is intentionally not a full integrity verifier:

- `node_modules` is TSPack-owned generated output
- do not edit dependency files in place
- if you suspect drift or corruption, delete `node_modules` or run `tspack sync --force`

On Windows, package and `.bin` materialization now stage into temporary sibling directories and then swap into place. Replace/remove/write operations use bounded retries for transient locked-file cases such as `ERROR_ACCESS_DENIED`, `ERROR_SHARING_VIOLATION`, and `ERROR_LOCK_VIOLATION`.

## Store verification

Before writing package content, materializer:

1. derives package store hash from lock package (`hash`, fallback `tree_hash`)
2. verifies artifact in store
3. materializes extracted content into destination with hardlink-first regular-file writes and copy fallback when hardlinking is unavailable

No fetch or resolve happens in materialization.

## Graph safety (M53c)

`tspack sync` materialization is graph-safe for dependency cycles and shared dependency subgraphs. The node_modules writer tracks package identity on the active materialization path: it still writes the required package entry for a direct dependency edge, but if that package already appears on the current path, it does not recursively descend into that package again. This prevents unbounded paths such as `node_modules/a/node_modules/b/node_modules/a/node_modules/b/...` while preserving the existing strict nested compatibility layout.

A defensive dependency-depth guard also stops materialization with `TSPACK_MATERIALIZE_PATH_DEPTH_EXCEEDED` before future traversal bugs can create OS path-length failures. Cycle handling should normally prevent this guard from firing for ordinary cyclic graphs.

## Link modes (M10)

Supported:

- `copy`
- `hardlink`
- `auto` (resolves to hardlink-first)

Not yet implemented in M10:

- `symlink`

Unsupported modes return diagnostics.

## Hardlink-first behavior (M65a)

Regular package files now materialize from the content-addressed store with a hardlink-first strategy:

- TSPack tries to hardlink each regular store file into `node_modules`.
- If the filesystem, device, permissions, or platform do not allow hardlinks, TSPack falls back to byte-copying that file.
- Directory staging and final replacement remain atomic at the package-directory level.

M66c keeps that staged package-directory replacement model. Warm sync no longer needs to relink the full package tree when the materialized projection is already current, but TSPack still does not hash or restat every file on each run.

This reduces disk amplification and speeds up `tspack sync`, but it also means a store file and a materialized `node_modules` file may be the same underlying file identity.

Treat `node_modules` as immutable generated output:

- do not edit files under `node_modules`
- do not run tools that patch dependency files in place
- in-place writes to a hardlinked dependency file can mutate the shared store artifact for every project using that package version

TSPack does not add store corruption detection or copy-on-write in M65a. If hardlinking is unsupported in the current environment, materialization falls back to copying automatically.

## Non-goals in M10

- no sync/update CLI behavior
- no package fetching
- no dependency resolution expansion
- no script execution
- no pack/why commands
- no build/test/dev/publish commands


M11: `tspack sync` invokes the materializer. `node_modules` remains a compatibility output, not source-of-truth.

Update/sync relationship: a successful `tspack update` now resolves dependencies and populates store artifacts required by the lockfile so `tspack sync` can materialize without manual store priming.

When `tspack update` populates the store from local `path` or `workspace` sources, TSPack-managed internal artifact directories are not package source content. Store hashing and copying skip `.tspack/`, `tspack-artifacts/`, `.git/`, and `node_modules/`, and the copy helper also skips a destination subtree if the destination is inside the source root. This is internal artifact protection, not a publish exclude policy: ordinary package content such as checked-in `dist/**` files remains eligible for store population and later materialization.

## CLI compatibility hardening (M28)

M28 preserves package executable behavior and generates strict root `.bin` entries.

- executable permission bits from package artifacts are preserved through store extraction and `node_modules` materialization on POSIX.
- root `node_modules/.bin` is generated only from root-visible packages (same strict root visibility from `*:target:*` and `*:tool` edges).
- transitive-only packages are not hoisted into root `.bin`.
- `package.json` `bin` supports string and object shapes.
- invalid `bin` paths, missing bin targets, bin name conflicts, and write failures emit `TSPACK_MATERIALIZE_BIN_*` diagnostics.
- POSIX uses deterministic symlinks to package bin targets (with wrapper fallback).
- Windows writes `.cmd` shims for root bins.

Still not supported in materialization:

- semantic hoisting
- lifecycle script execution
- npm install / npx behavior

