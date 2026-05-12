# Source resolvers (M8)

TSPack resolves dependency sources into normalized lockfile package records.

Supported source kinds:
- `npm`
- `git`
- `path`
- `workspace`

## Git

- Git refs (`tag`, `rev`, `branch`, `ref`) resolve to an exact commit.
- Lockfile truth stores exact `rev` and `tree_hash`.
- Branches are update intent only; lock output is always commit-pinned.
- M8 tests only use local git repositories (no live network).

## Path

- Path dependencies use safe relative paths.
- Absolute paths are rejected.
- Traversal outside the workspace/package root is rejected.
- Lock records `source="path"`, normalized `path`, and deterministic content hash.

## Workspace

- Workspace dependencies resolve by workspace package name.
- Missing package names fail with a diagnostic.
- Lock records `source="workspace"`, package/root identity, and deterministic content hash.

## Deterministic local hashing

For `path` and `workspace` sources, M8 hashes directory contents deterministically:
- sorted relative file paths
- regular files only
- skipped directories: `.git`, `node_modules`
- symlinks are currently ignored

## Script handling

For git/path/workspace packages, lifecycle scripts are only recorded as capabilities.
Scripts are never executed during resolution.

## M8 non-goals

- no node_modules materialization
- no content-addressed store
- no sync/update CLI behavior changes
- no package script execution
