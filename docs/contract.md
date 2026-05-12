# TSPack v1 Contract (Concise)

- TSPack is a TypeScript-first package manager.
- `manifest.tsx` expresses user intent.
- `ts-lock.toml` records resolved truth.
- The Go core is authoritative.
- The TypeScript frontend will parse `manifest.tsx` in later milestones.
- `node_modules` may be generated later only as a compatibility materialization.
- Build/test/dev/publish workflows are explicit non-goals for v1.
- Fetch is not execute.
- Sync/check/pack must not mutate the lockfile.
