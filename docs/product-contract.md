# TSPack Product Contract (M32)

## Product thesis

TSPack is a TypeScript lifecycle tool with one core thesis:

**Declare targets. Resolve sources. Enforce boundaries. Lock reality. Pack exactly.**

Its architecture is layered, with package lifecycle guarantees at the center and native development/runtime loops built around the same manifest+lock contract.

## Layered product model

## 1) Core Package Lifecycle (stable)

Scope:
- manifest intent (`manifest.tsx` / `package.manifest.tsx`)
- authoring-only manifest TypeScript surface (`tspack/manifest`) for editor typechecking/autocomplete
- init-generated local declaration delivery for manifest authoring (`.tspack/types/tspack-manifest.d.ts`, `tspack-env.d.ts`)
- target-scoped graph + dependency classification
- resolver + lockfile (`ts-lock.toml`)
- sync/materialization (`node_modules` compatibility artifact)
- checks, pack, and why
- security and capability policy

Primary commands:
- `tspack init`
- `tspack migrate`
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
- package-qualified target selection
- readiness checks
- cwd policy
- explicit env overlays
- doctor integration
- inspect-run startup reuse
- `tspack run [target]`
- `tspack inspect` (experimental)
- run-target inspection (`tspack inspect dev` / `tspack inspect --run dev`)

`tspack inspect` is TSPack's runtime-observation layer. It extracts browser-computed structure for tests, IDEs, and future LLM context while keeping source as text and the runtime as layout truth. It is not visual programming, source mutation, or screenshot analysis.

Primary commands:
- `tspack run`
- `tspack inspect` (**experimental**)

## Command responsibility boundaries

- **migrate** is an onboarding/draft generator. It translates mechanical package.json, package-lock, source import, and script evidence into a reviewable manifest/report, but it is not semantic migration completion.
- **check** validates contract consistency (including suspicious lock graph conditions like duplicate locked versions); no lock mutation.
- **update** resolves dependencies, populates required content-addressed store artifacts, and writes the lockfile.
- **sync** materializes from lock/store state; no lock mutation.
- **pack** creates package archives; no lock mutation.
- **why** explains presence/reachability; no lock mutation.
- **how** explains diagnostic remediation guidance; no mutation.
- **outdated** reports dependency freshness from metadata only; no lock/store/node_modules mutation.
- **test/artifact/bench/doom** execute harness workflows and may write harness outputs, but do not rewrite manifest/lock contract state.
- **run** launches declared runtime targets only; not npm scripts.
- **inspect** performs experimental structural inspection from explicit targets/URLs/CDP/host modes.

## Mutation guarantees

Unless explicitly documented otherwise:
- `migrate` may write only explicit migration outputs such as `manifest.migrated.tsx` and `tspack-migration.md` when `--write` is passed. It must not mutate package.json, source files, package-manager state, or `ts-lock.toml`.
- `check`, `sync`, `pack`, `why`, `how`, `outdated`, `run`, and `inspect` must not mutate `ts-lock.toml`.
- `run` and `inspect --run` must not mutate manifest intent (`manifest.tsx` / `package.manifest.tsx`).
- `update` is the lock mutation command.
- Harness commands (`test`, `artifact`, `bench`, `doom`) may write harness outputs/artifacts, not lock/manifest contract state.

## Claude-fooding Phase 2 validation

Claude-fooding Phase 2 moved the package-manager critical path from prototype behavior to a validated loop:

```text
update -> store -> sync
```

`update` now prepares the content-addressed store required by the lockfile before `sync` materializes compatibility `node_modules`. See `docs/claude-fooding-phase2.md` for the remediation closeout.

## Claude-fooding Phase 3 boundary model

Claude-fooding Phase 3 closed out the boundary/import model with documented physical-file boundaries (`from`), graph-reachable boundaries (`transitiveFrom`), runtime allow/deny controls (`allowOnly` and `denyDeps`), type-level deny controls (`denyTypeDeps`), and explainability through `tspack check --explain <file>`. See `docs/claude-fooding-phase3.md` for the remediation closeout.

## Claude-fooding Phase 4 native xTest model

Claude-fooding Phase 4 closed out the native xTest harness with runtime assertions, static type assertions, snapshots, watch mode, batch execution, static discovery, and local source import closure support. See `docs/claude-fooding-phase4.md` for the remediation closeout.

## Claude-fooding Phase 5 RunTarget model

Claude-fooding Phase 5 closed out the runtime loop with declared RunTargets, readiness checks, doctor integration, package-qualified target selection, cwd policy, explicit env overlays, and inspect-run startup reuse. See `docs/claude-fooding-phase5.md` for the remediation closeout.

## Claude-fooding Phase 6 pack/why model

Claude-fooding Phase 6 closed out pack and why as publish/audit-grade release-gate surfaces: pack produces deterministic archives, verifies package metadata and package path references structurally, and keeps explicit publish policy authoritative; why provides structured dependency explanations and root-to-transitive reverse dependency paths. See `docs/claude-fooding-phase6.md` for the remediation closeout.


## Claude-fooding Phase 7 supply-chain policy

Claude-fooding Phase 7 closed out TSPack's npm lifecycle-script policy as default non-execution plus explicit capability visibility and evidence. `update`, `sync`, and materialization do not execute lifecycle scripts; lifecycle capabilities are recorded in the lockfile, surfaced by `check`, `why`, and `doctor security`, and may be acknowledged only as warning-suppression metadata. Behavior fixtures and reports are evidence links, not execution permission.

Lifecycle execution, if ever added, belongs behind a swappable backend seam. TSPack v1 does not require an OS jail because it does not execute lifecycle scripts by default. See `docs/claude-fooding-phase7.md` for the Phase 7 security/policy closeout.


## Migration onboarding model

`tspack migrate` is an onboarding bridge for existing npm-style projects. It translates mechanical evidence into a reviewable `manifest.migrated.tsx` draft and `tspack-migration.md` report, with stable TODO markers for human or LLM review. It is not a semantic migration-completion oracle: package-lock data is evidence rather than TSPack lock truth, source scans are observed usage rather than architecture truth, script classifications are RunTarget suggestions rather than active command migration, and `--check` proves structural compatibility rather than project correctness. See `docs/migrate.md` and `docs/claude-fooding-migration.md`.

## Security guarantees

- Fetch is not execute.
- Manifest helper typings are authoring support only; they do not execute manifest code.
- Lifecycle scripts are not executed by default.
- Arbitrary package script execution is not part of the contract.
- `node_modules` is a compatibility artifact, not source-of-truth state.
- `run` executes declared `RunTargets`, not `package.json` scripts.
- `inspect` requires explicit input modes; no silent local-port scanning or arbitrary app attach.

## Stability bands

- **Stable core**: check/update/sync/outdated/why/how/pack, lock+graph+materialization semantics.
- **Stable native harness surface**: test/artifact/bench/doom command family.
- **Experimental**: inspect backend/flag surface and backend refinement.

## Backend and plugin seams

- Manifest frontend and native harness bridges are explicit seams between Go CLI orchestration and TypeScript runtime helpers.
- Inspect backends (Playwright/CDP/host paths) are explicit backend seams and remain experimental.
- Runtime launch backends are explicit `RunTarget.runtime` launch adapters, not task-runner plugins.


- `tspack format` and `tspack lint` are delegated lifecycle helpers backed by Biome. They use local-first backend resolution, fall back to a temporary default config when no project Biome config exists, and can participate in CI through the optional read-only `tspack check --format` gate. They are not part of package resolution truth. See `docs/format-lint.md` and `docs/claude-fooding-phase8.md`.

- `tspack doctor` adds non-mutating environment diagnostics. See `docs/doctor.md`.

- `outdated` is a read-only freshness query (current/wanted/latest) and does not mutate lock/store/node_modules.

## Runtime profile portability seam

The workspace runtime profile is the portability seam for JavaScript runtime selection:

```tsx
<Workspace name="demo" runtime="nodejs">
```

`nodejs`, `bun`, and `deno` are the supported profile names; omitted runtime defaults to `nodejs`. The `nodejs` profile is the behavior-preserving compatibility baseline for existing projects: `<Workspace name="demo">` and `<Workspace name="demo" runtime="nodejs">` are equivalent after normalization and should preserve check, update, sync, pack, why, run, test, format/lint, inspect, and doctor behavior. The product contract is to change the runtime profile while keeping the project contract stable. Runtime profile selection does not mean package-manager delegation: TSPack still owns dependency resolution, lockfiles, materialization, checks, packaging, and lifecycle security policy. Runtime profile selection is not npm package metadata and must not leak into generated pack `package.json` output.
