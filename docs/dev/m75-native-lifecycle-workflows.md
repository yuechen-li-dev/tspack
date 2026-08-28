# M75 native lifecycle workflows

M75 adds first-class Workflow IR operations for `Build`, `Test`, and `Audit`.
They are native operations, not aliases for command strings. Local and GitHub
thin-runner execution use the same provider-neutral plan.

## Ownership after M75

- `project.RunBuild` owns typed package/target selection, ordered application
  results, artifact aggregation, diagnostics, and cancellation boundaries.
- `project.RunTest` owns the typed test request/result boundary and calls the
  existing context-aware xTest/Vitest application backend. Native xTest JSON is
  retained as counts and per-test evidence.
- `project.RunAudit` owns lockfile loading, source coverage, severity policy,
  finding counts, and cancellation-aware OSV scanning.
- `internal/cli` parses flags, renders human/JSON reports, and maps failures to
  exit status. Workflow projects the same facts into step results and events.

The existing compiler implementations are still physically located in the CLI
package. A direct adapter invokes them without self-shelling, while
`project.RunBuild` owns the shared orchestration contract. This means Build
compiler invocations receive the workflow/CLI cancellation context and use
owned process-tree cleanup. Completing the physical compiler extraction is the
explicit remaining architectural item; duplicating compiler behavior in
workflow was rejected.

## Effects and concurrency

Plans expose conservative effects: Test is workspace-read plus process, Build
is workspace-read/write plus process, and Audit is workspace-read plus network.
Native steps do not accept arbitrary working directories or environment/secret
bindings. Jobs may run concurrently under the existing scheduler. Tests are
never deduplicated; explicit Audit steps always execute; Build relies on the
existing compiler cache/fingerprint behavior rather than workflow memoization.

The root CI workflow now uses `Sync()`, `Check()`, `Test()`, `Build()`, and
`Audit()` in its validation job. Its additional focused Go and manifest frontend
jobs remain because native lifecycle defaults do not yet represent the complete
release qualification matrix (VS Code, compatibility drift, and all explicit
developer commands).
