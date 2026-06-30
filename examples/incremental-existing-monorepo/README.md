# Incremental existing monorepo

This fixture models an npm-workspaces monorepo before full TSPack migration.
The root `package.json` remains authoritative. The `packages/ui/package.manifest.tsx`
file is an annotation-only package manifest that classifies selected existing
`package.json` dependencies as TSPack semantic `dep`, `peer`, and `tool` intent.

Run:

```bash
go run ./cmd/tspack adopt --report --root examples/incremental-existing-monorepo
```

The report should show the UI package annotation and warn that `react` is
annotated as a peer while package.json currently lists it in `dependencies`.
No `package.json` or `ts-lock.toml` files are written.
