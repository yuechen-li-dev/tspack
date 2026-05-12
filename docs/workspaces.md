# Workspaces (M6b)

Root manifest owns topology, package manifests own package contracts, and one `ts-lock.toml` owns resolved truth for the workspace.

```text
manifest.tsx
ts-lock.toml
packages/core/package.manifest.tsx
packages/react/package.manifest.tsx
```

Non-goals in M6b: no resolver/fetch/sync/node_modules/materialization/build/test/dev/publish.
