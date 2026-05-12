# tspack why

`tspack why <query>` explains why a dependency, target, or lock package appears in the workspace.

## Query kinds
- Dependency key or external package name (`vue`, `@scope/pkg`)
- Target name (`react`)
- Lock package ID (`npm:vue@3.4.0`)

## Output model
- Manifest declaration reasons (kind, optional, scope)
- Reachability from targets and non-reachability from other targets
- Lock package matches and lock edges (direct target/tool edges and transitive package edges)

## Behavior with missing or invalid lockfile
- Missing lockfile emits `TSPACK_WHY_LOCKFILE_MISSING` warning and still returns manifest graph explanations.
- Invalid lockfile emits lock diagnostics and still tries to return graph-based explanations.

## Non-goals
- No security or vulnerability audit
- No license audit
- No dependency health scoring
- No update recommendation
