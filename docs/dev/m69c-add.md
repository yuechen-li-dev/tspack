# M69c first-class `tspack add`

## Status

M69c exposes dependency authoring as a user-facing lifecycle command:

```text
CLI request
  -> package/source/constraint policy
  -> M69a authoring edit and dependency tape
  -> M69b source projection
  -> guarded manifest write
  -> ordinary update/resolution
  -> exact ts-lock.toml truth
```

The CLI does not edit TSX or interpret the tape. `internal/project` owns the
typed request/result and orchestration, `internal/authoring` remains semantic
truth, and `internal/manifestedit` remains the only source mutation planner.

## Package specifications and source policy

M69c accepts `foo`, `foo@^4`, `foo@4.17.21`, `@scope/foo`, and
`@scope/foo@^3`. Parsing treats the final `@` after a scoped package slash as
the constraint delimiter. Arbitrary npm install specs such as git URLs,
GitHub shorthands, file paths, and workspace protocols are rejected.

npm is the default and only public M69c source. The request and authoring IR use
source-qualified identities so later registry work does not need to redesign
identity or tape semantics.

## Constraint policy

An unqualified add fetches npm metadata, ignores prereleases, selects the
highest stable semantic version, and authors `^<selected-version>`. If no stable
release exists, the operation fails before writing and asks for an explicit
prerelease constraint. Explicit ranges and exact versions are validated,
checked for a satisfying published release, and preserved byte-for-byte as
authoring intent.

The default dependency kind is TSPack `dep`. Optional is independent of kind and
is exposed as `--optional`. Tool/dev shorthand is deferred because TSPack tools
also have an explicit `<Tools>` selection surface; adding a declaration without
that selection would promise unusable tooling.

## Existing and derived declarations

An unqualified add matching one editable owned declaration with the same kind
and optional semantics is an immediate no-op: no metadata call, source rewrite,
or lock rewrite occurs. An explicit package spec replaces the matching editable
declaration. Multiple editable matches are diagnosed rather than guessed.

When only a generated, template, or concept-owned declaration exists, add
creates a higher-precedence explicit declaration. The older tape entry and its
provenance remain available and are reported as shadowed. Explicit ownership is
therefore meaningful even when the selected constraint happens to be
equivalent today.

## Package targeting and authority

Exactly one native package is selected automatically. A multi-package
workspace requires `--package <name>`. The frontend carries each native
package's owning manifest path, including split-workspace paths, so project
orchestration never guesses a file from package layout.

Projects with no native package, annotation-only package manifests, dynamic
dependency surfaces, ambiguous islands, and non-owned source shapes fail before
mutation. package.json remains authoritative for incremental projects; the
diagnostic directs users to `tspack npm install`.

## Write safety and update integration

The operation performs semantic validation and source projection before any
write. A non-dry add then atomically writes the guarded projection and runs an
ordinary update dry-run as a resolver preflight. If preflight fails, the
manifest is atomically restored only when it still equals the planned bytes,
which avoids overwriting a concurrent user edit. The commit uses the same
memoizing registry adapter, so metadata and tarballs fetched by selection or
preflight are reused by normal update/store work.

Update still owns graph resolution, store population, lock serialization, and
diagnostics. A package-level normal/runtime declaration that is not selected by
a target is treated as a direct package requirement; target allowlists retain
their existing validation role. If the final lockfile write itself fails, the
manifest is intentionally left authored and the update diagnostic is preserved
so rerunning `tspack update` converges without losing user intent.

`--dry-run` fetches only the metadata needed for version selection, applies the
semantic edit, and plans projection in memory. It does not transiently write the
manifest and does not mutate the lockfile or store. Resolver lock planning is
deferred until commit because the frontend currently evaluates filesystem
manifests rather than an in-memory candidate source.

## Output and remaining milestones

Human output reports package, source, kind, selected exact version, written
constraint, changed files, and shadowed origins. `--json` exposes durable
semantic fields without AST ranges or tape internals.

M69d remove is complete. M69e still includes tool/dev selection, additional
source selection, richer diff polish, and any target-specific authoring
controls. JSR and compiler work remain out of scope.
