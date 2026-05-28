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

Boundary scope matching supports:
- exact path, for example `src/index.ts`
- prefix form ending in `/**`, for example `src/**`

A boundary row may use `from` or `transitiveFrom`. Do not specify both in the same row.

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

## `transitiveFrom` matches files reachable from seeds

`transitiveFrom` is graph-reachable. It first finds seed files matching the same exact-path or `/**` pattern forms as `from`, then walks local relative runtime imports from each seed. The rule applies to imports physically written in every file in that reachable closure, including the seed file.

Use `transitiveFrom` when the policy should follow the local import graph from an entry-like source file:

```js
// Imports written in src/index.ts and every local file reachable from it are checked.
{
  transitiveFrom: "src/index.ts",
  denyDeps: ["react-dom"]
}
```

Given this graph:

```text
src/index.ts -> src/button.tsx -> react-dom
```

The row above denies the `react-dom` import in `src/button.tsx`, and the diagnostic path includes the seed-to-import chain:

```text
src/index.ts -> src/button.tsx -> react-dom
```

`transitiveFrom: "src/**"` is also valid. Every file under `src/` can act as a seed, so this can be broader and noisier than a single exact seed. Reporting is deterministic and uses the first deterministic local import path discovered for each seed.

Import cycles are handled with a seen set. A cycle does not infinite-loop, and the rule still applies to reachable files in the cycle.

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
  transitiveFrom: src/index.ts
    seed: src/index.ts
    path: src/index.ts -> src/button.tsx
    denyDeps: react-dom

External imports:
  react-dom
    decision: denied
    reason: denied by transitive boundary from src/index.ts
    diagnostic: TSPACK_BOUNDARY_EXPLICIT_DENY

Notes:
  boundary `from` matches the file containing the import statement.
```

Add `--json` for tooling:

```sh
tspack check --explain src/button.tsx --json
```

The JSON payload includes `reachableFrom`, `matchedRules`, `imports`, and `diagnostics` fields. Transitive matches include `transitiveFrom`, `seed`, and the reachable `path`. If the file is not reachable from any declared target entry, explain mode still reports matched rules and imports, and adds a note that target-scoped allowances could not be evaluated.

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
- type-level boundary rules

- `TSPACK_BOUNDARY_TYPE_ONLY_RUNTIME_IMPORT`: runtime import of a dependency declared with kind `type`.
