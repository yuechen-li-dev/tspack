# M68c CLI runtime decoupling and test rationalization

## 1. Summary

M68c removed normal process termination from `internal/cli`, introduced an
injectable `cli.App`, made status an explicit return to the executable, and
converted the majority of identifiable TSPack subprocess call sites to an
in-process lane. The retained process cluster is concentrated in RunTarget,
xTest, inspect/browser-bridge, child-tool, environment, readiness, stream, and
process-tree behavior.

The structural change succeeded, but the comparable CLI runtime did not
materially improve. Removing outer TSPack processes exposed the next blocker:
the expensive work is repeated manifest-frontend, compiler/typecheck, and child
tool execution inside the commands, not TSPack executable startup. The current
runtime also uses a serialized compatibility bridge for legacy renderers and a
typed recovered exit status while handler signatures are migrated. Therefore
this pass is Outcome B, not a claim of completed runtime/test redesign.

## 2. Baseline

- Branch and HEAD: `main`, `fd45050`.
- Working tree: clean before M68c.
- Version: `tspack v0.1.8`, commit/build metadata unknown under `go run`.
- CLI: 70.777s wall time (`go test` reported 69.826s).
- Project: 3.139s wall time (`go test` reported 2.204s).
- Full Go suite: 271.984s wall time; all packages passed.
- Production `os.Exit`: 127 occurrences: 125 in `internal/cli`, two in the
  Skyrim integration, and none in the executable bootstrap.
- TSPack subprocess references: 164 under the reproducible narrow definition
  (`exec.Command` lines naming the shared TSPack binary or `cmd/tspack`). There
  were 232 child-process references overall, including legitimate Node, Go,
  Git, shell, bridge, compiler, and tool processes.
- Process-bound test functions: 93 by function-body inspection.
- Test files: 82 repository-wide and 30 in CLI; CLI tests were 10,389 lines.
- Largest CLI test: `cli_tools_integration_test.go`, 1,523 lines.
- Shared CLI builds: one in `TestMain`; `buildTspackBinary` only returns it.
- `t.TempDir` references: 538. Golden/snapshot text references: 67. Test names
  containing network/live/registry/browser/Skyrim: 44. These are inventory
  signals, not claims that every matching test is live or every temp directory
  is heavyweight.

## 3. CLI runtime before

`cli.Main` dispatched `func([]string)` handlers. Expected failures called
`os.Exit` throughout parsers, renderers, helpers, and command orchestration.
Handlers wrote to global process streams, so an ordinary parser or JSON/output
assertion often had to launch the shared executable to survive failure paths and
capture output.

## 4. CLI runtime after

`cli.App` owns stdin, stdout, and stderr and exposes
`Run(context.Context, []string) int`. `cmd/tspack/main.go` constructs the default
app, runs it, and is the only normal CLI owner of `os.Exit`. Unknown commands and
help/version dispatch return status directly. Existing command exits cross one
typed compatibility boundary and are recovered only by `App.Run`; unrelated
panics are rethrown.

## 5. Process ownership and IO injection

Production CLI `os.Exit` calls fell from 125 to zero. Repository production
occurrences fell from 127 to three: the executable's single normal termination
and two integration-specific Skyrim exits pending integration result plumbing.
An AST guardrail enforces zero CLI calls and exactly one executable call.

`App.SetIO` supports a harness without importing CLI internals. Legacy renderers
still reference process files, so custom IO is connected through serialized OS
pipes for the duration of a run. This preserves child-tool stream behavior and
made all handlers exercisable in-process, but serialization/global stream
connection is explicitly transitional; direct writer threading is the next
runtime blocker, not an intended embedding API.

## 6. Test-pyramid audit

| Lane | Contract proved | Rough cost observed | Duplicate/fixture finding | Process needed? |
| --- | --- | --- | --- | --- |
| A. semantic/application | resolution, policy, diagnostics, mutation authority | milliseconds to a few seconds when frontend/store work is real | lifecycle semantics already have strong `internal/project` coverage | no |
| B. CLI parser | flags, positionals, invalid combinations | normally milliseconds in-process | many historical cases launched TSPack only to survive `os.Exit` | no |
| C. renderer/output | text, JSON schema, diagnostic mapping | normally milliseconds after semantic result exists | repeated text/JSON process assertions overlap renderer helpers | no |
| D. process boundary | executable status, pipes, environment, signals, child trees | tens of milliseconds through seconds | valuable around RunTarget/xTest; redundant for help/init/doctor/lifecycle formatting | yes |
| E. filesystem/materialization | lock/store/materialized files, hardlinks, Windows replacement | package baseline 3-5s for project/materialize clusters | use application snapshots instead of one CLI launch per read-only command | only for cross-process locks |
| F. network/registry | registry metadata, OSV/HTTP failures, resolver concurrency | variable; resolver baseline 3.095s | local servers are appropriate; live service dependence should remain isolated | only for transport/process facts |
| G. browser/Playwright | bridge routing, target startup, inspection artifacts | typically child-process/readiness dominated | do not repeat core parsing or mutation matrices here | often |
| H. platform/Windows | locks, path/executable resolution, process trees, rename/retry | materialize/process dominated | unique Windows evidence retained | sometimes |
| I. fixture/golden | stable large contracts and generated drift | fixture dependent | small strings/structs should use direct assertions; no golden was removed without a reviewed replacement | no |
| J. architecture | dependency direction, bootstrap and exit ownership | sub-second | unique, cheap regression prevention | no |

## 7. Redundant categories and conversions

The audit found process duplication in help aliases, init validation/output,
doctor text/JSON, check/update parsing and rendering, why output, migrate output,
artifact/doom presentation, and pack output. Those tests now use the in-process
runtime while retaining their existing semantic assertions. Policy planning and
outdated shape tests were already direct. Project tests remain the primary proof
for lifecycle mutation authority.

No test was deleted merely from line-level review: the large run/tool files mix
parser cases with real child-process contracts, and deleting whole clusters
would weaken evidence. Test functions removed: zero. Fixtures removed or
consolidated: zero. The reduction is 89 process call sites and 44 process-bound
test functions, not a ceremonial test-count reduction.

## 8. Lifecycle, clitest, fixtures, waits, concurrency, and platforms

- Check, update, why, pack, policy planning, and related JSON/text assertions can
  now execute through `App.Run`; sync/outdated semantic mutation guarantees stay
  at the typed project-operation layer.
- `clitest` now separates `RunApp` from explicitly named `RunProcess` and
  `RunProcessInDir`. Workspace snapshots/assertions remain small helpers.
- A narrow `inProcessCommand` migration adapter supports old tests that used
  only `Run`, `Output`, or `CombinedOutput`; it intentionally has no Start,
  Wait, Process, signal, or process-state API.
- The adapter serializes temporary cwd/environment changes and restores both.
  Combined stdout/stderr uses a synchronized buffer; the first converted run
  caught and fixed this concurrency hazard.
- Arbitrary sleeps were not mechanically removed. Existing waits in process
  readiness/termination tests observe sockets or process output; `perf` retains
  a 5ms timing fixture. Further wait changes without dedicated flake evidence
  would trade determinism for a cosmetic reduction.
- Resolver concurrency, store synchronization, Windows file-lock/retry, and
  RunTarget child-tree coverage remain with their semantic/platform owners.
  No Windows-specific invariant was removed.

## 9. Integration placement and retained subprocess spine

Browser and Skyrim packages remain below `internal/integrations`, with the
existing dependency-direction guardrail. Normal core validation was not hidden
or re-tagged. The 49 retained process-bound test functions cover these unique
categories:

- shared executable construction and one independent executable/help smoke;
- xTest/native bridge invocation, compact/list/filter stream contracts, and
  missing-bridge behavior;
- RunTarget argv/no-shell behavior, cwd, environment/PATH/runtime inheritance,
  child output passthrough, readiness, timeout, early exit, Ctrl+C/process-group
  cleanup, and finite/server child-tree termination;
- inspect/browser bridge invocation and cleanup when a started target or bridge
  fails;
- one Biome backend start-failure boundary.

The confidence spine continues to include lifecycle application tests, the
fresh update/sync/check project flows, read-only operation snapshots,
representative resolver/security tests, RunTarget process flows, and specialized
integration tests.

## 10. Structural metrics

| Metric | Before | After |
| --- | ---: | ---: |
| production `os.Exit` | 127 | 3 |
| production CLI `os.Exit` | 125 | 0 |
| identifiable TSPack subprocess references | 164 | 75 |
| process-bound test functions | 93 | 49 |
| in-process converted call sites | 0 | 94 (`newInProcessCommand` 88, direct `runTestApp` 6) |
| CLI test files | 30 | 32 |
| CLI test LOC | 10,389 | 10,598 |
| largest CLI test file | 1,523 lines | 1,523 lines |
| shared CLI binary builds | 1 | 1 |
| test files removed | 0 | 0 |
| test functions removed | 0 | 0 |
| fixtures removed/consolidated | 0 | 0 |
| retained process-specific test functions | 93 | 49 |

The two new CLI test files are the direct `App` contract and the temporary
in-process migration adapter. The added LOC is runtime/harness and guardrail
evidence, not additional semantic matrices.

## 11. Runtime metrics and slow clusters

| Suite | Before wall | After comparable wall |
| --- | ---: | ---: |
| CLI | 70.777s | 69.6s wall during final focused sequence; `go test` reported 68.616s |
| project | 3.139s | `go test` reported 2.450s in the final focused sequence |
| full Go | 271.984s | 276.817s |

Baseline package ranking was CLI 72.628s, materialize 5.076s, resolver 3.095s,
project 3.046s, testcmd 1.998s, installscript 1.119s, projectir 1.085s,
templates 1.042s, Skyrim 0.833s, and check 0.774s.

Post-conversion CLI per-test timing identifies the real remaining costs:

| Test | Seconds |
| --- | ---: |
| TemplateManifestEditorTypecheck | 6.93 |
| TemplateManifestEditorTypecheck/static | 5.49 |
| RuntimeSwitchDoctorRuntimeReportsSelectedProfile | 4.31 |
| CLIRunRuntimeSwitchExplicitTargetsStayExplicitAcrossWorkspaceProfiles | 4.04 |
| CompatHelpersFixtureCommands | 3.22 |
| RepoRootManifestNarrowsManifestEditorTSConfig | 2.15 |
| CLICheckFormatBackendAndConfigBehavior | 1.70 |
| CLIInspectRunTimeoutAndExitedEarly | 1.52 |
| CLIRunStdoutMatchStreamSelectionAndEarlyExit | 1.52 |
| CLICheckFormatJSONMissingBackendIsStructured | 1.47 |

The baseline run did not retain per-test JSON, so a before/after test-name table
would be invented. Package-level before ranking and exact after test timing are
reported instead. The final measured after package values available from
uncached focused/full runs were CLI 67.914-68.616s, project 2.414-2.450s,
resolver 2.272s, testcmd 1.544s, check 0.565s, and pack 0.486s. Other packages
were cached in the final full run, so a synthetic ten-package after ranking is
not reported.

## 12. Documentation and guardrails

`docs/dev/testing-strategy.md` defines the hierarchy, process-test rule, fixture
rule, semantic matrix rule, integration rule, mutation evidence, and performance
review. `AGENTS.md` carries the concise enforceable version. Architecture docs
now name `App.Run`, returned status, injected IO, and `RunApp` as the default.

Architecture tests enforce bootstrap-only `cmd/tspack`, one executable
`os.Exit`, zero CLI `os.Exit`, core/integration dependency direction, the
explicit registry, and typed lifecycle facade placement.

## 13. Deferred larger ideas

| Idea | Problem | Direction and benefit | Risk and why separate |
| --- | --- | --- | --- |
| Public embedding API | current IO bridge serializes global stream connection | writer-aware handlers and stable requests would allow concurrent embedding | public compatibility commitment; separate API design |
| Complete `project.Result` removal | compatibility union still exists | finish typed consumers and remove pointer-union branching | broad caller migration, limited M68c speed benefit |
| Graph/lock IR redesign | related dependency representations remain | explicit resolved-graph boundary could remove conversions | selection/format drift risk |
| Filesystem transactions | commands own several staged-write patterns | purpose-built commit/rollback primitives | Windows durability and fault-injection scope |
| Resolver redesign | repeated frontend/resolver setup dominates integration cost | reusable immutable semantic inputs and narrower resolver fixtures | resolver semantics and concurrency risk |
| Plugin architecture | integrations remain compile-time | define extension contracts from multiple proven adapters | premature public abstraction |
| Test sharding | CLI is the dominant package | split execution after evidence lanes are stable | CI infrastructure, not local evidence design |
| Remote fixture cache | repeated tool/frontend work is costly | content-address immutable prepared fixtures | cache invalidation and CI trust model |
| Distributed CI | wall time includes build/tool startup | schedule isolated lanes independently | infrastructure complexity; does not simplify evidence |

## 14. Validation and outcome

Validation results:

- `go test ./internal/cli -count=1`: passed, 68.616s reported.
- project, check, and resolver focused suites: passed; resolver race suite
  passed in 3.817s.
- `go test ./... -timeout 420s`: passed in 276.817s wall time; CLI reported
  67.914s.
- manifest frontend build and manifest API typecheck: passed; frontend tests:
  201 passed, two expected inspect-environment skips.
- VS Code compile and tests: passed, 35 tests.
- compatibility diff: all five generated/editor surfaces up to date.
- version, help, check, why, and outdated smokes: passed. Check retained the
  existing non-error conflict/lifecycle diagnostics.
- audit executed and intentionally returned 1 for six existing advisories. The
  current set includes critical `GHSA-5xrq-8626-4rwp` for Vitest 2.1.9 plus
  Vite, nanoid, and esbuild findings; dependency remediation is outside M68c.
- the platform-appropriate PowerShell self-host command matrix passed and
  verified no tracked-state mutation. The first comparison attempt used
  PowerShell array comparison incorrectly; the corrected string snapshot gate
  passed.
- `git diff --check` passed before the final report update and is rerun at
  closeout.

**Outcome B — Meaningful progression.** Runtime process ownership and majority
subprocess conversion landed, and the next blocker is isolated with timing
evidence: expensive frontend/typecheck/child-tool work remains inside commands,
while direct handler result/writer threading remains necessary to remove the
serialized compatibility bridge. Claiming Outcome A without a material runtime
reduction or with the bridge still present would be inaccurate.

## 15. Deviations from scope

- No semantic test or fixture cluster was deleted without a reviewed cheaper
  proof; conversion produced structural simplification but not test-count
  reduction.
- Direct writer/result threading was not half-implemented across 26 handlers.
  One explicit compatibility boundary was used and documented.
- No public API, graph/lock/resolver redesign, test sharding, remote cache,
  release, tag, or publish work was performed.
