# TSPack Roadmap

This roadmap records known post-`0.1.0` work without turning the first release into a stability promise.

## Post-0.1.0 planned

- Phase 11 inspect/browser deep testing.
- Policy-driven update mutation.
- Targeted policy planning.
- React/single-version coherence policy.
- `check --lint`.
- Per-file format diagnostics.
- Pre-commit hook generation.
- `setup-tspack` hosted smoke after the first public release exists.
- Homebrew, mise, and npm bootstrapper distribution.
- `get.tspack.dev` hosted installer.
- Visionary / VS Code fork work.

## Non-goals for 0.1.0

- Replacing all npm scripts.
- Zero-bootstrap self-hosting.
- Production stability claims.
- New package-manager, resolver, lockfile schema, lifecycle execution, or security model behavior.

## Template roadmap

M54a added the concept-aware inert template engine and built-in static template. M54b/M54d added React app and React library built-ins. M55a completes the internal Template IR normalization path for built-in and local templates while preserving current behavior. Remote templates, registries, and interactive prompts remain later work.

## Template roadmap notes

M54b added the built-in React + Vite app template, and M54d added the React library template. Future work remains concept overlays, UI-library overlays, Next.js, Vue, Tailwind, router-enabled templates, concept validators, remote templates, and package-manager-specialized templates.


## Template overlay future

Older template notes used the word overlays for avoiding hardcoded template explosion; M60 refines that direction toward inert concept fragments as the generative unit rather than overlays as the primary model. Those future capabilities should be reconsidered as concept fragments such as `ui.mui`, `ui.shadcn`, `ui.antd`, `style.tailwind`, and `router.react-router`. A Next.js template remains future work and is intentionally separate from the M54d React library template.


## M60 concept fragment composition track

M60 starts the transition from concepts as template metadata toward concepts as inert generative fragments. The architecture direction is that templates are named concept compositions, concept fragments contribute structured project intent, the engine merges those fragments into a semantic project/template IR, and `TemplatePlan` remains the concrete write boundary. This track should not begin from overlays as the primary model.

M60a is design only and is recorded in `docs/design/concept-fragment-composition.md`. It does not implement a fragment engine, built-in registry, local custom concepts, template migration, overlays, command execution, remote concepts, package installation during init, or package-manager behavior changes.

M60b adds the internal `internal/concepts` fragment model, Go-embedded built-in registry, graph resolver, merge engine, `MergedConceptIR`, and deterministic merge diagnostics. It remains dormant infrastructure: static, React app, and React library templates are not migrated, no public custom concept files are accepted, and generated template output is unchanged.

Planned follow-up phases:

- M60c: migrated static template manifest generation to concept fragments internally while keeping non-manifest file projections and user-facing init behavior unchanged.
- M60d: migrate the React app template manifest generation to concept fragments and revise composition to explicit concept stack semantics with no hidden auto-insertion.
- M60e: migrated the React library template manifest generation to concept fragments internally while keeping non-manifest file projections unchanged.
- M60f: add local custom concept fragments as inert files that lower into the same IR as built-ins.
- M61a: add a local Tailwind React app concept fixture that composes built-in React/Vite/TypeScript concepts with local `my-company.tailwind` without adding a built-in Tailwind concept.
- M61b: added a local MachinaLayout React app concept fixture that composes built-in React/Vite/TypeScript concepts with local `my-company.machina-layout`, contributes the real npm `machinalayout` dependency, and keeps MachinaLayout out of the built-in public registry.
- M61c: composed Tailwind + MachinaLayout in one local React/Vite/TypeScript app fixture with explicit stack order, deterministic file ownership, merged dependency/tool contributions, and check/build-clean smoke coverage.
- M61d: define the built-in concept promotion policy, including maturity levels, promotion checklist, anti-patterns, validation expectations, and promotion workflow.
- Future: M62 concept-authored config projections; M63 backend/service concept renderer support; Shadcn and Storybook local concept fixtures; built-in promotion of Tailwind or MachinaLayout only after the promotion criteria are met; and remote registry exploration much later, if ever.

## CLI help future

M56a keeps help output human-readable and deterministic. Structured JSON help such as `tspack help commands --json` or `tspack help init --json` remains future work rather than a release blocker for v0.1.3.

## Experimental Python module design

M57a records an exploratory Python module/domain design in `docs/design/python-module-experiment.md`. The direction is one manifest frontend with multiple project/package backends: `manifest.tsx` remains the universal typed contract, while TypeScript/npm remains the product focus today and Python/PyPI stays experimental future work.

This is not Python support yet. The roadmap explicitly excludes `manifest.py`, executable Python project configuration, PyPI resolver implementation, `uv` integration, lockfile renaming, packaging publication, and changes to existing TypeScript/npm behavior for this milestone.

Future Python work should be high-locality: Python-specific behavior belongs behind an ecosystem/backend seam, not as scattered `pypi` conditionals across the TypeScript/npm resolver, run, security, lockfile, pack, template, and CLI paths.

## M57b ecosystem/backend seam spike

M57b adds a small ecosystem/backend seam spike in `docs/design/ecosystem-backend-seam.md` plus internal vocabulary for the production TypeScript/npm backend and reserved Python-family/PyPI design terms. The intent is high locality: future ecosystem behavior should live behind backend-owned validation/resolution/materialization/security/runtime seams instead of scattered conditionals. The seam separates package ecosystem, backend, runtime family, runtime implementation, environment/materialization strategy, execution/build mode, version/range scheme, and security/build risk so CPython, PyPy, Cython/native-extension, Triton, Mojo, Numba, and JAX-style systems do not collapse into one flat `python` runtime.

This remains architecture groundwork only. TSPack still has no Python runtime, Python resolver, PyPI source acceptance, uv integration, Python templates, pyproject projection, `manifest.py`, `runtime: "python"`, lockfile rename, or package-manager behavior change. TypeScript/npm remains the only production behavior, and venv-like layouts are future implementation details rather than user-facing project model.

## M57c ecosystem-aware project IR fixture spike

M57c adds a small internal project/package IR fixture spike under `internal/projectir`. The layer can represent current TypeScript/npm package intent and reserved future Python-family/PyPI package intent with separate ecosystem, backend, dependency-intent, range-scheme, runtime-family, runtime-implementation, environment/materialization, and execution-mode axes. The TypeScript/npm fixture remains production-descriptor backed; the Python-family/PyPI fixture remains reserved/internal data only.

Python remains experimental design only. TSPack still has no Python CLI, runtime execution, resolver, sync/materializer behavior, lockfile schema migration, PyPI source acceptance, uv integration, pyproject projection, templates, or package-manager behavior change.

## M59a RunTarget environment declarations

M59a adds manifest-declared RunTarget environment contracts with required/default/secret metadata, check-time declaration validation, run-time missing-env validation before process start, default injection, and safe redaction in list/JSON output.

Future M59 work may add strict environment allowlisting/scrubbing, runtime env access tracing, service dependency declarations, a service package kind, and NestJS migration/template support. M59a intentionally does not load `.env` files or orchestrate services.


M59b adds RunTarget service requirements with TCP and HTTP preflight checks before `tspack run` exec. `tspack check` validates only declaration shape; future work may add runtime doctor/preflight-only commands, service orchestration, Docker Compose integration, NestJS migration, strict env scrubbing, and runtime access tracing.

## M59c semantic service package kind

M59c adds `kind: "service"` as a package classification for deployable
backend/runtime units. The kind is accepted by manifest authoring types,
validated and preserved in Go IR, displayed in RunTarget listing output, and
kept compatible with RunTarget `Env(...)` contracts and `Service(...)`
requirements.

M59c intentionally does not change dependency resolution, sync, package-manager
behavior, or RunTarget execution. `tspack pack` reports service packages as
unsupported rather than inventing service deployment artifacts.

Future service work may add deeper package-kind service validation, service
deployment artifacts, Docker Compose/service orchestration, inter-service
dependency checks, NestJS migration/templates, and OpenAPI/artifact targets.

## M59d NestJS service dogfooding example

M59d adds `examples/nestjs-service/`, a minimal generic NestJS backend service example that combines the existing backend primitives without adding new product behavior. The example uses `kind: "service"`, manifest-declared `Env(...)` contracts, an optional external `Service(...)` requirement, explicit RunTargets, and a short friction report.

M59d intentionally remains an example milestone, not a template or migration milestone. It does not add Docker Compose orchestration, database startup, OpenAPI generation, deployment targets, service package pack artifacts, npm script fallback, or package-manager behavior changes. Future NestJS template/migration work remains separate and should use the dogfooding findings as input.

## M59e backend RunTarget friction fixes

M59e completes the immediate backend RunTarget friction fixes from the NestJS service example: HTTP readiness URLs support `${PORT}`-style env interpolation, `tspack run --preflight-only` validates env and external `Service(...)` dependencies without starting the command, and docs/examples clarify that `Service(...)` is an external dependency preflight while `ready` is the target's own post-start health signal.

Future backend work remains deliberately out of scope: `.env` loading, strict env scrubbing, runtime env tracing, Docker Compose or service orchestration, OpenAPI/artifact targets, service package artifacts, and composable template concepts.

## M60f local custom concept fragment MVP

M60f adds local-only custom concept fragments for expert-authored local templates. Templates declare `[[localConcepts]]` entries, local TOML concept files are validated as inert data, fragments merge with the built-in concept registry under explicit stack semantics, and file contributions can add rendered/copy files through the normal TemplatePlan safety boundary.

Unsupported manifest contributions from local concepts now fail loudly for local templates that do not have concept manifest rendering, so dependency/tool/run-target intent cannot silently disappear. M60g adds opt-in generic concept-rendered manifest support for local templates through `[generation] manifest = "concept"`, allowing local TOML concepts to contribute supported manifest rows deterministically. M60g.1 hardens the local concept-rendered fixture into a check-clean React/Vite/TypeScript project that exercises init, update, sync, check, format-check, policy dry-run, build, local dependency contribution, local run target contribution, and local file contribution. M61a adds Tailwind as a local fixture concept first, not as a built-in. M61b adds MachinaLayout as the second practical UI-stack local concept fixture using the real npm `machinalayout` package and documented React adapter imports. M61c composes Tailwind + MachinaLayout in one local React/Vite/TypeScript app fixture without promoting either concept to the built-in registry. M61d defines the built-in concept promotion policy so local concepts remain the incubation path and built-ins remain curated stable primitives. Future work may add M62 concept-authored config projections, M63 backend/service concept renderer support, Shadcn and Storybook local concept fixtures, built-in promotion of Tailwind or MachinaLayout only after the criteria are met, and remote registry exploration much later, if ever. M60f/M60g do not add remote concepts, script execution, package installation during init, template inheritance, a public marketplace, or package-manager behavior changes.
