# M68b lifecycle/application decomposition

## Before

After M68a, command registration and executable bootstrap had clear owners, but
`internal/cli/lifecycle_commands.go` still combined six lifecycle parsers,
project invocation, policy planning, diagnostics presentation, text rendering,
and JSON conversion. Its single `runCommand` selected behavior using the command
name throughout one control flow. `internal/project/project.go` likewise held
check, update, sync, pack, why, manifest loading, store population, and lifecycle
security diagnostics behind one union `Result`.

That shape made the effective application API “call a broad function, then
inspect which pointer in `project.Result` is populated.” It also obscured which
operations could write resolved state.

## Application model

Lifecycle commands now follow this flow:

```text
dedicated CLI parser
    -> explicit project request
    -> typed lifecycle operation result
    -> command text or JSON renderer
    -> diagnostic-to-exit mapping
```

`internal/cli/lifecycle_command_paths.go` owns only cheap path selection for the
workspace root, manifest, lockfile, and store. Manifest frontend execution,
graph construction, lock loading, resolution, store access, and materialization
remain explicit work inside `internal/project` operations.

| Operation | Inputs | Required state | Mutation authority | Semantic output | CLI responsibility |
| --- | --- | --- | --- | --- | --- |
| Check | project paths, optional explain file | manifest and graph; lock when present | none | diagnostics or explanation | flags, format phase, text/JSON, exit |
| Update | project paths, optional query, dry-run | manifest, graph, existing lock, resolver | lock and store unless dry-run | diagnostics, lock diff, target selection | progress choice, text/JSON, exit |
| Sync | project paths, clean/force | manifest, graph, resolved lock, store | hydrate store and materialize only | diagnostics | progress choice, diagnostics, exit |
| Pack | project paths and pack options | manifest, graph, consistent lock when present | artifact output only | diagnostics, artifact/preview records | text rendering and exit |
| Why | project paths and explanation query | manifest, graph, optional/required lock by mode | none | structured explanation | npm-observation routing, text/JSON, exit |
| Outdated | project paths, grouping view | manifest, graph, lock, registry metadata | none | typed version candidates and summary | grouping and text/JSON |
| Policy plan | project paths | outdated candidates, policy, security evidence | none | typed candidates plus security gates | policy-plan schema and text/JSON |

The compatibility `project.Result` remains for existing internal callers and
rendering adapters. New lifecycle command execution uses `CheckRequest`,
`UpdateRequest`, `SyncRequest`, `PackRequest`, `WhyRequest`, `OutdatedRequest`,
and `PolicyPlanRequest` with their command-specific results.

## After

Production ownership is now feature-named:

- `check_command.go`, `update_command.go`, `sync_command.go`,
  `pack_command.go`, `why_command.go`, and `outdated_command.go` own command
  parsing and presentation.
- `diagnostic_rendering.go` owns durable diagnostic/exit presentation.
- `lifecycle_command_paths.go` owns shared cheap project path flags.
- `internal/project/*_operation.go` owns each lifecycle implementation.
- `lifecycle_operations.go` is the typed, presentation-free application facade.
- `outdated.go`, `update_policy.go`, and `policy_security.go` remain distinct:
  observation, policy classification, and security admission are not collapsed
  into update mutation.

`project.go` is now the small compatibility vocabulary/default-options owner;
update resolution/store population, lifecycle loading/support, and each other
operation have named owners.

## Check phases

Check preserves its existing diagnostic semantics and deterministic sort order.
Its application owner shows the expensive stages in order: manifest/frontend and
graph loading, security-evidence validation, package/source checks, optional lock
loading, graph consistency, version conflicts, and lifecycle capability policy.
CLI-only format validation remains visibly separate because it is an optional
command presentation/integration phase, not part of the project check API.

## Behavioral invariants

- `update` may resolve dependencies, populate the store, and write
  `ts-lock.toml`.
- update dry-run and policy dry-run do not write `ts-lock.toml`.
- `sync` hydrates missing store artifacts and materializes strictly from lockfile
  truth; it never writes `ts-lock.toml`.
- `check`, `pack`, `why`, and `outdated` never write `ts-lock.toml`.
- `pack` may create requested archives but does not resolve dependencies.
- `why --reverse` requires lock truth; ordinary why retains its existing
  missing-lock behavior.

Existing project tests continue to prove check, sync, pack, why, update dry-run,
and policy dry-run mutation guarantees. Update/sync integration tests continue to
prove lock generation, store population, hydration, and deterministic output.

## Test lanes

`internal/cli/clitest` is the deliberate process harness. It owns subprocess
capture, JSON decoding, temporary workspace creation, manifest writing, exit
assertions, and tracked-file snapshots. It stays deliberately small rather than
becoming a command DSL.

CLI tests are split into command and lane owners. Parser, renderer, report, and
application tests run directly where practical. Process tests remain for actual
dispatch, stdio, exit status, environment inheritance, runtime processes, and
tool integration. The shared test binary remains built once in `TestMain`.

## Deferred ideas

- Removing the compatibility union `project.Result` entirely requires migrating
  non-lifecycle internal callers and is suitable for M68c.
- Making every CLI handler return an exit value through injected IO would allow
  more renderer/parser tests to avoid processes, but is a CLI runtime redesign
  rather than a safe partial conversion.
- Resolver/project graph vocabulary and lockfile IR simplification remain
  separate milestones because they risk selection, graph, format, or ordering
  semantics.
- A transaction abstraction for lock/store/materialization writes needs focused
  failure-injection design.
- `migrate`, `run`, and `doctor` were not split: no new lifecycle service removed
  duplicated semantics from them during this pass.
- The existing Vitest advisories, including critical `GHSA-5xrq-8626-4rwp`,
  require a dependency-remediation milestone and were not changed here.
