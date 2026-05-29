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


## JSON output

`tspack why <query> --json` emits a stable structured report on stdout instead of human text. The report uses two-space indentation, has no timestamps, and keeps handled diagnostics inside the `diagnostics` array so successful and expected diagnostic paths do not mix human text into stdout/stderr.

Supported forms include:

```bash
tspack why react --json
tspack why react --package @acme/components --json
tspack why npm:loose-envify@1.4.0 --json
tspack why loose-envify --json
```

The top-level report contains `command`, `query`, the `package` filter or `null`, `ok`, path fields, `summary`, `explanations`, and `diagnostics`:

```json
{
  "command": "why",
  "query": "react",
  "package": "@acme/components",
  "ok": true,
  "summary": {
    "explanations": 1,
    "diagnostics": 0,
    "warnings": 0,
    "errors": 0
  },
  "explanations": [
    {
      "kind": "dependency",
      "package": "@acme/components",
      "dependencyKey": "react",
      "dependencyKind": "peer",
      "source": {
        "kind": "npm",
        "package": "react",
        "range": ">=18 <20"
      },
      "reachableFrom": [
        {
          "package": "@acme/components",
          "target": "core",
          "reason": "peer",
          "ref": "@acme/components:target:core"
        }
      ],
      "lockEdges": [
        {
          "from": "@acme/components:target:core",
          "to": "npm:react@19.2.6",
          "kind": "peer",
          "optional": false
        }
      ]
    }
  ],
  "diagnostics": []
}
```

`lockEdges` are structured objects and remain declaration-scoped: a dependency explanation includes only the lock root edge(s) relevant to that declaration plus reachable transitive edges below those roots. Lock-package ID queries include matching lock packages and inbound/outbound lock edges.

In JSON mode, not-found and lockfile diagnostics are also structured. For example, a bare transitive-name miss such as `tspack why loose-envify --json` returns parseable JSON with `TSPACK_WHY_NOT_FOUND`; its `details` list includes matching lock IDs and suggested full lock-ID commands when lock data is available.

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
