# ScriptC hot-path M71a dogfood

This fixture starts as an ordinary Node + TypeScript application. The `app`
target remains owned by `tsc`; only `src/hot/**` is owned by ScriptC. The app
depends on the ScriptC native executable artifact and crosses the boundary once
per batch through a small JSON sidecar protocol. It never imports ScriptC-owned
source.

`tsconfig.json` deliberately includes all `src/**/*.ts`. TSPack projects the
ScriptC-owned source out of the `tsc` build, so users do not maintain a duplicate
exclude list.

The workload reports Node kernel time, ScriptC kernel time, total sidecar time,
and boundary overhead. This bridge is suitable for coarse batches and numeric
kernels, not tiny per-function calls.
