# tspack why

`tspack why <query>` explains why a dependency, target, or lock package appears in the workspace.

## Query kinds
- Dependency key or external package name (`vue`, `@scope/pkg`)
- Target name (`react`)
- Package name where it is declared in the manifest
- Lock package ID (`npm:vue@3.4.0`)

Transitive packages can exist only in `ts-lock.toml` and may not be manifest dependency keys. Use the lock package ID form for those packages:

```bash
tspack why npm:<name>@<version>
```

For scoped packages, the lock ID keeps the package scope inside the npm ID:

```bash
tspack why npm:@scope/pkg@1.2.3
```

## Output model
- Manifest declaration reasons (kind, optional, scope)
- Reachability from targets and non-reachability from other targets
- Lock package matches and lock edges (direct target/tool edges and transitive package edges)
- Lock edge lines are deduplicated for readability while keeping deterministic ordering.
- Lock edges shown under a declaration are scoped to the relevant root edge(s) for that declaration and the reachable transitive lock edges below those roots. A declaration for `react@19` should not print transitive edges that are only reachable from a different declaration resolving `react@18`.

## Not-found guidance for transitive package names

If a bare package query does not match a declared dependency key/target (for example `tspack why loose-envify`) but matching lock packages exist, `TSPACK_WHY_NOT_FOUND` includes detail lines that list matching lock IDs and suggest exact commands, such as:

```bash
tspack why npm:loose-envify@1.4.0
```

When multiple lock versions match, suggestions are sorted by lock ID and every matching full lock ID is listed.

## Behavior with missing or invalid lockfile
- Missing lockfile emits `TSPACK_WHY_LOCKFILE_MISSING` warning and still returns manifest graph explanations.
- Without lock data, `tspack why` cannot suggest transitive lock package IDs for a not-found bare name.
- Invalid lockfile emits lock diagnostics and still tries to return graph-based explanations.

## Non-goals
- No security or vulnerability audit
- No license audit
- No dependency health scoring
- No update recommendation
