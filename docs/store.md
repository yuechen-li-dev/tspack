# Content-addressed store (M9)

TSPack M9 introduces a local content-addressed store for resolved package artifacts.

## Purpose

The store owns local artifact identity. `node_modules` is not a source of truth; it will be a later materialized view.

Current sources of truth are:
- manifest IR and graph
- `ts-lock.toml`
- the content-addressed store

## Hash format

Store hashes are normalized to `sha256:<hex>`. Inputs may use raw hex or `sha256-<hex>` and are normalized.

## Deterministic layout

```
<store-root>/
  blobs/sha256/<prefix>/<hex>.tgz
  extracted/sha256/<prefix>/<hex>/
  metadata/sha256/<prefix>/<hex>.json
```

Paths are derived only from hash, not package name.

## Artifact handling

- npm tarballs: bytes are hashed, stored in `blobs`, extracted into `extracted`.
- directory/tree artifacts (git/path/workspace): deterministic file snapshot copied to `extracted`.

Directory behavior:
- skips `.git` and `node_modules`
- ignores symlinks
- hashes sorted relative paths + bytes

Lifecycle scripts are metadata only and are never executed by store operations.

## Security

Tar extraction rejects absolute paths, `..` traversal segments, and destination escape attempts.

## Sync hydration and offline behavior

- `tspack sync` materializes from `ts-lock.toml`, not from fresh dependency resolution.
- If a required artifact is already present locally, sync verifies and reuses it without network access.
- If the local store is empty or missing required artifacts, sync may hydrate locked artifacts from their recorded source identity before materialization.
- For locked npm packages, sync uses the locked package name/version plus lockfile integrity/hash checks; it does not pick newer versions or rewrite the lockfile.
- If a required artifact is missing and the source cannot be fetched or verified, sync fails clearly instead of pretending the workflow is offline.

- `Has(hash)` checks local availability.
- `Get(hash)` finds local store references.
- `Verify(hash)` validates presence and metadata/hash consistency.

## Non-goals in M9

- no `node_modules` materialization
- no sync/update CLI behavior
- no package fetching
- no GC implementation


## Materialization integration (M10)

The store is the artifact source for `internal/materialize` node_modules generation. The materializer verifies store artifacts before writing compatibility output.
