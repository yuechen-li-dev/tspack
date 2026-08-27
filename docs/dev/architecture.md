# TSPack architecture

This map is the practical answer to “where should this code live?” The filesystem
is intended to expose the dependency direction without requiring knowledge of
the project history.

## Repository map

| Path | Responsibility | Classification |
| --- | --- | --- |
| `cmd/tspack` | Process bootstrap only; passes process arguments to `internal/cli` | CLI/presentation |
| `internal/cli` | Command registry, command-specific parsing, text/JSON renderers, exit mapping, and cheap workspace path loading | CLI/presentation |
| `internal/project` | Typed lifecycle application operations for add, update, sync, check, pack, why, outdated, and policy planning | application/orchestration |
| `internal/authoring` | Dependency authoring declarations, provenance, ordered tape, effective projection, and pure edit semantics | core domain |
| `internal/manifestedit` | Syntax-qualified owned dependency islands, deterministic dependency rendering, and source-edit planning over M69a semantic edits | core domain + manifest frontend boundary |
| `internal/manifest`, `manifesttypes` | Normalized manifest IR and the small public declaration type vocabulary | core domain |
| `internal/graph`, `projectir`, `ecosystem` | Semantic workspace/dependency models and ecosystem-neutral project evidence | core domain |
| `internal/requirements` | SSA-like dependency-intent IR, shared-slot precedence, shadowing, and compatibility classification | core domain |
| `internal/resolver`, `lockfile` | Version resolution and deterministic selected/requirement lock truth | core domain + infrastructure boundary |
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
manifest-frontend  ->  dependency authoring IR
        |                    |
        |                    v
        |              ordered dependency tape
        |                    |
        |                    v
        |             effective manifest IR
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

Registry-backed edges enter the resolver through a source-keyed backend
registry. npm and JSR adapters normalize metadata, transitive source identity,
artifacts, integrity, and capabilities before the shared parallel resolver and
store path. Workspace/path/git retain their distinct local semantics. See
`docs/dev/m70a-registry-backends.md`. Add selects one typed backend before
metadata lookup; npm is the default and there is no cross-registry discovery or
fallback policy.

`internal/packageidentity` owns the boundary between semantic package truth and
runtime compatibility spelling. `PackageIdentity` (`source + logical name`) is
used by authoring, graph, lock, and provenance. `MaterializationIdentity`
selects a package-tree name, and `ImportIdentity` selects the spelling consumed
by a runtime/compiler resolver. For Node, `jsr:@std/path` maps to
`npm-compat:@jsr/std__path`; that mapping is consumed by project results,
`why`, and materialization but never flows back into authoring truth. The
materializer validates the complete destination plan before filesystem writes
and rejects two semantic packages that map to one path.

Runtime, optional, and peer registry metadata are normalized into
source-qualified facts. Shared environment facts flow through the M70x
requirement tape before the existing version selector. npm alias values retain
their local reference separately from semantic target identity. See
`docs/dev/m70x-requirement-tape.md`.

Dependencies point down toward domain and infrastructure capabilities. Core
packages must not import `internal/cli` or `internal/integrations`. Integrations
may consume stable domain contracts but must not import CLI presentation.
Architecture tests enforce these two rules and keep `cmd/tspack` bootstrap-only.

## Command and application layer

`internal/cli/app.go` owns top-level help/version behavior. `registry.go` is the
single command-registration point. Each lifecycle command has a dedicated
feature-named `*_command.go` owner for parsing, typed request construction,
rendering, and exit mapping. `reports.go` owns lifecycle JSON contracts;
specialized commands such as audit and adoption keep schemas beside their
implementation. `workspace.go` and `lifecycle_command_paths.go` own cheap path
context only. Potentially expensive manifest/frontend work remains explicit.

`internal/project` owns package-manager semantics. `lifecycle_operations.go`
exposes command-specific requests and semantic results without terminal or JSON
dependencies. Feature-named operation files own the implementation. The broad
`project.Result` survives as a compatibility adapter, not as the preferred new
application API. CLI must not reimplement resolution, lockfile, store,
materialization, policy classification, or security-gate behavior.

## Manifest frontend boundary

internal/manifestfrontend is the durable Go execution boundary. It owns the
process-local Node worker, JSON-line protocol, request-scoped cwd and
environment, one-shot compatibility fallback, shutdown, and structural
counters. internal/project and internal/cli consume this boundary and validate
normalized output in internal/manifest. Worker reuse amortizes Node and
TypeScript module bootstrap only; each request rereads and reevaluates the
manifest. Changed frontend artifacts invalidate worker reuse.

The TypeScript package in `manifest-frontend` owns TSX evaluation and authoring
types. Go discovers and invokes its built bridge through `internal/bridge` and
`internal/nodecmd`, then validates the normalized result in `internal/manifest`.
Generated declaration copies have one source in the frontend/build pipeline and
are checked with `tspack compat diff`.

## Dependency authoring boundary

`internal/authoring` owns declarations before resolution: source-qualified
package identity, dependency kind, provenance, layer/order, authority,
editability, shadow decisions, and pure edits. `internal/manifest` builds each
package tape during normalized IR validation and projects only effective
dependencies into the existing graph. Graph and resolver packages do not own
authoring precedence, and lockfiles do not contain authoring history. Concept,
template, package-manifest, and compatibility producers converge on the same
declaration vocabulary. See `docs/dev/m69a-authoring-ir.md` for the full model.

`tspack add` and `tspack remove` enter this boundary as typed
`project.AddDependencyRequest` and `project.RemoveDependencyRequest` values.
Project orchestration selects the package and declaration, applies the pure
authoring edit, asks `internal/manifestedit` for a projection, performs a guarded
atomic manifest write, and invokes the ordinary update operation. Remove
classifies any unshadowed declaration before projection and classifies resolved
lock persistence only after update. CLI code only parses flags and renders the
typed result. Source-qualified backend metadata and artifacts are memoized
across selection, edit preflight, and update commit so the safety pass does not
duplicate registry requests or share cache entries across sources; remove
does not query registry metadata merely to select a declaration.

## Cross-cutting services

- Diagnostics originate as `internal/diag.Diagnostic`; feature reports translate
  them only at their presentation boundary.
- Boundary/type checks live in `internal/check`, `boundary`, and `typesurface`.
- Lifecycle capabilities and evidence live in `capability`, `installscript`, and
  `securityevidence`; OSV querying lives in `audit`.
- Audit coverage is a typed per-source result. npm has an OSV ecosystem; JSR is
  an unsupported ecosystem rather than an npm alias, and unmapped sources have
  unknown coverage.
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

Unit and application tests live beside their semantic owner. Parser, renderer,
and report tests stay in CLI and run in-process when process behavior is not
under test. `cli.App` owns injected IO and returns exit status; only
`cmd/tspack/main.go` turns that status into process termination. `clitest.RunApp`
is the default CLI harness. Explicit process runners share one binary per
package run; do not add per-test `go run ./cmd/tspack` calls. Keep process tests
for dispatch, exit codes, real pipes, signals, environment, process trees,
cross-process locks, and executable behavior. Slow/live tests must state their
external requirement. Browser and Skyrim tests live with their integration
packages. See `docs/dev/testing-strategy.md` for the evidence hierarchy.

Generated files are changed through their owner generator or compatibility
writer. `compat diff`, generator tests, frontend type checks, and embedded-surface
tests are the drift gates. Generators compare desired and existing bytes before
writing so an unchanged generation pass does not churn timestamps or invalidate
tool caches.

The complete Go source roots are `cmd`, `internal`, and `tools`. Repository-root
`go test ./...` is not the normal validation command because `dist` can contain
large generated release and benchmark workspaces that Go package discovery will
scan. Use `go test ./cmd/... ./internal/... ./tools/...`; this preserves the Go
package architecture without coupling correctness latency to generated output
volume.

## Where a new feature goes

1. Put new semantic vocabulary in `manifest`, `graph`, or another durable domain
   package only when it is genuinely shared.
2. Put a new lifecycle request/result and orchestration in `project`; put lower
   resolution or persistence mechanics in their existing core owner.
3. Put a command's registration in `registry.go`, parsing/rendering/exit mapping
   in one feature-named CLI file, and semantic behavior below the CLI boundary.
4. Put OS/browser/ecosystem/deployment-specific behavior in
   `internal/integrations/<name>` behind a narrow adapter.
5. Put fixtures under `fixtures` or package `testdata`; keep generated output out
   of production source directories unless it is embedded runtime data.
6. Avoid new top-level `internal` packages unless the responsibility is durable,
   named, and has a clear dependency direction. Never create a generic `utils`.
