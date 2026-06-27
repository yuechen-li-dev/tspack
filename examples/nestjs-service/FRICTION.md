# NestJS Service Example Friction Notes

## manifest API friction

The manifest can express a generic NestJS service with existing primitives. The main readability cost is declaring several tool and runtime dependencies explicitly, especially scoped packages that need stable dependency keys.

## RunTarget env friction

`Env(...)` works well for defaulted `PORT` and `NODE_ENV`. A realistic `DATABASE_URL` should not be required by the golden `dev` target unless the app actually connects to a database, because required env validation correctly blocks process start.

## RunTarget service requirement friction

`Service(...)` is appropriate for external dependencies, not for the service's own `/health` endpoint. The example uses an optional TCP Postgres preflight and RunTarget HTTP readiness for self-health. This distinction is important enough to document prominently.

## NestJS/TypeScript config friction

NestJS needs `experimentalDecorators` and `emitDecoratorMetadata`. A CommonJS build keeps the example simple and avoids ESM loader tradeoffs. `tsx watch src/main.ts` is lighter than adding `@nestjs/cli`, but it means the example owns its TypeScript compiler settings directly.

## package-manager/update/sync friction

The example depends on registry packages, so a cold `tspack update` requires network access. This should remain a manual smoke command unless the repository's test harness has an offline fixture for these packages.

## check/format friction

Biome can format and lint the small NestJS source files without special NestJS rules. Generated directories such as `dist`, `.tspack`, and `node_modules` are ignored.

## run lifecycle friction

Readiness URLs are static. If a user overrides `PORT` with `--env PORT=4000`, the manifest readiness URL still points at port 3000. Backend examples need to call this out until TSPack has an explicit dynamic readiness-port story.

## future feature suggestion

- Service packages could recommend, but not require, conventional `dev` and `start` RunTargets.
- RunTarget docs could add a backend example that contrasts readiness with external `Service(...)` preflights.
- Explicit `.env` loading may be useful for services, but should remain opt-in and visible in the manifest.
- `tspack run --preflight-only` would help validate env and service requirements without starting long-running processes.
- A generic `node-service` example or template should probably precede any NestJS-specific template.
- OpenAPI generation, Docker Compose orchestration, service startup, deployment targets, and service pack artifacts remain future work outside this milestone.
