# TSPack

TSPack is a TypeScript-first package manager focused on deterministic dependency intent (`manifest.tsx`) and resolved truth (`ts-lock.toml`).

## Current milestone

- **M14**: security/capability policy hardening (fetch-not-execute, lifecycle capability visibility).

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


## M11 commands

- `tspack check`
- `tspack update`
- `tspack sync`

Out of scope in v1: build/test/dev/publish/pack/why/add/remove.
