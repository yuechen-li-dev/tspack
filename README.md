# TSPack

TSPack is a TypeScript-first package manager focused on deterministic dependency intent (`manifest.tsx`) and resolved truth (`ts-lock.toml`).

## Current milestone

- **M7**: npm-compatible source resolution into lockfile package/edge/target data.

## What works now

- Minimal Go CLI entry point with `tspack --version` and `tspack help`.
- Go diagnostics model scaffold and deterministic sorting tests.
- TypeScript manifest frontend parses restricted `manifest.tsx` AST into deterministic JSON IR.
- Stable frontend diagnostics for forbidden imports/dynamic constructs/etc.
- Valid/invalid fixture coverage with golden IR snapshots.

## What intentionally does not work yet

- Git/path/workspace source resolution.
- Lockfile TOML semantics/read/write.
- `node_modules` materialization.
- package script execution.
- `build/test/dev/publish` package-manager commands.

## Testing

```bash
go test ./...
cd manifest-frontend
npm install
npm test
```
