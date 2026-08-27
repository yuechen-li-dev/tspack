# M69a dependency authoring IR and ordered tape

## Status

M69a introduces a dependency authoring domain between the manifest frontend and
the existing graph/resolver path. It deliberately does not change registry
selection, graph resolution, or `ts-lock.toml`.

The three dependency truths are now explicit:

1. Authoring IR records every declaration and its provenance.
2. The dependency tape orders declarations, records shadowing, and projects the
   winners.
3. The resolved graph and lockfile record exact selected package instances.

The resolved graph is not used as an editable authoring model.

## Pipeline before M69a

The frontend evaluated `manifest.tsx` or each referenced
`package.manifest.tsx`, mapped dependency helper values directly to a package's
normalized `dependencies` array, and discarded the `defineDeps` authoring
object after extracting dependency reference keys. `internal/manifest`
validated that final array. `internal/graph` then indexed it by dependency key,
and `internal/project` passed npm dependencies from the graph to the existing
resolver. The resolver produced `ts-lock.toml` and materialization inputs.

Dependency declarations could originate from:

- inline project packages in `manifest.tsx`;
- full `package.manifest.tsx` contracts in split workspaces;
- `dep`, `peer`, and `tool` helper values, including npm, git, path, and
  workspace sources;
- built-in and local concepts merged by `internal/concepts` and projected into
  generated template TSX;
- built-in and local templates;
- package annotation manifests used during incremental adoption;
- observed package.json `dependencies`, `devDependencies`,
  `peerDependencies`, and `optionalDependencies`;
- target and `<Tools>` references, which select declared dependencies but do
  not create new package requirements;
- transitive resolver metadata, which is resolved truth rather than authoring
  intent.

Before M69a, manifest source path, concept/template ownership, declaration
order, shadowed requests, and package.json observation authority were gone by
the time graph construction began. Concepts also rejected differing duplicate
dependency contributions during merge, so their priority was diagnostic rather
than an editable declaration history.

## New ownership and flow

`internal/authoring` owns the durable authoring vocabulary, tape transform, and
pure edit operations. It does not import project, graph, resolver, lockfile, or
CLI packages.

```text
manifest.tsx / package.manifest.tsx / concept or template producer
        |
        v
manifest frontend dependencyAuthoring declarations
        |
        v
internal/authoring.PackageIR
        |
        v
ordered TapeResolution
        |                   \
        |                    -> provenance, shadowing, conflicts, edit results
        v
[]EffectiveDependency
        |
        v
existing manifest validation -> graph -> resolver -> ts-lock.toml
```

Legacy normalized IR without `dependencyAuthoring` remains accepted. Manifest
validation synthesizes generated, derived declarations from its existing
`dependencies` array, builds a tape, and projects the same effective values.
This keeps fixtures and embedding callers compatible while giving every loaded
package an inspectable tape.

## Authoring IR

`DependencyDeclaration` captures:

- an optional stable declaration ID and dependency-reference key;
- a `PackageIdentity` containing source kind and source-local name;
- a complete `PackageSource` and separate requested constraint;
- TSPack dependency kind and optional semantics;
- declaration origin;
- precedence layer, layer order, and declaration order;
- authority and editability.

`PackageSource` is shared by the normalized effective dependency shape. The
current manifest validator still accepts npm, git, path, and workspace source
kinds only. The identity model itself is not npm-only: `npm:foo` and
`other:foo` are distinct identities. Future JSR work can add a source kind and
backend validation without changing tape identity.

## Provenance

Origins are typed as project manifest, package manifest, concept, template,
compatibility observation, generated/default, or explicit user operation. An
origin may include a semantic name, project-relative source path, and a stable
reference.

The frontend emits project or package manifest provenance by default. Split
workspace declarations carry the package manifest path from the root package
row. Dependency helper options can provide explicit declaration metadata.
Generic concept templates write concept ownership into those options. The
built-in static, React, and React-library renderers mark their dependency set as
template-generated. Package.json adoption reports use compatibility origins.

The model retains semantic identity, not source ASTs. It is therefore suitable
for explanations without turning evaluated TypeScript syntax into the runtime
data structure.

## Precedence and load order

Tape order is deterministic. Declarations sort by:

1. layer rank;
2. explicit layer order;
3. declaration order;
4. stable declaration ID as a final tie-breaker.

Layers apply from lower to higher precedence:

```text
base -> concept -> template -> project -> package -> compatibility -> explicit
```

Later tape entries win for the same source-qualified package identity and
effective dependency key. Distinct explicit aliases for the same source remain
separate effective declarations, preserving existing target-reference
semantics.
Compatibility is the authoritative final observed layer only in incremental
adoption reports; it is not inserted into native TSPack project resolution.
Explicit user edit declarations remain above every producer.

M60 concept stack behavior is preserved: the first listed concept is highest
priority and there is no hidden insertion. Concept declarations assign lower
priority concepts earlier tape positions so the first listed concept appears
later and wins. Reversing the listed concepts reverses that result.

## Shadow and conflict semantics

Every declaration receives a tape entry. A shadowed entry remains present and
points to the later entry that shadows it. The tape classifies transitions for
the same source-qualified identity and effective dependency key as:

- equivalent duplicate;
- constraint override;
- dependency-kind override;
- optional-semantics override.

These are explicit non-fatal authoring decisions. The later declaration is the
effective projection.

Different source-qualified identities are retained independently even when
their textual names match. If two distinct identities would project to the
same graph dependency key, the tape reports a fatal key collision instead of
letting a name-only map silently choose one. Authors can use distinct explicit
keys once the relevant source backend exists.

Concept merge continues to reject differing duplicate contributions at its
existing projection boundary. Every `MergedConceptIR` carries
`DependencyAuthoring` lowered from the pre-merge fragment stack, so identical
duplicates and their provenance survive even when the generated manifest
projection needs one declaration. `concepts.DependencyDeclarations` is also
available directly for conflict diagnostics and future template/edit workflows.

## Effective projection and resolver compatibility

`TapeResolution.Effective` contains one winner per source-qualified identity
and effective dependency key.
During manifest validation it replaces the package's normalized dependency
array before the existing dependency, target-reference, graph, and resolver
validation runs. Neither `internal/graph` nor `internal/resolver` interprets
authoring precedence.

For legacy or non-overlapping declarations, the projected effective values have
the same key, kind, source fields, requested range, and optional flag as the
pre-M69a dependency array. Graph sorting and resolver selection remain
unchanged. Authoring data is not written to `ts-lock.toml`; the lockfile remains
resolved truth.

## Dependency kinds

The authoring IR uses TSPack's current semantic kinds: general/runtime `dep`,
`peer`, `tool`, and the normalized reserved `runtime`, `type`, `test`, and
`workspace` values already accepted by the manifest/graph boundary. Optional
is an orthogonal flag. The model does not copy npm dependency-section taxonomy
as a universal package model.

Incremental package.json observation maps section signals as follows:

| package.json section | TSPack authoring kind | optional |
| --- | --- | --- |
| `dependencies` | `dep` | no |
| `devDependencies` | `tool` | no |
| `peerDependencies` | `peer` | no |
| `optionalDependencies` | `dep` | yes |

This is an observation/classification view, not a claim that TSPack owns those
sections.

## Authority and editability

Authority is `owned`, `observed`, or `generated`. Editability is `editable`,
`derived`, `observed`, `generated`, or `concept-owned`.

- ordinary native manifest declarations are owned and logically editable;
- concept declarations are generated and concept-owned;
- built-in template defaults are generated;
- legacy normalized IR declarations are generated and derived;
- package annotation declarations are owned classification evidence but
  derived in incremental mode;
- package.json declarations are observed and non-editable.

An edit operation can require `EditableOnly`, so future commands cannot
silently remove package.json observations, generated template facts, or concept
internals. Source projection must still prove that a logically editable
declaration has a safe owned source island before rewriting TSX.

## Tape edit API and removal

The pure edit API provides add, remove, replace, and constraint-change
operations. It accepts a package authoring IR and returns before/after tape
resolutions plus semantic changes. It never mutates a graph or lockfile.

Selectors can identify a declaration by ID, source-qualified identity, kind,
origin kind/name, source path, and editability. Zero matches is not-found;
multiple matches return a typed ambiguity with every matching declaration.
There is no arbitrary first-match removal.

`Add` defaults to an owned, editable explicit-user layer. Removing an effective
declaration rebuilds the tape. If an older declaration becomes effective, the
result includes an `unshadowed` change. Thus removing an explicit
`typescript@^6` can naturally reveal the concept-owned `typescript@^5.9`
without claiming that TypeScript disappeared from the project.

The change vocabulary supports added, removed, changed, shadowed, and
unshadowed results. Terminal text and JSON command DTOs are intentionally not
part of this domain package.

## Incremental adoption

`adoption.Report` now includes a dependency authoring tape. Package annotation
declarations appear first as derived package-manifest evidence. Sorted
package.json observations appear in the compatibility layer and win when both
describe the same package, preserving the existing rule that package.json is
authoritative during incremental adoption. The report does not rewrite
package.json or feed these observations into native resolution.

## Determinism and performance

Pure tests cover ordered shadow chains, reversed concept order, removal and
unshadowing, ambiguous removal, editability filtering, source-qualified
identity, fatal projected-key collisions, and repeated randomized input
construction. The frontend preserves source array order and split-manifest
paths. The tape transform is an in-process sort and linear projection; it does
not invoke Node or start a new frontend worker. A benchmark covers a
100-declaration tape.

## Projection and later M69 milestones

M69a provides the semantic edit result needed by later commands but does not
perform source projection. Arbitrary TSX is not reconstructed from evaluated
IR and no regex editing was added.

The intended canonical target is a TSPack-owned dependency declaration island
using the existing `defineDeps` plus package `dependencies.values` syntax. The
frontend now accepts `dependencyDeclaration` defaults on `Package` and
per-dependency `declaration` metadata, which gives generated templates a safe
way to state provenance. M69b still needs a source-preserving projector for a
clearly delimited owned island, including idempotence and diff tests.

Roadmap status:

- M69b canonical projection: enabled, not implemented;
- M69c `tspack add`: semantic add/edit result complete, CLI and projection
  remain;
- M69d `tspack remove`: selector, ambiguity, remove, remains-required, and
  unshadow semantics complete; CLI and projection remain;
- M69e ergonomics: not started.

No `add` or `remove` command is exposed yet because doing so without a safe
source projector would either rewrite arbitrary user code or pretend an IR-only
change persisted.

## Deferred M70+ directions

- JSR and a general multi-registry resolver should add source/backend support
  behind `PackageSource` and keep source-qualified identities.
- Alternate TypeScript compilers and richer `tsconfig.tsx` should follow the
  same frontend -> normalized semantic IR -> backend pattern; they do not
  belong in the dependency tape.
- Authoring provenance should remain outside the lockfile unless a separate
  resolved-provenance design is explicitly justified.
- Filesystem transactions, public embedding APIs, plugin architecture, and a
  graph/lock IR redesign were not required for M69a.
