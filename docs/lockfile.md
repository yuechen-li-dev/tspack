# Lockfile (`ts-lock.toml`) in M6

`manifest.tsx` expresses intent while `ts-lock.toml` records resolved truth.

## Principles
- Deterministic and stable ordering.
- Human-reviewable and hash-pinned.
- Parse/write round-trips are stable.
- `sync` / `check` / `pack` must not mutate lockfiles (enforced in later milestones).

## M6 scope
M6 only implements lockfile data modeling, TOML parse/write, deterministic sorting, semantic validation, graph-vs-lock target consistency checks, and semantic diffing.

Non-goals: resolver, npm/git fetching, update/sync CLI flows, store, `node_modules`, pack/why/build/test/dev/publish.

## TOML shape
- `[lock]` header with `format=1`, `tool="tspack"`.
- `[[package]]` entries for source-pinned packages.
- `[[edge]]` entries connecting source graph nodes to resolved package ids.
- `[[target]]` entries for target outputs.
