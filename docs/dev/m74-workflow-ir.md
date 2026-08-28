# M74 Workflow IR foundation

## Boundary

M74 establishes this dependency direction:

```text
manifest workflow declaration
  -> internal/manifest validated declaration IR
  -> internal/workflow deterministic execution plan
  -> local executor or GitHub thin-runner adapter
```

`internal/workflow` is a durable application/runtime owner. It imports manifest
truth and typed project lifecycle operations; neither core package imports CLI.
The CLI owns parsing, human/JSON rendering, exit mapping, and explicit provider
artifact writes.

The manifest frontend continues to evaluate only a bounded declarative subset.
Workflow helpers construct inert data. Functions, loops, conditionals, network,
filesystem mutation, `process.env`, `fetch`, arbitrary helpers, and arbitrary
imports remain rejected during declaration.

## Reuse audit

The M68 application seams support native `Sync`, `Check`, and `Pack` execution
through `project.RunSync`, `project.RunCheck`, and `project.RunPack`. Dependency
resolution, store hydration, materialization, graph checks, and package creation
remain in their existing owners.

Build compiler orchestration, xTest/Vitest execution, and OSV audit still have
CLI-owned behavior. M74 does not copy that logic into the workflow package and
does not shell back into the TSPack CLI to pretend those are native operations.
They can be requested through visible `Process` escape hatches while future
lifecycle decomposition moves typed operations below CLI.

| Existing concern | Current owner | M74 use |
| --- | --- | --- |
| sync / store / materialization | `internal/project`, `store`, `materialize` | native typed operation |
| check / graph / security policy | `internal/project`, `check`, `graph` | native typed operation |
| pack / publish file policy | `internal/project`, `pack` | native typed operation; publish remains package policy |
| build / compiler targets | `internal/cli`, `internal/compiler` | visible process escape hatch until application seam exists |
| test discovery / xTest | `internal/cli`, `internal/testcmd`, frontend native test | visible process escape hatch until application seam exists |
| audit / OSV | `internal/cli`, `internal/audit` | deferred native operation; no network hidden in workflow planning |
| RunTarget and environment contracts | `internal/cli`, `internal/manifest` | not reimplemented; distinct runtime target concept |
| package selection | `internal/project` and manifest package identities | reused by native Pack and package cwd resolution |
| CLI process ownership | command-specific CLI owners | workflow owns only its explicit external child process trees |

## IR and normalization

Workflow identity, job identity, step ordinal, matrix axes, trigger filters,
platform, package selectors, environment value kind, secret identity, argv,
shell-script intent, cwd, capabilities, and timeout survive normalization.
Validation rejects duplicates, invalid identities, unknown/self dependencies,
cycles, invalid platforms, empty matrices, unsupported step kinds, unsafe cwd,
unknown packages, invalid environment entries, and secret values embedded where
only a reference is legal.

Planning sorts map-derived facts, expands matrices before backend selection,
assigns ordinal step identities, and connects a dependent job to every expanded
instance of each declared prerequisite. Plans contain no clocks, random IDs,
temporary paths, or provider expressions.

## Execution

The scheduler has a fixed concurrency bound. Facts from independent jobs run in
parallel; state aggregation and dependency decisions are serialized. States are
pending, running, succeeded, failed, skipped, blocked, and cancelled. Default
step failure ends the job; blocked dependencies never start.

Events are provider-neutral semantic values: workflow/job/step start, step
output, step/job completion, and workflow completion. CLI text and line-delimited
JSON renderers consume them. Runtime durations are result metadata, not plan
identity.

The executor owns a typed `ExecutionContext` with `IsCI` and an optional
provider identity. Steps do not inspect ambient provider variables or branch on
the provider by default; the fact remains available for future operations that
genuinely need it.

External processes use executable/argv APIs. Shell interpretation exists only
for `shellScript`. Environment resolution merges job then step declarations,
resolves secret identities at execution time, and redacts known secret material
from streamed output. Workspace and package cwd resolution stays bounded.

## GitHub decision

Thin-runner mode is the first backend because it preserves exact local/provider
planner and scheduler parity. The adapter owns trigger YAML, checkout, first-party
release setup, GitHub secret expression syntax, and runner mechanics. It does
not lower the semantic DAG into a second native GitHub scheduler.

Generated YAML is deterministic, parser-validated, marked as generated, and
written only by explicit export. Drift check is explicit through export
`--check`; it is not yet folded into general `tspack check` so repositories with
hand-written workflows remain untouched.

## Deferred boundaries

- typed Build/Test/Audit after application-seam decomposition;
- conditions and typed output references;
- provider artifact upload/download mechanics;
- cache intent (native operations should derive safe cache identity);
- job-level timeouts, retries, resumability, and history;
- native GitHub DAG lowering, established only if a real need appears;
- second provider abstraction, established by a second provider;
- Deployment IR, which owns desired hosting/cloud topology rather than CI
  execution graphs.

## Baseline and measured overhead

Implementation started from clean `main` at `fcc5174` with Go 1.27.0, Node
26.2.0, and the repository release contract at v0.1.8. The explicit-root Go
suite passed before changes.

On the Windows amd64 development host (Ryzen 7 7700X), an 80-instance matrix
plan benchmark was 69-70 microseconds per plan. A 40-job no-op bounded scheduler
run was 68-69 microseconds. Five fresh CLI `workflow inspect` processes measured
862, 179, 174, 178, and 175 milliseconds; the first includes cold executable
and Node/frontend startup, while warmed process runs were about 176 milliseconds.
Planning and scheduling are negligible beside lifecycle work; frontend/process
startup remains the dominant inspect cost.
