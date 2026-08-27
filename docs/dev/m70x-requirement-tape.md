# M70x requirement tape and deterministic peer/alias resolution

## Status

M70x introduces a dependency-intent IR for shared environment requirements.
The model is deliberately analogous to compiler IR/SSA: hidden mutable resolver
state becomes explicit evidence, every declaration has an identity and origin,
and a deterministic reduction pass selects one controlling definition before
downstream version selection or materialization starts.

This closes the M70c peer and npm-alias model blockers without introducing a
SAT solver, global constraint intersection, graph backtracking, or
network-completion policy.

## Before-state model

Before M70x, the normalized registry backend emitted runtime and optional
dependencies only. npm metadata decoded `peerDependencies` but discarded it.
Project-authored peers entered the same resolver work queue as ordinary edges,
and every committed lock edge was traversed by the Node materializer. These
facts conflated two different concepts:

```text
ownership edge: package A owns/uses package B
requirement:    package A asks its environment to provide package B
```

The npm adapter also treated a dependency map key as package identity and sent
an alias value such as `npm:bar@^2` to the SemVer parser. It had no place to
represent `reference foo -> semantic target npm:bar`.

## IR and passes

`internal/requirements` owns the normalized IR:

```text
PackageRequirement
  ID
  Target { Source, Name }
  Reference
  Constraint
  Kind / Optional
  Origin
  Scope
  Order
```

The resolver pipeline is now:

```text
parallel metadata/artifact facts
  -> source-qualified PackageRequirement values
  -> stable Requirement Tape build
  -> one controller per slot
  -> existing SemVer version selector
  -> classification against the selected version
  -> lock/explanation DTOs
```

The backend still receives one constraint at a time. It does not see or solve
the losing constraint set.

## Resolution slots and scope

The implemented slot key is `workspace environment + source-qualified package
identity`. Thus `npm:foo` and `jsr:foo` are different slots. Filesystem ancestry
and hoisting are not peer scope. Ordinary transitive runtime edges are not
placed on this shared tape and may continue to resolve as isolated package
instances. Direct workspace/package dependency declarations and peers
participate in the workspace environment because the current materializer
exposes those roots in one workspace `node_modules` environment.

## Precedence

Precedence is a property of the IR, not discovery timing:

```text
transitive runtime
transitive optional
peer
package explicit
project explicit
explicit override
```

Within one rank, larger stable `Order` wins. ID is the final deterministic
tiebreaker. Resolver workers gather facts concurrently, while serial commit
assigns requirements from stable source/name/range/from/kind ordering. This is
the existing “parallelize facts, serialize truth” rule.

The current manifest needs no separate override syntax. An explicit direct
dependency already controls its shared slot. M69 add/remove changes the
controller naturally: adding a direct declaration shadows peers; removing it
lets the next tape entry control.

## Classification and severity

After selection, entries are classified as `controlling`,
`shadowed-compatible`, `overridden-incompatible`, `optional-unsatisfied`,
`invalid`, or `pending`.

An incompatible losing peer emits warning
`TSPACK_PEER_REQUIREMENT_OVERRIDDEN`. It names the requiring package, target,
constraint, selected version, and controlling requirement. Required controller
lookup/selection failures retain the resolver's error policy; optional failures
remain warnings.

## Peer semantics

Registry `peerDependencies` and `peerDependenciesMeta.optional` are normalized
for npm and JSR compatibility metadata. A transitive peer creates or evaluates
the workspace environment slot. It never creates
`package/node_modules/peer` ownership. The lock uses a
`workspace:peer:<source>:<name>` root edge for the selected environment
provider, while provenance and constraints live in requirement records.

JSR compatibility keys beginning with `@jsr/` become JSR peer identity;
ordinary keys become npm peer identity. A same-text package from the other
source cannot satisfy the slot.

Requirements discovered from a selected package remain evidence on the tape
even if a later controller makes that earlier package unreachable. This
monotonic fact model avoids graph backtracking: superseded evidence remains
explainable, while unreachable package and ownership entries are pruned from
installed graph truth.

## Reference and alias identity

Registry metadata `"foo": "npm:bar@^2"` normalizes to:

```text
Reference: foo
Target:    npm:bar
Range:     ^2
```

Version resolution, lock package IDs, audit identity, and requirement slots use
`npm:bar`. The edge carries `foo`, and Node materialization places the target at
the `foo` path. Regular npm keys and JSR compatibility keys continue to use
their canonical Node spelling.

The materialization plan detects two semantic packages targeting the same path
before creating `node_modules`. Alias-driven collisions report
`TSPACK_ALIAS_REFERENCE_COLLISION` deterministically.

Direct manifest alias syntax is not expanded by M70x. Existing manifest keys
remain dependency-reference identifiers rather than implicit npm aliases;
registry aliases are represented internally and on transitive edges.

## Explanation, check, lock, and audit

`tspack why <reference|name|source:name>` shows requirement origin, kind,
constraint, status, controller, selected version, and alias target. JSON uses
explicit DTO fields. `tspack check` replays incompatible requirement warnings
in stable order.

The lock format version remains `1`, with additive optional `[[requirement]]`
records and edge `reference` fields so offline `why`, `check`, and `sync` need no
registry access. Installed package truth remains `[[package]]`. Audit scans only
selected lock packages; losing requirements are not installed artifacts.

## Complexity and guardrail

Tape build is sorting plus linear slot reduction: `O(N log N)` time and `O(N)`
space. Classification is linear. Benchmarks cover 100, 1,000, and 10,000
requirements. Tests protect the semantic guardrail: conflicts choose the
explicit or later same-rank tape entry directly, without intersection search,
global downgrade, or retry combinations.

## Test and real-metadata matrix

Deterministic fake registries cover peer `^18` versus `^19`, project override
and removal/unshadow, npm and JSR same-name separation, JSR-to-npm and
JSR-to-JSR peers, normalized npm-source-to-JSR peers, optional peers, npm aliases, two aliases to one target,
alias collisions, and reversed metadata completion delays.

The August 2026 metadata sanity matrix used published packages:

- `react-dom@18.3.1` peer `react ^18.3.1`;
- `react-dom@19.1.0` peer `react ^19.1.0`;
- `react-redux@9.2.0` peers `react ^18 || ^19`, `redux ^5`, and matching
  `@types/react` ranges;
- `@types/react-dom@19.1.5` peer `@types/react ^19`;
- JSR `@luca/flag@1.0.1` remains the real compatibility-artifact smoke package.

Published metadata is evidence that the normalized fields match real registry
shapes; deterministic conflict policy stays in controlled fixtures so tests do
not depend on mutable network responses.

## M70 closeout and handoff

M70c's isolated semantic blockers are closed: registry peers are requirements,
npm aliases separate reference from identity, source-qualified satisfaction is
enforced, and collisions are preflighted. M70d may build source allowlists,
mirrors, preferences, fallback, corporate feeds, and air-gapped policy on the
controlling-requirement boundary. M70x does not implement those policies.
