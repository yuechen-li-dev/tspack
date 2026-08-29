# Workflow language

TSPack workflows are small, inert programs for software-project automation.
The TypeScript manifest builds a typed authoring tree, TSPack lowers it to a
provider-neutral Flow machine, and the Go executor performs selected effects.
YAML is only provider adapter output.

```tsx
<Workflows
  rows={[
    Workflow("CI", {
      triggers: [Push({ branches: ["main"] }), PullRequest()],
      flow: Sequence(
        Sync(),
        Check(),
        Parallel(
          Branch("test", Test()),
          Branch("build", Build()),
        ),
        Audit(),
      ),
    }),
  ]}
/>
```

`Sequence` expresses ordinary continuation. `Parallel` forks named `Branch`
declarations and joins them with the initial `all` policy. Every branch must
succeed before the continuation starts. If a branch fails, pending sibling
work and the continuation become blocked; already-running effects report their
real result before the wave closes. Same-wave results are applied in stable
node order, so goroutine completion order does not change workflow meaning.

The helpers produce plain records. The manifest frontend still rejects general
functions, loops, conditionals, arbitrary imports, `process.env`, and network
access. The expression callbacks accepted by `MatchResult` and `ForEach` are a
restricted exception: the frontend evaluates them only while constructing
inert IR and never stores a closure. Loading a manifest never performs workflow
effects.

## Effects and typed results

`Sync`, `Check`, `Test`, `Build`, `Pack`, and `Audit` are semantic effects that
call typed project application seams. `Process` is an executable plus argv;
`ShellScript` is the explicit shell escape hatch.

```tsx
Process("verify", {
  command: ["some-tool", "--verify"],
  cwd: "workspace",
  env: [WorkflowEnv("TOKEN", Secret("CI_TOKEN"))],
  capabilities: ["process", "workspaceRead", "environment", "secrets"],
  timeoutSeconds: 30,
})
```

Build, Test, and Audit return typed inert effect references. Each effect
produces one immutable result identity, and field access produces a projection
of that identity rather than a string or copied payload.

```tsx
const build = Build();

Sequence(
  build,
  MatchResult(build, {
    succeeded: result => Pack(result.artifacts),
    failed: () => Process("report failure", { command: ["report"] }),
    cancelled: () => Process("report cancellation", { command: ["report"] }),
    timedOut: () => Process("report timeout", { command: ["report"] }),
  }),
)
```

The TypeScript surface exposes only fields from the application result models:
Build has `artifacts`, `targets`, and `diagnostics`; Test has `passed`,
`failed`, `skipped`, `durationMs`, `tests`, and `diagnostics`; Audit has
`source`, `auditLevel`, `failing`, `report`, and `diagnostics`. Thus
`Build().passed`, `Test().artifacts`, and `Audit().targets` fail typechecking.
There are no GitHub-style named string outputs.

Secret references remain identities in semantic data. Values are resolved only
at effect execution and known secret material is redacted from process output.
Working directories are `workspace` or `package:<identity>` and remain bounded
to the workspace.

## Flow machine

`tspack workflow inspect CI --json` emits stable schema version 3. A conceptual
view is:

```text
entry -> Sync -> Check -> fork
                         test  -> Test  -> branch exit
                         build -> Build -> branch exit
                       join all -> Audit -> terminal/succeeded
```

The JSON contains nodes, transitions, regions, effects, value definitions,
ordered aggregate references, typed predicate nodes, projections, match nodes, iterator cursors, cleanup metadata, fork targets,
join dependencies, and four explicit effect outcome paths:
`effectSucceeded`, `effectFailed`, `cancelRequested`, and `timeout`.
Transitions contain semantic enum kinds, never provider expressions.

The snapshot records every node state, active effects, typed values, machine
status, step count, retained trace, and dropped-trace count. The pure simulator
steps `(flow, snapshot, event)` without processes. The real executor feeds the
same machine typed events after invoking effects.

Trace rows include active state before and after, the event, eligible and
selected transition, effect operation, bounded value identity, and result kind. Detailed history is
bounded to 256 rows by default while terminal state and a dropped-row count are
retained. Runs have a default 10,000 semantic-step budget.

## Failure, timeout, and cancellation

Normal authoring omits repetitive error branches, but lowering always creates
them. Failure follows `effectFailed -> terminal/failed`; timeout follows
`timeout -> terminal/timedOut`; cancellation follows
`cancelRequested -> terminal/cancelled`. A normalized machine can route failure
to a recovery/report effect. `MatchResult` deliberately replaces those default
paths for one Build, Test, or Audit result and requires all four closed outcome
kinds. Each arm lowers to a guarded transition; no provider condition or
runtime closure exists.

`Finally(body, cleanup)` lowers cleanup separately for success, failure,
cancellation, and timeout. Cleanup runs with a context detached from the
ordinary cancellation request and bounded to 30 seconds. A body failure remains
primary and cleanup failure is attached in `cleanupFailures`; cleanup failure
after success makes the workflow fail. Cancellation or timeout remains the
terminal outcome when cleanup succeeds. A failing cleanup after a cancelled or
timed-out body yields a structured failed outcome retaining the original cause.
Nested `Finally` is compositional up to eight levels.

Ctrl+C becomes a machine `cancelRequested` event. Existing context-aware native
operations and owned process-tree cleanup perform physical cancellation. Step
timeouts become machine `timeout` events rather than remaining scheduler-only
bookkeeping.

## Placement and compatibility

Logic is independent of placement. `On(platform, options, ...nodes)` creates an
execution region containing platform and environment facts. Regions answer
where effects run; sequence, branch, and join answer what happens.

Native finite iteration is explicit. Ordinary `ForEach` remains sequential and
fail-fast. Parallelism and collect-all behavior must be authored:

```tsx
ForEach("platform", [Linux(), Windows()], platform =>
  On(platform, Test()),
  {
    mode: ParallelForEach({ concurrency: 2 }),
    failure: CollectAll(),
  },
)
```

`ForEach` accepts 1 to 256 statically materialized string, number, boolean, or
platform values. Parallel concurrency is 1..32. Lowering emits one cursor node
per source item with source identity, index, count, binding, value, mode,
concurrency, failure policy, and aggregate identity. The executor starts the
first bounded window and each completed slot admits the next source item;
completions ready together are applied by stable iteration identity.
Aggregate elements are ordered value identities, not copied result payloads.
`CollectAll()` records `succeeded`, `failed`, `cancelled`, or `timedOut` for
every element and attempts all source items. A collect-all result is an
immutable `WorkflowAggregate<WorkflowIterationOutcome<T>>`: its typed `length`,
constant indexes, and elements are available to another `ForEach`. Indexes are
validated during manifest evaluation; invalid indexes never become JavaScript
`undefined`.

```tsx
const runs = ForEach("platform", [Linux(), Windows()], platform =>
  On(platform, Test()), {
    mode: ParallelForEach({ concurrency: 2 }),
    failure: CollectAll(),
  });

Sequence(
  runs,
  MatchResult(runs[0], { /* the normal four outcome arms */ }),
  ForEach("run", runs, run =>
    MatchResult(run, { /* the same normal outcome algebra */ })),
)
```

Fail-fast fan-out is deliberately a partial aggregate: inspection retains its
observed prefix, but authoring cannot index or iterate it. Use `CollectAll()`
when downstream aggregate consumption is required.

Sequential fan-out may be nested. Flow schema v4 records hierarchical cursor
paths and rejects plans above 4,096 total materialized iteration instances;
the 256-item limit still applies independently to every loop. Nested results
are never implicitly flattened. Nested `ParallelForEach` is currently rejected
until the scheduler has an explicit non-multiplicative concurrency law.

Typed successful-result facts use serializable builders rather than expression
strings or runtime callbacks:

```tsx
const audit = Audit();

Sequence(
  audit,
  When(
    GreaterThan(audit.failing, 0),
    Process("report", { command: ["report"] }),
  ),
)
```

M78 supports numeric `GreaterThan`/`LessThan`, collection-metadata
`NotEmpty`/`IsEmpty`, and bounded `And`/`Or`/`Not`. Guards read already-produced
snapshot values. Missing values are errors, never accidental false values.

M74/M75 `jobs`, `needs`, `steps`, and `matrix` declarations remain accepted as
compatibility authoring. They lower to the same Flow IR and executor. Jobs are
not a fundamental control construct; they survive as legacy placement/result
grouping. Matrix remains deterministic declaration-time compatibility sugar;
new workflows should use `ForEach`, `On`, and an explicit `Parallel` when
concurrency is intended.

Migration is structural rather than provider-specific: matrix job outputs map
to a collect-all `ParallelForEach` aggregate, `needs.foo.outputs.x` maps to a
typed result projection, matrix post-processing maps to `ForEach` over the
collected aggregate, and nested matrices map to bounded nested `ForEach`.

Values are classified as control, small serialized, artifact reference,
region-local, or placement values. Artifact and region-local values cannot
cross execution regions implicitly. `Transfer(build.artifacts, Windows())` is
an explicit semantic effect which stages authorized workspace files into a
bounded content-addressed local transport area, verifies SHA-256 integrity, and
produces a new artifact reference owned by the target region. It accepts at
most 64 regular, non-symlink files of at most 256 MiB each. Paths must remain
inside the workspace. Transport metadata contains no environment or secret
material. Provider artifact publishing, caches, and deployment are distinct
concepts. Values from parallel branches become available after the
deterministic join; no race selects a winner.

General cycles are rejected with `TSPACK_WORKFLOW_CYCLE_UNBOUNDED`. Future
retry/loop syntax must mark a cycle explicitly and provide a bound. Validation
also rejects duplicate or unknown identities, unknown transition targets,
ambiguous event transitions, unhandled effect outcomes, unreachable nodes,
unknown regions, and joins with no branches.

## Commands and provider boundary

```text
tspack workflow list
tspack workflow inspect CI
tspack workflow inspect CI --json
tspack workflow run CI --jobs 4
tspack workflow run CI --json
tspack workflow export github CI
tspack workflow export github CI --check
```

Human inspection prints the flow program and edges. JSON inspection prints
normalized Flow IR rather than the legacy jobs list.

GitHub remains a thin runner invoking:

```text
tspack workflow run CI --ci-provider github
```

Generated YAML owns triggers, checkout, TSPack setup, runner mechanics, and
provider secret spelling. It does not reimplement the Flow graph as a GitHub
DAG. Core Flow data contains no GitHub, GitLab, YAML, `${{ }}`, `needs`, or
`runs-on` concepts.

Migration is direct: legacy `matrix` becomes `ForEach`, upload/download used for
in-workflow movement becomes `Transfer`, a successful-result `if` becomes typed
`When`, a status condition becomes `MatchResult`, provider `always()` becomes
`Finally`, and a named output string becomes a typed effect result, aggregate,
or projection. Provider adapters do not change the core meaning.

For the complete M78 laws, limits, and boundaries, see
[`docs/dev/m78-typed-fanout-artifact-transport.md`](dev/m78-typed-fanout-artifact-transport.md).
