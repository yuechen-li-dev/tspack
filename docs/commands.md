# Commands (M11)

- `tspack check`: validates manifest/frontend, IR, graph, boundaries, type surfaces, and lock consistency when lockfile exists. It does **not** write lockfile, store artifacts, or `node_modules`.
- `tspack update`: resolves sources and writes `ts-lock.toml` deterministically. In orchestration code, resolver clients can be injected (tests use fake clients; no live network is required).
- `tspack sync`: requires an existing lockfile, validates lock consistency against graph, then materializes strict `node_modules` from store artifacts. It never mutates `ts-lock.toml`.

## Manifest frontend bridge

Go command orchestration invokes the manifest frontend parser bridge (`manifest-frontend/dist/src/cli.js`) and consumes deterministic JSON output from `parseWorkspace(...)`.

If that built CLI is missing, commands fail with `TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED` and an actionable message to run:

`cd manifest-frontend && npm run build`

The bridge never executes `manifest.tsx` directly.

`node_modules` remains a generated compatibility artifact, not source-of-truth.
