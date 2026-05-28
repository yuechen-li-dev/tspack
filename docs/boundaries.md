# Runtime boundaries (M4)

M4 validates runtime imports discovered from source graph walking against M3 target dependency permissions.

Rules implemented:
- runtime external import must be target-allowed
- undeclared imports fail
- tool dependencies imported by runtime targets fail
- peer scope violations fail
- optional peer leaks fail
- explicit `denyDeps` fail
- explicit `allowDeps` rows are conservative: if row matches and package not in `allowDeps`, produce allow violation when target would otherwise allow

Boundary `from` matching supports:
- exact path
- prefix form ending in `/**`

## `from` matches the importing file

`from` is file-pattern based. It matches the source file where an import statement is physically written, not every file that is transitively reachable from an entry point.

For example, this row checks only imports written inside `src/index.ts`:

```js
// Only imports written inside src/index.ts are checked by this row.
{
  from: "src/index.ts",
  denyDeps: ["react-dom"]
}
```

If `src/index.ts` imports `src/button.tsx`, and `src/button.tsx` imports `react-dom`, the `react-dom` import is not denied by the exact-file row above. The importing file for that external dependency is `src/button.tsx`, not `src/index.ts`.

Use a file-set pattern when the policy should apply to imports written anywhere under a directory:

```js
// Imports written anywhere under src/ are checked by this row.
{
  from: "src/**",
  denyDeps: ["react-dom"]
}
```

The `src/**` row checks imports physically located in files such as:

- `src/index.ts`
- `src/button.tsx`
- `src/nested/card.ts`

Import-chain diagnostics may still show transitive reachability, for example:

```text
src/index.ts -> src/button.tsx -> react-dom
```

That trace explains why a violating file was reachable from a checked target entry. It is related to boundary enforcement, but it is not the same concept as matching a boundary row. The boundary row match is based on the importing file where the external import occurs.

If you want “everything reachable from this entry point” semantics, that is not what `from` means today. Use `from: "src/**"` for file-set restrictions. A future `transitiveFrom` rule may support graph-reachable restrictions.


## Debugging one file with `check --explain`

Use `tspack check --explain <file>` when a boundary diagnostic is surprising or when you want to inspect one source file without running normal human diagnostics output. The command is read-only and does not update the manifest, lockfile, store, or `node_modules`.

Example:

```sh
tspack check --explain src/button.tsx
```

Sample text output:

```text
Boundary explanation for src/button.tsx

Reachable from targets:
  core
    path: src/index.ts -> src/button.tsx

Matched boundary rules:
  from: src/**
    denyDeps: react-dom

External imports:
  react
    decision: allowed
    reason: declared dependency for target core

  react-dom
    decision: denied
    reason: denied by boundary rule matching this file
    diagnostic: TSPACK_BOUNDARY_EXPLICIT_DENY

Relative imports:
  ./style-helper.js
    resolved: src/style-helper.ts

Notes:
  boundary `from` matches the file containing the import statement.
```

Add `--json` for tooling:

```sh
tspack check --explain src/button.tsx --json
```

The JSON payload includes `reachableFrom`, `matchedRules`, `imports`, and `diagnostics` fields. If the file is not reachable from any declared target entry, explain mode still reports matched rules and imports, and adds a note that target-scoped allowances could not be evaluated.

Dependency identity matching:
- npm dependencies match the declared dependency key, source package, or source name when present.
- workspace and path dependencies match only exact declared identifiers: dependency key, source name, or source package when present.
- workspace/path matching does not allow arbitrary packages just because the source kind is `workspace` or `path`.

Node builtin policy in M4:
- builtins are classified by scanner
- dependency boundary checks ignore builtins for now (TODO: environment/runtime policy)

M4 non-goals:
- package fetching/resolution
- lockfile/store/node_modules flows
- build/test/dev/publish command implementation
- transitive entry-point boundary rules such as `transitiveFrom`

- `TSPACK_BOUNDARY_TYPE_ONLY_RUNTIME_IMPORT`: runtime import of a dependency declared with kind `type`.
