# M77: typed workflow values, match, cleanup, and finite iteration

M77 extends the M76 Flow machine without adding a value system, scheduler, or
executor. The existing `Snapshot.Values` store remains authoritative. Flow
schema version 2 describes the definitions and control nodes that make those
values usable from authoring.

## Value model

An effect call is inert manifest data with a deterministic source identity.
Lowering combines the workflow identity, effect source identity, result slot,
and optional field path:

```text
value/ci/effect/218
value/ci/effect/218.artifacts
```

Every identity has one producer. Results and projections are immutable. A
projection definition points at its source identity and field path; it neither
copies the structured result nor converts it to a dictionary. Build, Test, and
Audit retain the typed `internal/project` operation results introduced in M75.

The frontend result fields exactly follow those Go models. In particular, the
Audit model currently exposes `source`, `auditLevel`, `failing`, `report`, and
`diagnostics`; M77 does not invent `findings`, `coverage`, or `blockingCount`.

Values live for one workflow run and are reachable only downstream of their
producer. The snapshot owns each structured result once. Projection and trace
records retain identities and bounded metadata. Artifact collections contain
artifact references, never artifact bytes.

## Placement law

Every value definition has a category:

- `control`: small scalar control fact;
- `smallSerialized`: bounded structured evidence;
- `artifactReference`: artifact metadata/reference;
- `regionLocal`: a complete runtime result;
- `placement`: a statically bound placement value.

Control and small serialized facts have a semantic serialization path.
Artifact references and region-local runtime values may not cross regions in
M77 because artifact transport is not implemented. Validation reports
`TSPACK_WORKFLOW_VALUE_REGION_ILLEGAL`; core Flow never substitutes GitHub
outputs or provider files.

Parallel branches retain distinct value identities. At an `all` join, every
completed branch value is available by identity in declaration order. There is
no mutable merge, global bag, or completion-time winner.

## MatchResult

`MatchResult(effect, arms)` accepts exactly `succeeded`, `failed`, `cancelled`,
and `timedOut`. All are required. The success callback sees the effect-specific
result reference; other callbacks see typed failure evidence. Expression
callbacks execute only in the purity-restricted frontend to construct plain
records. No closure survives in manifest IR or Flow.

Lowering emits a `match` node whose source is the effect value identity and
four `continue` transitions guarded by closed outcome kinds. The effect's
outcome is recorded independently of whether it has a structured payload. A
matched effect inside a parallel branch still reaches its branch exit; the
match occurs after the join. Ordinary `Sequence` keeps default failure,
cancellation, and timeout propagation unless the user explicitly matches a
result. One value may be matched once in M77.

Build, Test, and Audit use the same generic lowering. Matching semantic facts
inside a successful result is deferred; M77 discriminates outcome kind only.

## Finally

`Finally(body, cleanup)` is authoring sugar over four visible cleanup paths.
The compiler copies cleanup structure for body success, failure, cancellation,
and timeout and labels every cleanup effect with its cause. No Go `defer`,
provider `always()`, or hidden exception mechanism defines semantics.

The result law is deterministic:

- body success + cleanup success: success;
- body success + cleanup failure/timeout: cleanup outcome fails or times out;
- body failure + cleanup success: preserve body failure;
- body cancellation/timeout + cleanup success: preserve cancellation/timeout;
- any failed cleanup after a non-success body: terminal failure with the body
  cause and cleanup failure both retained in the snapshot.

Cancellation of a running body routes to cleanup. Cleanup uses
`context.WithoutCancel` plus a 30-second bound, unless the cleanup step has its
own shorter timeout. Hard process termination remains outside graceful
cancellation. Nested cleanup is compositional and limited to eight levels.

## ForEach

M77 accepts finite, statically materialized arrays containing 1..256 values.
The callback receives the actual TypeScript item type and is evaluated once per
item as inert IR construction. Each expansion is namespaced, so repeated body
syntax produces distinct SSA identities.

Lowering is sequential and fail-fast. One explicit iterator node per item
records source identity, zero-based index, total count, bound value identity,
value, `mode: sequential`, and `failurePolicy: failFast`. Results therefore
remain ordered by source index. There is no generator suspension, hidden
continuation, matrix scheduler, or goroutine fan-out. An ordered aggregate
result and explicit bounded parallel mode are M78 seams.

The machine-wide semantic step limit remains 10,000, trace retention remains
256 rows, `ForEach` is capped at 256 items, and `Finally` nesting is capped at
eight. These bounds prevent accidental graph or planning explosion.

## Inspection and provider boundary

Human inspection prints produced values, projections, consumers, guarded match
arms, cleanup causes, and cursor progress. JSON schema version 2 includes
`values`, match nodes, iterator data, cleanup labels, and guarded transitions.
Trace rows include value identity and outcome kind, not full artifacts or test
evidence.

The GitHub export remains a thin runner invoking TSPack. Flow values never
become `${{ needs.*.outputs.* }}`, `$GITHUB_OUTPUT`, condition strings, or
provider-native DAG data.

## Migration

```text
legacy matrix       -> ForEach (+ explicit Parallel if desired)
status condition    -> MatchResult
always() cleanup    -> Finally
named string output -> typed result reference or projection
```

Legacy jobs, needs, matrices, M76 `Sequence`/`Parallel`, and the single Flow
executor remain compatible. Compiler-specific Build implementation placement
is unchanged because M77 did not require moving it.

## M78 seams

M78 should add an ordered typed `ForEach` aggregate, explicit bounded parallel
iteration, optional collect-all failure policy, semantic artifact transport,
and successful-result fact matching. Those additions should reuse the schema-v2
identities, cursor records, and guarded transitions rather than create new
runtime concepts.
