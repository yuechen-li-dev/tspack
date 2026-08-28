# M76 workflow language and Flow IR

## Status and baseline

M76 started from revision `60665ac36d4c0e2dcec9a6cd3c4b74135d0de214`
with Go 1.27.0 and Node 26.2.0. The worktree was clean. Before changes,
`go test ./cmd/... ./internal/... ./tools/... -count=1 -timeout 420s` passed,
the manifest frontend passed 245 tests with 2 skipped, and the VS Code extension
passed 35 tests.

The M75 root workflow was a three-job DAG. The `validate` job ran native Sync,
Check, Test, Build, and Audit steps. Two `needs: ["validate"]` jobs then ran the
focused Go workflow tests and manifest frontend workflow tests. The generated
GitHub artifact remained a single Linux thin runner invoking
`tspack workflow run CI --ci-provider github`.

## M75 audit and YAML residue

M75 has useful typed effects and application seams, but control flow lives in
the scheduler rather than in its `Plan` data. `PlanJob.Needs` is interpreted by
maps of pending/running/terminal job states. Sequential continuation is the
position of a step in a slice. Parallelism is the accidental readiness of
multiple jobs plus goroutine availability. Dependency failure becomes
`blocked` in scheduler branches. Timeout and cancellation are context state
translated into terminal step status after effect execution.

| M75 concept | Classification | M76 decision |
| --- | --- | --- |
| workflow and trigger | fundamental | keep triggers outside execution flow |
| typed lifecycle/process effect | fundamental | keep behind one explicit effect boundary |
| step timeout | fundamental policy | lower to an explicit timeout event path |
| job | placement/isolation boundary plus authoring sugar | lower legacy jobs; do not make jobs the control primitive |
| needs | provider-origin authoring artifact | derive control edges while lowering legacy declarations |
| steps array | convenient sequence sugar | replace with language-shaped sequence data |
| matrix job decoration | provider-origin sugar | lower compatibly; future authoring uses bounded fan-out |
| job condition strings | provider-origin artifact | reject provider expressions; use typed outcome guards |
| skipped/blocked | derived execution status | retain in results, not as hidden scheduler control |

## Desired authoring language

The TypeScript manifest frontend intentionally evaluates a bounded declarative
subset. Function-valued builder callbacks, arbitrary loops, and ordinary
imperative execution would violate that purity boundary. M76 therefore uses
language-shaped inert combinators whose arguments are plain typed data:

```ts
Workflow("CI", {
  triggers: [Manual(), Push({ branches: ["main"] })],
  flow: Sequence(
    Sync(),
    Check(),
    Parallel(
      Branch("test", Test()),
      Branch("build", Build()),
    ),
    Audit(),
  ),
});
```

Representative designs establish that the surface can grow without changing
the runtime:

```ts
// Linear CI
Sequence(Sync(), Check(), Test(), Build(), Audit())

// Conditional path (typed outcome matching; staged after the first slice)
MatchResult(Build(), {
  succeeded: Pack(),
  failed: Report(),
  cancelled: Cancelled(),
  timedOut: ReportTimeout(),
})

// Always-run cleanup (admitted by IR; authoring helper staged)
Finally(Build(), Cleanup())

// Retry is an explicit cycle with a bounded counter, not scheduler magic
Retry({ limit: 3 }, Build())

// Bounded fan-out replaces matrix decoration
ForEach("platform", [Linux(), Windows()], On(Value("platform"), Test()))
```

`Sequence`, `Parallel`, and `Branch` are declarations, not JavaScript control
operators. Their constructors only return inert authoring records. Native
effect helpers continue to return inert effect declarations and never execute
project work while loading the manifest.

## Comparative design record

### Oct Flow MIR

Borrowed: named entry state, explicit state bodies, explicit goto/return/
suspend-style boundaries, typed locals/value identities, match-shaped control,
and a step budget. Adapted: a workflow state requests one asynchronous effect
and waits for a typed outcome event rather than evaluating general expressions.
Rejected: Oct expression MIR, mutable blackboard language, stack calls, compiled
backend machinery, and durable suspension implementation. TSPack needs a small
effect machine, not a general language compiler.

### Oct `Make.oct`

Borrowed: orchestration is ordinary readable language-shaped source; planning
constructs typed data; effects cross an explicit authority boundary; command
argv is distinct from shell text; plan/config can be tested without running
tools; maximum steps prevent hanging flow targets. Adapted: TSPack keeps its
manifest purity evaluator and uses inert combinators instead of evaluating
normal Oct functions. Rejected: target DAG as the primary workflow model,
incremental file/timestamp semantics, and Oct as an execution dependency.

### Copeland TS

Borrowed: discriminated payload results, exhaustive match intent, explicit
fallibility, and ordinary readable control-flow conventions. Adapted: outcome
guards in normalized data stand in for a general-purpose source match parser.
Rejected: rebuilding Copeland parsing, typechecking, bind propagation, or MIR in
Go. The existing TypeScript manifest frontend remains the authoring boundary.

### MachinaLayout DeusMachina

Borrowed: one normalized transition runtime for multiple authoring surfaces,
typed events, deterministic source-order selection, immutable snapshot wrappers,
and traces containing state before/after, considered transition, selected row,
and reason. Adapted: transition actions are serializable effect references,
never functions, and workflow states are flat stable node identities rather
than hierarchical UI paths. Rejected: mutable boards, callback guards/actions,
utility scoring, parent fallback, and stack navigation.

### MachinaLayout `matchKind`

Borrowed: finite discriminated outcome kinds and exhaustive authoring intent.
Adapted: normalized guards are enum values such as `effectSucceeded`,
`effectFailed`, `cancelRequested`, and `timeout`; Go validation checks complete
and unambiguous machine paths. Rejected: runtime TypeScript dispatch.

### MachinaIter

Borrowed: a visible cursor when iteration position matters, typed yielded/done/
failed states, inspectable trace, and a default 10,000-step safety limit.
Adapted: the workflow snapshot carries active node identities and explicit
branch state; bounded fan-out will carry item index/value as plain data.
Rejected: forcing linear workflows through an iterator abstraction or hiding
continuation in a generator.

### TSPack M75

Borrowed: provider-neutral effects, lifecycle application seams, process-tree
cancellation, secret redaction, conservative capabilities, deterministic
matrix expansion, and the GitHub thin-runner boundary. Adapted: legacy
jobs/needs/steps lower to Flow IR and run on the same machine executor as new
authoring. Rejected: jobs as the semantic control primitive and map readiness as
the source of workflow meaning.

## Authoring IR and execution IR

The manifest authoring IR is a recursive finite tree: sequence, parallel with
named branches, effect, and later match/finally/fan-out nodes. It preserves
readability and placement metadata but is not executed directly.

The normalized `Flow` schema is versioned and graph-shaped for inspection:

```text
Flow {
  schemaVersion
  identity, entry, success, failure, cancelled
  regions[]
  nodes[] { identity, kind, region, effect?, join? }
  transitions[] { identity, from, event, guard?, to }
  values[] { identity, type, producer }
}
```

Effects are semantic records containing operation and bounded arguments.
Control nodes represent entry, fork, join-all, and terminals. Every effect has
explicit success, failure, cancellation, and timeout transitions. Default
failure propagation is therefore visible in the graph even though authoring
sugar omits it.

An execution snapshot records node status, active effects, branch membership,
typed effect results by stable value identity, cancellation state, step count,
and trace cursor. Runtime goroutines may perform effects, but they do not carry
semantic continuation state.

## Determinism, joins, and races

`Step(flow, snapshot, event)` is pure with respect to control selection. It
validates the event against the current active node, considers transitions in
stable declaration order, rejects ambiguity, increments the step count, and
returns a copied snapshot plus trace. Unknown state/value/transition targets,
duplicate identities, unreachable required terminals, illegal joins, and
accidental cycles are validation errors.

Parallel branches start together. The initial policy is `all`: every branch
must succeed before the join continues. A failed, cancelled, or timed-out
branch selects a defined terminal path, blocks pending sibling continuation,
and retains the real result of effects already running in that wave. Physical
completion events are buffered and applied in stable branch
identity order when more than one is ready, so goroutine timing does not define
the semantic result. Future `any` or `firstSuccess` constructs require distinct
IR policy values rather than changing `all`.

## Values, placement, and future growth

Effect outputs receive stable SSA-like value identities. A value has one
producer and is immutable. Consumers refer to value identities, so data flow
can imply control dependencies without GitHub-style string outputs. The first
M76 slice preserves typed native results in snapshots and traces; field-level
artifact references and exhaustive result-match authoring can be added without
changing effect invocation or stepping.

Execution regions own platform, environment, and isolation facts. Legacy jobs
lower to regions only when those facts are semantically relevant. New logical
sequence and parallel structure is independent of placement. Matrix remains a
compatibility expander; future `ForEach` uses an explicit bounded cursor.

General runtime loops are deferred. A future cycle must be explicitly marked
and bounded. Every run has a default 10,000 semantic-step budget and bounded
trace retention; terminal summary counts survive trace truncation.

## Provider boundary

Triggers remain external start-selection metadata. Flow nodes, events, guards,
effects, values, and regions contain no GitHub, GitLab, YAML, provider
expression, `needs`, or runner concept. GitHub export remains a thin runner and
does not lower the Flow graph into a second provider scheduler.

## Implemented M76 slice and measured cost

The landed slice includes inert `Sequence`, `Parallel`, `Branch`, and `On`
authoring; legacy job/matrix lowering; schema-versioned Flow nodes, transitions,
regions, effect values, fork/join data, and terminals; validation; immutable
snapshots; typed machine events; deterministic stepping; bounded traces; a
10,000-step default budget; a no-effect simulator; and a concurrent real
executor that uses the same stepping function. The previous jobs/needs
scheduler implementation was removed.

On the baseline Ryzen 7 7700X Windows host, three benchmark samples measured:

- preferred-flow lowering: 13.6-13.9 microseconds;
- Flow validation: 4.0 microseconds;
- one cloned-snapshot machine transition: 1.65 microseconds;
- a 40-effect no-op bounded executor run: 3.38-3.78 milliseconds.

The no-op executor benchmark intentionally amplifies orchestration overhead;
real Build/Test/Audit/process work dominates it. Snapshot copying favors
inspectability and deterministic simulation over premature mutation-based
optimization.

M76 stops at Outcome B. Exhaustive TypeScript `MatchResult` authoring,
field-level typed value references, friendly `Finally`, and explicit-cursor
`ForEach` remain deferred. The normalized machine already supports custom
failure transitions and typed result storage, so those additions do not require
a second runtime or another execution-IR redesign. Compiler implementations
also remain physically in `internal/cli`; the existing typed project Build seam
keeps that cleanup orthogonal to Flow semantics.

## Verification

- Real root `CI` run: Sync, Check, Test (4 passed), Build, Audit (zero
  blocking findings with complete coverage), then two concurrent process
  branches; terminal status succeeded.
- Broad Go validation:
  `go test ./cmd/... ./internal/... ./tools/... -count=1 -timeout 420s` passed.
- Focused Flow race validation passed for lowering, simulator, machine, and
  real-executor tests.
- Manifest frontend: 28 files passed, 246 tests passed, 2 environment-dependent
  tests skipped. One first full run hit a transient Playwright
  `page.reload: net::ERR_ABORTED`; an immediate complete rerun passed.
- Manifest frontend build, bridge build, manifest API typecheck, and root
  `tsconfig.tspack.json` typecheck passed.
- VS Code extension: 4 files and 35 tests passed; compile and canonical UI
  context drift check passed.
- Legacy workflow CLI list/inspect/run/export coverage passed through legacy
  lowering and the Flow executor.
- `tspack check --root .` passed with the repository's existing advisory
  multi-version and blocked-lifecycle warnings.
- GitHub thin-runner export drift check passed unchanged.
- `git diff --check` passed.

The baseline had 840 top-level Go tests; the final tree has 847, including 14
top-level workflow tests. The manifest frontend moved from 245 passed plus 2
skipped to 246 passed plus 2 skipped. VS Code remained at 35 passed tests.
