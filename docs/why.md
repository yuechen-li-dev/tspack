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


## Reverse why

`tspack why --reverse <query>` answers the inverse lock-graph question: which declared roots ultimately pull a locked package into `ts-lock.toml`?

Use reverse why when you want to know who pulls a transitive package in, or which package targets/tools transitively depend on a specific lock package.

Supported reverse query forms are lock/package oriented:

```bash
tspack why --reverse loose-envify
tspack why --reverse npm:loose-envify@1.4.0
tspack why --reverse npm:loose-envify
tspack why --reverse react --package @acme/app
tspack why --reverse react --json
```

Bare package names, including scoped names such as `@biomejs/cli-linux-x64`, match all locked npm packages with that name. If multiple versions are locked, reverse why reports each matching lock package in deterministic lock-ID order.

Reverse paths are printed from the root that introduced the package toward the queried package, even though the graph walk runs over incoming lock edges. For example:

```text
Reverse why: npm:loose-envify@1.4.0

npm:loose-envify@1.4.0 is pulled in by:

  @acme/app:target:app
    path:
      @acme/app:target:app
      -> npm:react@18.3.1
      -> npm:loose-envify@1.4.0
```

Roots are lock edge sources outside the locked package set, such as package targets (`@pkg:target:<target>`), package tools (`@pkg:tool`), and other project/manifest roots that point into the lock graph. Transitive lock packages are not treated as roots.

`--package <pkg>` filters reverse paths to roots owned by that package. If the lock package exists but no reverse path remains after filtering, the command succeeds with an empty reverse path list and reports that no reverse paths came from the package.

Reverse why requires `ts-lock.toml`. A missing lockfile is an error in reverse mode because the command cannot answer from manifest declarations alone; run `tspack update` first.

### Reverse JSON output

`tspack why --reverse <query> --json` writes stdout-only JSON with `mode: "reverse"`, matching `lockPackages`, structured `reverse` paths, and diagnostics. Each reverse path includes the matched lock package, root, root-to-query node path, and edge objects:

```json
{
  "command": "why",
  "mode": "reverse",
  "query": "loose-envify",
  "package": null,
  "ok": true,
  "summary": {
    "lockPackages": 1,
    "reversePaths": 1,
    "diagnostics": 0,
    "warnings": 0,
    "errors": 0
  },
  "lockPackages": [
    {
      "id": "npm:loose-envify@1.4.0",
      "name": "loose-envify",
      "version": "1.4.0",
      "source": "npm"
    }
  ],
  "reverse": [
    {
      "lockPackage": "npm:loose-envify@1.4.0",
      "root": "@acme/app:target:app",
      "path": [
        "@acme/app:target:app",
        "npm:react@18.3.1",
        "npm:loose-envify@1.4.0"
      ],
      "edges": [
        {
          "from": "@acme/app:target:app",
          "to": "npm:react@18.3.1",
          "kind": "runtime",
          "optional": false
        },
        {
          "from": "npm:react@18.3.1",
          "to": "npm:loose-envify@1.4.0",
          "kind": "runtime",
          "optional": false
        }
      ]
    }
  ],
  "diagnostics": []
}
```

## Not-found guidance for transitive package names

If a bare package query does not match a declared dependency key/target (for example `tspack why loose-envify`) but matching lock packages exist, `TSPACK_WHY_NOT_FOUND` includes detail lines that list matching lock IDs and suggest exact commands, such as:

```bash
tspack why npm:loose-envify@1.4.0
```

When multiple lock versions match, suggestions are sorted by lock ID and every matching full lock ID is listed.

## Behavior with missing or invalid lockfile
- Missing lockfile emits `TSPACK_WHY_LOCKFILE_MISSING` warning and still returns manifest graph explanations for normal why.
- Reverse why treats a missing lockfile as an error because reverse paths require lock graph edges.
- Without lock data, `tspack why` cannot suggest transitive lock package IDs for a not-found bare name.
- Invalid lockfile emits lock diagnostics and still tries to return graph-based explanations.

## Non-goals
- No security or vulnerability audit
- No license audit
- No dependency health scoring
- No update recommendation

## Related docs

- See `docs/claude-fooding-phase6.md` for the Phase 6 pack/why remediation closeout.

## Package capabilities

When a matching lock package declares lifecycle capabilities, `tspack why` prints them under `capabilities` with `execution: blocked by default`. JSON output includes a `capabilities` array on lock package objects with `{ kind, script, command, execution, acknowledged, acknowledgementReason }`. Human output prints `acknowledged: true` with the manifest reason when a lifecycle capability matches project policy, or `acknowledged: false` otherwise. Reverse why keeps the same reachability semantics and includes acknowledgement metadata for the matched lock package.
