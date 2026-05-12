# TSPack

TSPack is a TypeScript-first package manager focused on deterministic dependency intent (`manifest.tsx`) and resolved truth (`ts-lock.toml`).

## Current milestone

- **M0**: repository skeleton, contracts, fixture layout, and baseline test scaffolding.

## What works now

- Minimal Go CLI entry point with `tspack --version` and `tspack help`.
- Go diagnostics model scaffold and deterministic sorting tests.
- Placeholder internal packages for future subsystem ownership.
- TypeScript frontend scaffold with a placeholder exported function and test.
- Initial schemas, docs, and fixture directory structure.

## What intentionally does not work yet

- Manifest parsing/execution (`manifest.tsx`) is not implemented.
- Resolution, fetching, lockfile semantics, store, and materialization are not implemented.
- Commands like `sync`, `check`, `pack`, `why`, and build/test runner features are not implemented.

## Testing

Run Go tests from repository root:

```bash
go test ./...
```

Run frontend tests:

```bash
cd manifest-frontend
npm install
npm test
```

## Status

TSPack is **not yet usable as a package manager**. This milestone is foundation-only.
