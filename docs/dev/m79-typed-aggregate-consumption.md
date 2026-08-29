# M79: typed aggregate consumption and bounded nested fan-out

M79 upgrades Flow to schema version 4. It promotes M78's ordered aggregate
definition into a narrow immutable authoring value without adding a JavaScript
array runtime or a second collection representation.

## Representation and type law

M78 already stored an aggregate as metadata plus an ordered list of stable
effect-result identities. `Snapshot.Values` owns each payload exactly once;
`Snapshot.Aggregates` only records element identity, source index, and observed
outcome. M79 preserves that representation.

The TypeScript surface is `WorkflowAggregate<T, Complete>`. Its deliberate
operations are `length`, a statically known integer index, and complete-
aggregate `ForEach`. There is no mutation, sparse storage, dynamic length,
dynamic workflow-value index, `map`, `filter`, `reduce`, sorting, or implicit
flattening. A callback returning `On(platform, Test())` preserves `Test` as the
element type. `CollectAll()` wraps it as
`WorkflowIterationOutcome<WorkflowTestEffect>`; fail-fast produces a partial
aggregate that cannot be consumed.

Constant indexing validates the target, completeness, integer value, sign, and
upper bound in the manifest frontend. It returns the original result identity
annotated with aggregate identity and source ordinal. Flow match nodes expose
this `projection` record; no result payload is copied.

Second-pass `ForEach` is still static lowering. Its item source is an aggregate
element reference, its cursor records `sourceAggregate`, `elementIdentity`, and
`path`, and its body consumes the original value identity in source order.
`MatchResult` reuses `succeeded`, `failed`, `cancelled`, and `timedOut`; the
successful arm keeps precise result projections and the other arms keep the
existing failure evidence types.

## Nested expansion and concurrency

Sequential fan-out may nest. The frontend namespaces every statically expanded
inner loop and Flow records deterministic hierarchical paths. Inner aggregates
remain distinct and are not flattened. A first-class outer
`WorkflowAggregate<WorkflowAggregate<T>>` return value is deferred; nested
execution itself does not fabricate a flattened aggregate.

The schema records one explicit cost model: the number of materialized
iteration instances across the authored finite IR. Every loop retains the
independent 1..256 source bound. The workflow-wide default is 4,096. Counting
uses unsigned overflow checks and reports the calculated total and limit before
Flow nodes are lowered. Complexity is proportional to permitted expansion;
payload sizes do not affect it.

Nested parallel fan-out remains deliberately unsupported in M79. M78's local
fork windows would compose multiplicatively without a scheduler-aware law.
Until that law exists, nested `ParallelForEach` receives
`TSPACK_WORKFLOW_NESTED_PARALLEL_UNSUPPORTED`. The executor worker limit remains
the global runtime authority, local fan-out concurrency remains 1..32, and
schema inspection exposes that ceiling.

## Inspection, snapshots, and compatibility

JSON inspection exposes schema v4 expansion metadata, aggregate completeness,
aggregate-element match projections, aggregate-source cursors, and hierarchical
paths. Human inspection prints the same relationships. Snapshots retain the
existing reference-only aggregate values and explicit cursor records; trace
remains bounded and does not include result payload dumps.

Artifact region ownership is unchanged. An element projection does not make an
artifact portable; successful branches use the existing typed artifact
projection and `Transfer`. Aggregate transport needs no special effect.
Existing predicates compose with successful result facts.

Legacy matrix workflows still lower through the same Flow executor and are not
rewritten into exposed aggregates. Migration guidance is:

```text
matrix job outputs       -> collect-all ParallelForEach aggregate
needs.foo.outputs.x      -> typed result projection
matrix post-processing  -> ForEach(collectedResults, ...)
nested matrix            -> bounded nested sequential ForEach
```

The dedicated `fixtures/workflow-m79` workflow demonstrates collect-all,
constant indexing, a second ordered pass, normal outcome matching, typed
predicates, nested 2x2 sequential fan-out, `Finally`, and artifact transport
composition. Root CI remains intentionally unchanged.
