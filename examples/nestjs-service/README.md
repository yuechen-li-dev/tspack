# NestJS Service Example

This example dogfoods the TSPack backend TypeScript primitives with a small NestJS service. It is an example, not a productized template.

## What it demonstrates

- `kind: "service"` as semantic package classification.
- `Env(...)` contracts for `PORT`, `NODE_ENV`, and an optional secret `DATABASE_URL`.
- `Service(...)` requirements with an optional local Postgres TCP preflight.
- Explicit `RunTargets` for development, build, start, typecheck, lint, and test.
- HTTP readiness for the service's own `/health` endpoint, with `${PORT}` interpolation so `--env PORT=...` changes the checked URL.

`tspack run` is not `npm run`: `package.json` intentionally has no scripts. The manifest is the source of truth for useful commands.

## RunTargets

| Target | Purpose |
| --- | --- |
| `dev` | Starts `tsx watch src/main.ts` and waits for `GET /health`. |
| `build` | Compiles `src/**/*.ts` to `dist/` with `tsc`. |
| `start` | Runs `node dist/main.js` and waits for `GET /health`. |
| `typecheck` | Runs `tsc --noEmit` over source and tests. |
| `lint` | Runs `biome check .`. |
| `test` | Runs `vitest run`. |

## Smoke commands

From this directory, using the local TSPack binary:

```sh
tspack update
tspack sync
tspack check
tspack check --format
tspack run --list
tspack run --list --json
tspack run typecheck
tspack run build
tspack run dev --once --ready-timeout 20
tspack run dev --env PORT=4000 --once --ready-timeout 20
tspack run dev --preflight-only
tspack run start --env PORT=4001 --once --ready-timeout 20
tspack run test
```

## Notes

The app exposes:

- `GET /` returning a small JSON message.
- `GET /health` returning `{ "ok": true }`.

The optional Postgres service requirement is deliberately not used by the app. It shows how a backend service can declare future external-service expectations without blocking the default development path.

## Env, Service, and readiness

`Env("PORT", { default: "3000" })` feeds the process environment and the manifest readiness URL `http://127.0.0.1:${PORT}/health`. For example, `tspack run dev --env PORT=4000 --once --ready-timeout 20` starts NestJS on port 4000 and checks `http://127.0.0.1:4000/health`.

`Service(...)` requirements are external dependency preflights checked before the target starts. The RunTarget `ready` URL is the target's own health signal checked after the process starts. `tspack run dev --preflight-only` validates env and external `Service(...)` requirements without starting NestJS and without checking the self-readiness URL.
