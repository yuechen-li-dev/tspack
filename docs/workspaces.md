# Workspaces (M6b)

Root manifest owns topology, package manifests own package contracts, and one `ts-lock.toml` owns resolved truth for the workspace.

```text
manifest.tsx
ts-lock.toml
packages/core/package.manifest.tsx
packages/react/package.manifest.tsx
```

Non-goals in M6b: no resolver/fetch/sync/node_modules/materialization/build/test/dev/publish.

## Annotation-only package manifests

Split workspace rows still require full `definePackage(<Package ... />)` package contracts for native workspace parsing. Incremental `annotatePackage(<PackageAnnotations ... />)` files are discovery/reporting inputs for `tspack adopt --report`; they do not populate the native `packages` list and do not satisfy update, sync, build, pack, run, or lock generation requirements.

This keeps the adoption ladder explicit: root `manifest.tsx` can observe workspace-level compatibility, package.json remains authoritative, and package-local annotation files can document semantic dependency intent before a package opts into full TSPack ownership.
