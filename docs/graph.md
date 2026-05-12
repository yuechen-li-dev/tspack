# Graph model (M3)

M3 introduces a queryable internal graph built from validated Manifest IR.

## Model

- WorkspaceGraph contains package nodes.
- PackageNode contains dependency nodes and target nodes.
- TargetNode contains allowed runtime and peer dependencies scoped per target.

## Key semantics

- Dependencies belong to targets/scopes, not globally to the entire package runtime surface.
- Tool dependencies are package/tool scope and are **not** target runtime deps by default.
- Optional peers are represented per-target (e.g. `vue` optional peer only on `vue` target).
- Build is defensive and returns deterministic diagnostics for malformed IR.

## Non-goals in M3

- No package resolution/fetching.
- No lockfile I/O.
- No node_modules materialization.
- No boundary enforcement yet.

## Dependency kinds

- `runtime`: must be available to runtime code for the target/package.
- `dep`: authoring/general dependency intent; treated as runtime-equivalent at graph/resolver boundaries.
- `peer`: consumer-provided runtime dependency, scoped per target.
- `tool`: build/dev/tooling-only dependency; excluded from target runtime deps.
- `type`: type-only dependency; may be referenced by target deps metadata but not runtime-visible.
- `test`: reserved test-only metadata in v1; not runtime-visible.
- `workspace`: dependency on another workspace package; runtime/type/tool visibility comes from where it is referenced.
