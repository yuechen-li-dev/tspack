# TSPack architecture

This map is the practical answer to “where should this code live?” The filesystem
is intended to expose the dependency direction without requiring knowledge of
the project history.

## Repository map

| Path | Responsibility | Classification |
| --- | --- | --- |
| `cmd/tspack` | Process bootstrap only; passes process arguments to `internal/cli` | CLI/presentation |
| `internal/cli` | Command registry, parsing, output, report DTOs, workspace loading, and application orchestration | application + CLI/presentation |
| `internal/project` | Update, sync, check, pack, why, outdated, policy, progress, and performance orchestration | application/orchestration |
| `internal/manifest`, `manifesttypes` | Normalized manifest IR and the small public declaration type vocabulary | core domain |
| `internal/graph`, `projectir`, `ecosystem` | Semantic workspace/dependency models and ecosystem-neutral project evidence | core domain |
| `internal/resolver`, `lockfile` | Resolution and deterministic lockfile truth | core domain + infrastructure boundary |
| `internal/store`, `materialize`, `pack` | Artifact persistence, verified materialization, and package production | infrastructure |
| `internal/check`, `boundary`, `typesurface`, `importscan` | Static project validation and source-boundary analysis | cross-cutting validation |
| `internal/audit`, `capability`, `installscript`, `securityevidence` | Vulnerability, lifecycle capability, and security evidence services | cross-cutting security/audit |
| `internal/diag`, `how`, `why` | Diagnostic primitives and owned explanatory models | cross-cutting diagnostics |
| `internal/integrations/browser` | Browser package transformation, runtime assets, host materialization, and adapter tests | adapter/integration |
| `internal/integrations/skyrim` | Skyrim host profiles, runtime/INI/process handling, and save fixtures | adapter/integration + dogfood |
| `internal/bridge`, `embeddedbridges`, `nodecmd`, `testcmd` | JavaScript bridge discovery/execution and native xTest process integration | adapter/infrastructure |
| `internal/npmbridge`, `npmobserve` | Explicit npm delegation and read-only npm adoption evidence | legacy/compatibility adapter |
| `internal/adoption`, `compat` | Incremental adoption and compatibility-file planning | legacy/compatibility application |
| `internal/concepts`, `templates` | Inert concepts, template resolution, and project scaffolding | application/domain support |
| `internal/pathutil`, `perf`, `version` | Narrow shared infrastructure services | infrastructure |
| `manifest-frontend` | TSX authoring API, manifest evaluation bridge, native xTest and inspect bridges | adapter/frontend boundary |
| `extensions/tspack-vscode` | VS Code presentation integration | adapter/integration |
| `examples` | Runnable or explanatory product examples | experimental/dogfood |
| `fixtures` | Valid, invalid, compatibility, and browser contract fixtures | fixture/test-only |
| `scripts` | Release, install, and self-host smoke entrypoints | infrastructure/operations |
| `tools` | Maintainer generators and proof scripts | generated-surface tooling + experiments |
| `schemas` | Machine-readable contract schemas | core/generated contract |
| `docs` | User, design, release, review, and developer documentation | documentation |
| `dist`, `.tspack`, generated declarations | Build output and materialized/generated surfaces; never hand-author derived copies | generated |

## Dependency spine

```text
manifest.tsx / package.manifest.tsx
        |
        v
manifest-frontend  ->  internal/manifest IR
        |                    |
        |                    v
        |             graph / project model
        |                    |
        |                    v
        |                 resolver
        |                    |
        |                    v
        |                 lockfile
        |                    |
        |                    v
        |                  store
        |                    |
        |                    v
        |               materialize / pack
        |                    |
        +------------> internal/project
                              |
                              v
                         internal/cli
                              |
                              v
                          cmd/tspack
```

Dependencies point down toward domain and infrastructure capabilities. Core
packages must not import `internal/cli` or `internal/integrations`. Integrations
may consume stable domain contracts but must not import CLI presentation.
Architecture tests enforce these two rules and keep `cmd/tspack` bootstrap-only.

## Command and application layer

`internal/cli/app.go` owns top-level help/version behavior. `registry.go` is the
single command-registration point. Feature-named `*_command.go` files own flag
parsing and rendering. `reports.go` owns the lifecycle command JSON schemas;
specialized commands such as audit and adoption keep their schemas beside their
implementation. `workspace.go` owns the cheap resolved workspace context and the
explicit, potentially expensive manifest-loading stage.

`internal/project` owns package-manager semantics. CLI code may translate flags
into `project.Options` and render `project.Result`; it must not reimplement
resolution, lockfile, store, materialization, or policy behavior.

## Manifest frontend boundary

The TypeScript package in `manifest-frontend` owns TSX evaluation and authoring
types. Go discovers and invokes its built bridge through `internal/bridge` and
`internal/nodecmd`, then validates the normalized result in `internal/manifest`.
Generated declaration copies have one source in the frontend/build pipeline and
are checked with `tspack compat diff`.

## Cross-cutting services

- Diagnostics originate as `internal/diag.Diagnostic`; feature reports translate
  them only at their presentation boundary.
- Boundary/type checks live in `internal/check`, `boundary`, and `typesurface`.
- Lifecycle capabilities and evidence live in `capability`, `installscript`, and
  `securityevidence`; OSV querying lives in `audit`.
- Performance measurement lives in `perf`, while orchestration decides when to
  record it.

## Integrations and experiments

Browser and Skyrim code are deliberately below `internal/integrations`. Their
runtime files, platform-specific process helpers, DTOs, and focused tests stay
with the integration. `internal/cli/*_integration.go` files are thin adapters.
New ecosystem or deployment-specific machinery follows the same boundary; it
does not enter `internal/project` unless it becomes a generic lifecycle concept.

Experiments belong in `examples`, purpose-named fixture directories, or a
purpose-named integration package. Do not use production packages as fixture
storage.

## Testing and generated files

Unit tests live beside their owner. CLI subprocess tests build one shared binary
per package run; do not add per-test `go run ./cmd/tspack` calls. Slow/live tests
must state their external requirement and remain distinguishable from the fast
unit path. Browser and Skyrim tests live with their integration packages.

Generated files are changed through their owner generator or compatibility
writer. `compat diff`, generator tests, frontend type checks, and embedded-surface
tests are the drift gates.

## Where a new feature goes

1. Put new semantic vocabulary in `manifest`, `graph`, or another durable domain
   package only when it is genuinely shared.
2. Put resolution/update/sync behavior in `resolver` or `project`, not in CLI.
3. Put a command's registration in `registry.go`, parsing/rendering in a
   feature-named CLI file, and reusable behavior below the CLI boundary.
4. Put OS/browser/ecosystem/deployment-specific behavior in
   `internal/integrations/<name>` behind a narrow adapter.
5. Put fixtures under `fixtures` or package `testdata`; keep generated output out
   of production source directories unless it is embedded runtime data.
6. Avoid new top-level `internal` packages unless the responsibility is durable,
   named, and has a clear dependency direction. Never create a generic `utils`.
