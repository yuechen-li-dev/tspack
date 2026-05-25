# tspack why

`tspack why <query>` explains why a dependency, target, or lock package appears in the workspace.

## Query kinds
- Dependency key or external package name (`vue`, `@scope/pkg`)
- Target name (`react`)
- Lock package ID (`npm:vue@3.4.0`)

For transitive dependencies in the lockfile, prefer lock package IDs:

```bash
tspack why npm:<name>@<version>
```

## Output model
- Manifest declaration reasons (kind, optional, scope)
- Reachability from targets and non-reachability from other targets
- Lock package matches and lock edges (direct target/tool edges and transitive package edges)
- Lock edge lines are deduplicated for readability while keeping deterministic ordering.

## Not-found guidance for transitive package names

If a bare package query does not match a declared dependency key/target (for example `tspack why loose-envify`) but matching lock packages exist, `TSPACK_WHY_NOT_FOUND` includes detail lines that list matching lock IDs and suggest an exact command, such as:

```bash
tspack why npm:loose-envify@1.4.0
```

## Behavior with missing or invalid lockfile
- Missing lockfile emits `TSPACK_WHY_LOCKFILE_MISSING` warning and still returns manifest graph explanations.
- Invalid lockfile emits lock diagnostics and still tries to return graph-based explanations.

## Non-goals
- No security or vulnerability audit
- No license audit
- No dependency health scoring
- No update recommendation
