# M70c cross-registry semantics

## Status

M70c separates TSPack package truth from Node compatibility spelling and closes
the collision, optional-edge, lifecycle, store-provenance, audit-coverage, and
explanation seams that could be closed without redesigning the dependency
solver. M70c originally ended in **meaningful progression** with transitive
registry peers and true npm aliases as isolated blockers. M70x subsequently
closed those blockers with an SSA-like requirement tape; see
`docs/dev/m70x-requirement-tape.md`.

The governing rule is:

```text
semantic package identity
  -> backend/runtime compatibility mapping
  -> materialized package tree and import spelling
```

Compatibility names never become manifest, Authoring IR, dependency-tape,
lockfile, or `why` package truth.

## Before-state inventory

| Semantic seam | Before M70c | M70c disposition |
| --- | --- | --- |
| Semantic package identity | Source-qualified in most authoring/resolver paths, but compatibility mapping was duplicated | Explicit boundary `PackageIdentity { Source, Name }` and one canonical compatibility mapper |
| Runtime/materialization identity | Derived locally by the materializer | Explicit Node usage mapping and pre-write collision detection |
| Import specifier | Documented but not returned as a typed result | `PackageUsage.Import` is reusable by add and why |
| Dependency edge source | Runtime/optional compatibility keys were partly normalized | Ordinary JSR metadata keys route to npm; `@jsr/` keys route to JSR |
| Peer dependencies | Registry response type decoded them, adapters ignored them | Closed by M70x source-qualified environment requirements |
| Optional dependencies | Preserved optionality, with limited cross-source proof | Source-qualified npm, JSR-to-JSR, and JSR-to-npm cases are covered |
| Aliases | `@jsr/scope__pkg` dependency keys were treated as JSR names | M70c covered compatibility keys; M70x adds true npm reference/target aliases |
| Exports/subpath exports | Relied on compatibility tarball `package.json` | Metadata remains preserved verbatim; no TSPack export rewrite was added |
| Types | Compatibility artifacts carried declarations | Existing NodeNext real-package proof retained |
| Lifecycle/capability metadata | npm captured scripts; JSR returned no capabilities | Controlled scripted JSR metadata proves compatibility scripts do not become capabilities |
| Audit ecosystem identity | npm checked; JSR had a textual not-checked row | Typed checked/unsupported/unknown coverage and whole-lock completeness |
| Why/how presentation | `why` retained source identity but did not explain imports | `why` carries semantic, materialization, and Node import usage; `how` remains diagnostic-code help |
| Check/diagnostics | Source and lock checks existed | Materialization collision has a source-qualified structured diagnostic; audit remains a separate network command |
| Lockfile presentation | Source-qualified package IDs already existed | M70x keeps package identity unchanged and adds deterministic requirement/reference evidence |
| Store provenance | Content deduplicated by hash, with one overwriteable metadata identity | Identical content retains a deterministic set of source-qualified provenances |

## Identity model

`internal/packageidentity` defines three deliberately separate values:

```text
PackageIdentity
  source: jsr
  name: @luca/flag

MaterializationIdentity
  source: npm-compat
  name: @jsr/luca__flag

ImportIdentity
  runtime: node
  specifier: @jsr/luca__flag
```

`PackageUsage` combines these values without terminal prose. npm normally maps
all three names to the same spelling. JSR maps semantic identity through its
npm compatibility name only at the Node/TypeScript boundary.

JSR mapping accepts scoped `@scope/package` names and rejects malformed or
ambiguous compatibility spellings. In particular, names containing the `__`
separator cannot be round-tripped safely and fail rather than being guessed.

## Authoring, lock, and explanation truth

The following surfaces continue to use `jsr:@scope/package`:

- `manifest.tsx` and the `jsr()` helper;
- normalized manifest and Authoring IR;
- dependency tape and edit selectors;
- resolver work, lock packages, and lock edges;
- source-qualified diagnostics and `why` queries.

`tspack add @luca/flag --source jsr` returns typed usage information. Human
output adds the non-noisy guidance `Import in Node/TypeScript as:
@jsr/luca__flag`; JSON includes the same semantic usage object. npm does not
print redundant guidance when its semantic and import names match.

`tspack why jsr:@luca/flag` and `why --json` retain the semantic lock package
and additionally show its npm-compat materialization and Node import spelling.
`how` remains the existing diagnostic-code explanation command rather than a
second package-consumption command.

## Same-name packages and collisions

The normal same-logical-name case remains distinct throughout the graph:

```text
npm:@scope/foo -> node_modules/@scope/foo
jsr:@scope/foo -> node_modules/@jsr/scope__foo
```

Authoring replacement and removal select by source plus logical name.
Unqualified removal is ambiguous when both sources are declared; `--source
jsr` removes only JSR intent. Adding a new JSR constraint replaces only the JSR
declaration.

A different collision is possible when an actual npm package already owns the
compatibility name, for example `npm:@jsr/scope__pkg` beside
`jsr:@scope/pkg`. The materializer now builds and validates its complete plan
before touching `node_modules`. It fails deterministically with
`TSPACK_MATERIALIZE_IMPORT_COLLISION`, the destination, and every
source-qualified package ID instead of silently overwriting either package.

## Dependencies, optional edges, peers, and aliases

JSR compatibility dependency keys normalize as follows:

```text
@jsr/scope__child -> jsr:@scope/child
ordinary-name     -> npm:ordinary-name
```

The rule applies to runtime and optional dependency maps. Optional edges retain
both `Optional = true` and their source-qualified identity. Existing TSPack
optional failure behavior is unchanged; M70c does not broaden it.

Compatibility names with no separator, empty components, extra separators, or
nested path shapes fail with an error that names the source-qualified parent.
Underscores that do not make the `__` separator ambiguous round-trip normally.

At M70c close these were the two explicit limitations: registry peers were
decoded but not emitted, and true npm alias constraints were not parsed into
reference, source, name, and constraint. M70x closes both source-qualified; an
`npm:foo` peer cannot be satisfied by `jsr:foo` merely because the logical text
matches.

## Exports, types, and module modes

TSPack extracts and materializes the compatibility artifact without rewriting
its `package.json`, JavaScript, declarations, `exports`, or subpath exports.
Consequently Node/TypeScript behavior follows the artifact published by JSR.
The existing `fixtures/valid/jsr-mixed` proof uses TypeScript `NodeNext` and
resolves real JSR declarations. M70c did not add an import-map or `tsconfig`
path alias, and it does not rewrite application imports.

JSR packages are not assumed to share one ESM/CJS profile. TSPack preserves the
compatibility package metadata and reports the stable Node import spelling;
whether a particular root or subpath supports ESM, CJS, or both remains a fact
of that package. The current real fixture proves its exercised root imports,
not universal CJS support or every published subpath.

## Lifecycle and security semantics

Registry source determines lifecycle capability semantics. npm metadata is
normalized into blocked lifecycle capabilities. JSR compatibility packages do
not inherit npm install-script capability semantics, even if controlled fake
compatibility metadata and its tarball contain `preinstall` or `postinstall`.
Those scripts are neither recorded as JSR install capabilities nor executed.

npm packages reached transitively from JSR remain npm packages and retain npm
capabilities. `update`, store capture, `sync`, and materialization continue to
fetch/read/copy only; none grants package code ambient execution.

## Audit coverage and provenance

OSV queries use only exact locked npm name/version pairs. Audit coverage now
records the whole locked package count, checked npm count, per-source coverage,
and whether coverage is complete:

```text
npm: checked
jsr: unsupported-ecosystem
other unmapped source: coverage-unknown
```

The human report says that no vulnerabilities were found **in checked
packages** when coverage is incomplete. JSR compatibility names are not sent
to OSV as npm identity, because correspondence between a JSR package and an npm
vulnerability record is not trustworthy merely because Node consumes an npm-
shaped artifact.

The native JSR registry API can expose normalized exports, yanked state, linked
GitHub repository data, immutable per-file checksums, and a module graph. The
npm compatibility endpoint already supplies the Node-ready tarball, tarball
integrity, dependency metadata, repository metadata when linked, declarations,
and exports-aware `package.json` required by TSPack's current materializer. It
can briefly lag native metadata, but switching or adding a native lookup would
add requests without fixing a current Node correctness bug.

JSR can publish GitHub Actions provenance through SLSA/Sigstore, but current
JSR documentation describes signed package manifests and npm-compatibility
tarball attestations as future work. A linked repository or provenance page is
useful inspectability evidence, not a vulnerability-ecosystem mapping or a
verified equivalence for the compatibility tarball. M70c therefore keeps the
compatibility endpoint and records native provenance as future work.

Authoritative JSR references:

- <https://jsr.io/docs/api>
- <https://jsr.io/docs/npm-compatibility>
- <https://jsr.io/docs/trust>

## Store and lock stability

The store continues to address bytes by SHA-256 and may deduplicate identical
content. Store metadata now retains every source-qualified package provenance
for a shared hash. The provenance set and canonical compatibility fields are
sorted deterministically, so insertion order produces byte-identical metadata.
Older singular metadata remains readable and is folded into the provenance set
on the next write.

The lockfile remains the authoritative graph provenance. Its format did not
change, IDs and edges remain source-qualified, and compatibility package names
do not replace logical names. Existing repeated mixed updates remain
deterministic.

## Evidence matrix

Real-package evidence remains in `fixtures/valid/jsr-mixed`:

| Package | Version | Evidence |
| --- | --- | --- |
| `jsr:@luca/flag` | `1.0.1` | simple leaf and Node compatibility import |
| `jsr:@std/path` | `1.1.6` | declarations/exports and TypeScript NodeNext root plus `./basename` subpath imports |
| `jsr:@deno/esbuild-plugin` | `1.2.1` | JSR-to-JSR and JSR-to-npm transitives |
| `npm:picocolors` | `1.1.1` | npm root beside JSR roots |

The mixed lock records 5 JSR and 28 npm packages. Its application code imports
the `@jsr/...` Node spellings, and the established dogfood run required Node,
TypeScript, and TSPack but no Deno executable.

Controlled tests cover:

- same logical npm/JSR names without collision;
- an npm package colliding with a JSR compatibility destination;
- malformed and ambiguous JSR compatibility names;
- JSR optional JSR and optional npm edges;
- an npm optional edge beside a same-name JSR package;
- compatibility metadata containing lifecycle scripts;
- identical content with distinct npm/JSR store provenance;
- mixed audit coverage and source-qualified `why` usage.

Transitive registry peers and true npm aliases were absent from the original
M70c matrix. Their deterministic fake-registry coverage belongs to M70x.

## Performance and request behavior

Identity and compatibility mapping are pure string/value operations and add no
registry request. Add still shares its source-qualified metadata memo across
selection, preflight, and commit. A simple add retains the M70b one-metadata,
one-artifact shape; an equivalent no-op performs zero requests. `why` derives
usage from the lock locally, materialization derives it while planning, and
audit does not contact JSR merely to explain missing coverage.

No native JSR metadata request, source scan, mirror, or fallback was added.
Existing `npm.metadata`, `npm.tarball`, `jsr.metadata`, and `jsr.tarball`
performance attribution remains intact.

## M70d handoff

The following are now stable primitives for source policy:

- explicit source-qualified semantic identities and backend selection;
- a pure semantic-to-Node compatibility mapping;
- source-qualified resolver memoization, lock IDs, edges, and diagnostics;
- deterministic materialization collision failure;
- multi-provenance content-store metadata;
- typed per-source audit coverage.

M70d can build registry allowlists, preferred sources, mirrors, fallback,
corporate feeds, air-gapped caches, and trust policy on those values. It must
not make a compatibility name authoritative, satisfy peers across sources by
name, or treat mirror/fallback selection as permission to change semantic
identity. No mirror or fallback policy is implemented by M70c.

## M70x closeout

M70c originally ended in **Outcome B — meaningful progression**. M70x supplies
the missing source-qualified dependency-spec/satisfaction design: peers lower
to environment requirements, npm aliases lower to reference plus semantic
target, conflicts use explicit tape precedence, and materialization collisions
fail before mutation. The M70c blocker is therefore closed without adding
global constraint solving.
