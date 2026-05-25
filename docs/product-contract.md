# TSPack Product Contract (M24)

## Product thesis

TSPack is a TypeScript lifecycle tool with one core thesis:

**Declare targets. Resolve sources. Enforce boundaries. Lock reality. Pack exactly.**

Its architecture is layered, with package lifecycle guarantees at the center and native development/runtime loops built around the same manifest+lock contract.

## Layered product model

## 1) Core Package Lifecycle (stable)

Scope:
- manifest intent (`manifest.tsx` / `package.manifest.tsx`)
- authoring-only manifest TypeScript surface (`tspack/manifest`) for editor typechecking/autocomplete
- target-scoped graph + dependency classification
- resolver + lockfile (`ts-lock.toml`)
- sync/materialization (`node_modules` compatibility artifact)
- checks, pack, and why
- security and capability policy

Primary commands:
- `tspack init`
- `tspack check`
- `tspack update`
- `tspack sync`
- `tspack why`
- `tspack how`
- `tspack pack`

## 2) Native Development Harness (stable)

Scope:
- native xTest (`*.xtest.tsx`, `Suite`, `Fact`, `Theory`, `Case`)
- invariant fixtures (`*.valid.tsx`, `*.invalid.tsx`)
- project fixtures/sandboxes
- test artifacts (`tspack artifact`)
- benchmarks (`*.benchmark.tsx`, `tspack bench`)
- prophecy/doom tests (`*.prophecy.tsx`, `tspack doom`)
- harness assertions/helpers (`assert.*`, `expect(...).because(...)`, `expect.noError`, `assert.LGTM`, command helpers)

Primary commands:
- `tspack test`
- `tspack artifact`
- `tspack bench`
- `tspack doom`

## 3) Runtime / Inspection Loop

Scope:
- declared `RunTargets`
- `tspack run [target]`
- `tspack inspect` (experimental)
- run-target inspection (`tspack inspect dev` / `tspack inspect --run dev`)

Primary commands:
- `tspack run`
- `tspack inspect` (**experimental**)

## Command responsibility boundaries

- **check** validates contract consistency; no lock mutation.
- **update** resolves and writes lockfile.
- **sync** materializes from lock; no lock mutation.
- **pack** creates package archives; no lock mutation.
- **why** explains presence/reachability; no lock mutation.
- **test/artifact/bench/doom** execute harness workflows and may write harness outputs, but do not rewrite manifest/lock contract state.
- **run** launches declared runtime targets only; not npm scripts.
- **inspect** performs experimental structural inspection from explicit targets/URLs/CDP/host modes.

## Mutation guarantees

Unless explicitly documented otherwise:
- `check`, `sync`, `pack`, `why`, `run`, and `inspect` must not mutate `ts-lock.toml`.
- `run` and `inspect --run` must not mutate manifest intent (`manifest.tsx` / `package.manifest.tsx`).
- `update` is the lock mutation command.
- Harness commands (`test`, `artifact`, `bench`, `doom`) may write harness outputs/artifacts, not lock/manifest contract state.

## Security guarantees

- Fetch is not execute.
- Manifest helper typings are authoring support only; they do not execute manifest code.
- Lifecycle scripts are not executed by default.
- Arbitrary package script execution is not part of the contract.
- `node_modules` is a compatibility artifact, not source-of-truth state.
- `run` executes declared `RunTargets`, not `package.json` scripts.
- `inspect` requires explicit input modes; no silent local-port scanning or arbitrary app attach.

## Stability bands

- **Stable core**: check/update/sync/why/pack, lock+graph+materialization semantics.
- **Stable native harness surface**: test/artifact/bench/doom command family.
- **Experimental**: inspect backend/flag surface and backend refinement.

## Backend and plugin seams

- Manifest frontend and native harness bridges are explicit seams between Go CLI orchestration and TypeScript runtime helpers.
- Inspect backends (Playwright/CDP/host paths) are explicit backend seams and remain experimental.
- Runtime launch backends are explicit `RunTarget.runtime` launch adapters, not task-runner plugins.


- `tspack format` and `tspack lint` are Biome-backed lifecycle UX commands. See `docs/format-lint.md`.

- `tspack doctor` adds non-mutating environment diagnostics. See `docs/doctor.md`.
