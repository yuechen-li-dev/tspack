# M78: typed fan-out, artifact transport, and fact-driven flow

M78 upgrades Flow to schema version 3. It extends the M77 value graph and pure
machine; it does not add a matrix runtime, output channel, provider condition,
or second outcome algebra.

## Ordered fan-out

`ForEach(identity, source, body)` remains source-compatible, sequential, and
fail-fast. `ParallelForEach({ concurrency })` makes parallelism explicit and
requires a limit from 1 through 32. Sources and aggregates remain limited to
256 elements. Each item has a cursor containing its source ordinal, binding,
mode, concurrency, failure policy, and aggregate identity.

The schema-v3 aggregate stores only ordered element value identities. Results
remain owned once by `Snapshot.Values`; large test, build, and audit payloads
are not copied. Snapshot aggregate state adds one closed outcome per observed
element. Source ordinal defines aggregate order even when physical effects
complete out of order.

The executor launches the first source-order window no larger than the declared
limit. Each branch exit explicitly activates item `i + concurrency`, so a free
slot admits the next pending source item without waiting for active siblings.
It sorts ready completions by stable node identity before stepping the machine.
Fail-fast stops later batches after the first semantically observed failure,
timeout, or cancellation. Already-running siblings are allowed to finish; M78
does not introduce a second cancellation option. `CollectAll()` routes every
closed outcome to the batch join, attempts every source item, and exposes an
`iterationOutcome<T>` aggregate. The existing `MatchResult` outcome kinds are
reused. General indexing and a second `ForEach` over aggregate outcomes are not
yet exposed in the bounded TypeScript surface. Nested `ForEach` is rejected to
avoid accidental multiplicative expansion.

## Artifact transport

Build artifacts now carry optional logical identity, SHA-256 content identity,
and origin-region facts. An ordinary artifact projection stays region-owned.
Cross-region validation continues to reject direct use.

`Transfer(artifacts, target)` is a provider-neutral effect. It is the only
schema-v3 effect allowed to consume an artifact reference from another region.
The result is a new build-shaped artifact reference owned by the target region,
so a later `On(target, Pack(portable.artifacts))` is legal. The Flow trace emits
`artifactTransferStarted`, `artifactTransferCompleted`, or
`artifactTransferFailed` events.

The local realization stages regular files beneath
`.tspack/workflow-transport/regions/<target>/<sha256>/`. It then re-hashes the
materialized file before publishing the result. Absolute/traversing paths,
symlinks, directories, more than 64 files, and files larger than 256 MiB are
rejected. Only declared build artifact paths are accepted. Secret values,
credentials, and environments never enter transport metadata.

Same-region consumers continue to use the producer reference directly and need
no transfer. Transport is semantic CI movement, not provider retention, cache,
or deployment. GitHub remains a one-job thin runner and does not lower transfer
to provider upload/download actions.

## Typed facts

`When(predicate, trueFlow, falseFlow?)` lowers to a predicate node with explicit
true and false transitions. M78 predicates are deliberately small:

- `GreaterThan` and `LessThan` for numeric Test/Audit control facts;
- `NotEmpty` and `IsEmpty` for safe collection metadata such as artifacts;
- `And`, `Or`, and `Not`, with maximum depth eight and at most eight children
  per boolean node.

The TypeScript callback-free builders lower to typed predicate records. The
machine deterministically projects an already-produced `NativeResult`; it does
not touch the filesystem, network, or process environment. Invalid field/type
combinations fail TypeScript or manifest/Flow validation. A missing source is a
control-flow error rather than false. Artifact predicates inspect collection
metadata only, never file contents.

## Limits and inspection

The existing 10,000 machine-step budget, 256-row trace cap, and eight-level
`Finally` bound remain. New bounds are 256 fan-out/aggregate elements, 32
parallel effects per fan-out, 64 transported artifacts, 256 MiB per transported
file, and predicate depth eight.

Human inspection prints fan-out mode/concurrency/failure policy, aggregate
element identities, transfer targets/consumers, and readable predicate inputs.
JSON inspection exposes the exact schema-v3 aggregate, cursor, transfer effect,
artifact metadata, and predicate records.

## Migration and compatibility

```text
legacy matrix                  -> ForEach / ParallelForEach
named output strings           -> typed result projections / aggregate refs
upload/download for CI movement -> Transfer
successful-result if expression -> When + typed predicate
status condition               -> MatchResult
always() cleanup               -> Finally
```

Legacy jobs and matrices still lower to the same Flow executor. Compiler Build
placement is unchanged; M78 needed no compiler-specific refactor.
