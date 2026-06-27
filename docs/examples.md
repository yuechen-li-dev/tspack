# Examples

## Runtime Switch Notes

`examples/runtime-switch-notes/` is the current dogfooding smoke for TSPack's runtime-profile and runtime-surface story.

It is intentionally small and boring: a static notes page with Node.js, Bun, and Deno RunTargets, source hints for inspect, native xTest coverage, a packable UI library, and documented check/doctor/why/format/lint commands.

Use it before release when you want one sample to exercise these surfaces together:

- `manifest.tsx` workspace and package contracts
- one-line workspace `runtime="nodejs" | "bun" | "deno"` profile switching
- explicit `runtime: "node" | "bun" | "deno"` RunTargets with readiness and cwd policy
- native xTest facts, snapshots, type assertions, and inspect assertions
- `pack --verify` for a library artifact
- `why` for a workspace dependency
- `doctor runtime`, `doctor run`, `doctor security`, and `doctor format`
- Biome-backed `format` and `lint`

See the example README and dogfooding report for the exact command matrix and current findings:

- `examples/runtime-switch-notes/README.md`
- `examples/runtime-switch-notes/DOGFOODING.md`

## NestJS Service Example

`examples/nestjs-service/` is a backend TypeScript dogfooding example for the M59 service primitives.

It keeps NestJS minimal and uses `manifest.tsx` as the source of truth for useful commands: the package is classified as `kind: "service"`, declares `Env(...)` runtime contracts, declares an optional external `Service(...)` requirement, and exposes explicit `dev`, `build`, `start`, `typecheck`, `lint`, and `test` RunTargets.

This is not a built-in NestJS template and does not add orchestration, database startup, OpenAPI generation, deployment behavior, or package-manager script fallback. See:

- `examples/nestjs-service/README.md`
- `examples/nestjs-service/FRICTION.md`
