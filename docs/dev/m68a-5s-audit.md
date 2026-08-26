# M68a 5S architecture audit

## Baseline

- Branch: `main`
- HEAD: `359b073` (`Synchronize concurrent artifact writes`)
- Reported release: `tspack v0.1.8` (`commit unknown`, `built unknown` for `go run`)
- Working tree: clean before M68a
- Baseline Go tests: passed; `cmd/tspack` took 216.016s and `internal/check`
  took 46.485s
- Frontend build/typecheck/tests: passed (201 passed, 2 skipped)
- VS Code compile/tests: passed (35 passed)
- Baseline drift: `compat diff` failed only for generated
  `.tspack/types/tspack-manifest.d.ts`; the xTest declaration and VS Code files
  were current

## 5S inventory

### Sort

Core domain is `manifest`, `manifesttypes`, `graph`, `projectir`, `ecosystem`,
`resolver`, and `lockfile`. Application orchestration is `project`, `cli`,
`adoption`, `compat`, `concepts`, and `templates`. Infrastructure is `store`,
`materialize`, `pack`, `bridge`, `embeddedbridges`, `nodecmd`, `pathutil`, `perf`,
and `version`. Validation/security services are `check`, `boundary`,
`typesurface`, `importscan`, `diag`, `how`, `why`, `audit`, `capability`,
`installscript`, and `securityevidence`. npm compatibility adapters are
`npmbridge` and `npmobserve`. Browser and Skyrim are specialized integrations.
`examples` are dogfood; `fixtures` and package `testdata` are test-only; `dist`,
`.tspack`, and declaration/build outputs are generated.

Baseline smells were a 2,871-line executable entrypoint, a 5,400+ line
`main_test.go`, 26 manual dispatch branches, command report DTO families in
`main.go`, browser and Skyrim implementation in the executable package, stale
placeholder package docs, repeated manifest-loading setup, and a test harness
that rebuilt or `go run`-linked the CLI around 150 times.

### Set in Order

The adopted spine is manifest frontend -> normalized manifest -> graph/project
model -> resolver -> lockfile -> store -> materialization/pack -> project
orchestration -> CLI. Diagnostics, security, audit, check, and performance are
cross-cutting services. Browser and Skyrim point inward through domain contracts;
core does not point outward to integrations or CLI.

### Shine

- Removed four stale M0 placeholder package comments and replaced them with
  ownership-oriented documentation.
- Removed the executable-package dumping ground by relocating its production
  code and tests to their owners.
- Removed repeated per-test CLI builds and direct `go run` subprocesses.
- Avoided full-repository source walks when a package has no transitive boundary
  rules.
- Renamed the kitchen-sink `main_test.go` to describe lifecycle command coverage.

### Standardize

- One declarative command registry now owns dispatch.
- `cmd/tspack` has one bootstrap file.
- CLI report DTOs have a named `reports.go` owner; specialized schemas remain
  beside specialized commands/integrations.
- `Workspace` in `internal/cli/workspace.go` makes root resolution cheap and
  manifest loading explicit.
- Specialized integrations use thin CLI adapters and may not import CLI.
- CLI subprocess tests use one shared binary per test-package run.

### Sustain

Architecture tests enforce bootstrap-only `cmd/tspack`, prohibit core imports of
CLI/integrations, prohibit integration imports of CLI, and pin the explicit
command registry. `docs/dev/architecture.md` and the root placement rules explain
where new work belongs.

## Changes completed in M68a

- Moved command implementation and tests from `cmd/tspack` to `internal/cli`.
- Reduced the executable to process bootstrap and exported `cli.Main(args)`.
- Replaced manual top-level dispatch with `registry.go`.
- Split dispatch, lifecycle report DTOs, workspace loading, and lifecycle command
  implementation into named files.
- Moved browser materialization, embedded runtime, contracts, and focused tests
  to `internal/integrations/browser`.
- Moved Skyrim host/runtime/INI/process/save-fixture implementation and tests to
  `internal/integrations/skyrim`, with injected manifest loading.
- Added dependency-direction guardrails.
- Optimized the CLI test harness and boundary-check no-rule path.

## Structural metrics

| Metric | Before | After |
| --- | ---: | ---: |
| `cmd/tspack/main.go` | 2,871 lines | 8 lines |
| Go files in `cmd/tspack` | 54 production/test files plus runtime assets | 1 bootstrap file |
| top-level manual dispatch branches | 26 | 1 registry lookup with 26 explicit registrations |
| top-level `internal` directories | 36 | 37; the single addition is the durable `integrations` boundary |
| largest CLI entry/command file | `main.go`, 2,871 lines | `lifecycle_commands.go`, 2,534 lines |
| specialized production code in CLI/executable package | browser + Skyrim, about 2,900 lines | thin adapters; implementations under `internal/integrations` |
| CLI package uncached test time | 216.016s | 72.8s after shared-binary change (first measurement) |
| `internal/check` uncached test time | 46.485s | 1.13s for `check` + `boundary` after lazy transitive scan |

Metrics are evidence, not targets. The remaining large lifecycle, migration, run,
doctor, project, and manifest files are called out below rather than split without
a stable semantic boundary.

## Remaining architectural debt

### P0

- `internal/cli/lifecycle_commands.go` still combines check/update/sync/pack/why/
  outdated rendering. Split by command schema and presentation behavior in a
  follow-up without moving project semantics upward.
- The CLI test suite is faster but its lifecycle file remains too broad. Split it
  by command after shared fixture/build helpers are extracted into a deliberate
  test harness.

### P1

- `internal/cli/migrate_command.go`, `run_command.go`, and `doctor_command.go` are
  still large. Their internal phases should become application services only
  where reuse and stable inputs/outputs justify the boundary.
- `internal/project/project.go` is a broad orchestrator. Separate update, sync,
  check, why, and pack facades while preserving one project lifecycle API.
- `manifest` IR and `graph` contain overlapping package/dependency vocabulary;
  ownership is understandable but conversion boundaries deserve a focused pass.

### P2

- Small top-level packages (`capability`, `securityevidence`, `how`, `projectir`,
  `ecosystem`) should be observed for durable ownership; merging them now would
  conflate distinct contracts for little navigation benefit.
- Historical design/release docs are numerous and occasionally describe old
  placement. They are records, not normative architecture; future sweeps should
  add status headers rather than rewrite history.

## Separate-pass ideas

| Idea | Problem | Proposed direction | Why separate | Expected benefit | Risk |
| --- | --- | --- | --- | --- | --- |
| Resolver/project model redesign | Manifest, graph, resolver, and project carry related representations | Define explicit intent, semantic graph, resolution request, and resolved graph boundaries | Changes touch resolution semantics and require dedicated compatibility proofs | Clearer dependency truth and fewer conversions | High: accidental resolution change |
| Lockfile IR simplification | Lock DTO and runtime consumers expose storage details broadly | Introduce a small resolved-graph API backed by lockfile serialization | Lock format must remain stable and migrations need focused review | Easier lock consumers and stronger invariants | High: format or ordering drift |
| Filesystem transaction layer | Init, migrate, compat, materialize, browser, and Skyrim each implement safe writes | Design purpose-specific staged write/commit primitives | Cross-cutting atomicity deserves fault-injection tests | Consistent rollback and Windows behavior | Medium/high: subtle durability regressions |
| General adapter/plugin architecture | Integrations now have clear homes but registration is compiled-in | Define lifecycle extension points only after more adapters establish common needs | A premature plugin API would freeze Skyrim/browser accidents | Smaller integrations and optional distribution | High: unstable public abstraction |
| Full CLI framework replacement | Parsing and exits remain hand-written inside handlers | Evaluate a framework or an error-returning command runtime with injected IO | Rewriting all commands now would dominate M68a and perturb output/exit contracts | Unit-testable commands and consistent flags | Medium/high: compatibility drift |
| Manifest frontend/compiler redesign | TSX bridge startup and generated declarations are coupled to Node artifacts | Specify a versioned frontend protocol and canonical declaration generator | Public manifest semantics and tooling compatibility need their own milestone | Faster loading and clearer generated ownership | High |
| Test harness decomposition | Shared binary fixed linking cost, but fixtures and subprocess helpers remain scattered | Create feature-scoped harness packages and explicit unit/integration test lanes | Mechanical splitting without ownership would only move clutter | Faster selection and clearer failure locality | Medium |
| Store ownership/integrity model | Store, resolver, project, and materializer share integrity assumptions | Specify artifact state transitions and verification ownership | Requires concurrency/failure analysis beyond a structural refactor | Stronger repair and transaction behavior | High |
| Public API rework | Current internal services grew around CLI needs | Define supported embeddable APIs only from real external consumers | No stable consumer contract is established yet | Cleaner automation/embed story | High: premature commitment |

## Validation

- `go test ./... -timeout 300s`: passed. The CLI package reported 72.149s and
  `internal/check` 0.591s in the final matrix.
- Manifest frontend build and manifest API typecheck: passed.
- Manifest frontend tests: 201 passed, 2 environment-dependent inspect tests
  skipped as expected.
- VS Code extension compile and tests: passed (35 tests).
- `tspack compat diff --root .`: passed after regenerating the baseline-stale,
  ignored manifest declaration through `compat write`.
- Version, help, check, and why smokes: passed.
- Audit smoke executed successfully and intentionally returned nonzero because it
  found six advisories in the existing lockfile, including critical
  `GHSA-5xrq-8626-4rwp` for Vitest 2.1.9. Dependency remediation is outside this
  refactor and no policy/version mutation was made.
- The POSIX self-host script reached its command matrix after LF normalization,
  but the installed Windows `bash.exe` environment does not expose Node on PATH.
  The equivalent PowerShell matrix passed all five commands and verified that
  tracked state was unchanged.
- `git diff --check`: passed.

The baseline compatibility drift and Windows CRLF-only format failure were
repaired without semantic changes. `.gitattributes` now keeps shell entrypoints
and the root manifest LF-stable across Windows checkouts.

## Outcome

**Outcome A — Success.** The required architecture and behavior validation pass.
The audit command's nonzero finding is a product result against existing locked
dependencies, not a refactor/test failure; its remediation is explicitly
deferred rather than smuggled into M68a.
