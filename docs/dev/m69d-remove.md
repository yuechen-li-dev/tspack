# M69d first-class dependency removal

## Status

M69d adds `tspack remove <package>` on the M69a dependency authoring tape and
the M69b source-preserving projector. Removal changes authoring intent first;
normal resolution decides which locked artifacts and edges disappear.

## Application contract

`project.RemoveDependencyRequest` carries project paths, the package selector,
an optional workspace package target, optional kind/optional selectors, and
dry-run state. `project.RemoveDependencyResult` reports the removed
declaration, semantic changes, newly effective declaration, remaining
provenance, manifest and lock changes, package-local declaration state,
resolved state, lock-package removal, diagnostics, and dry-run state. Terminal
text and JSON DTOs remain in `internal/cli`.

## Selection and authority

The public selector accepts npm package names such as `foo` and `@scope/foo`.
Install-style constraint forms such as `foo@^4` are rejected with guidance to
use `tspack remove foo`; removal selects authored identity rather than querying
registry versions.

A single native package is selected automatically. Multiple native packages
require `--package <name>`. Within that package, exactly one owned, editable
declaration must match. Several matches fail with package, source, kind,
optional, constraint, and manifest provenance details. `--optional` narrows to
an optional declaration. No `--dev` alias is introduced.

Concept, template, generated, derived, and observed declarations are never
rewritten. A derived-only match is a successful no-op with its current
effective provenance. A package.json-native incremental project fails with
`TSPACK_REMOVE_AUTHORITY_DENIED` and points to `tspack npm uninstall`.

## Semantic and source flow

The operation loads normalized authoring IR, selects the editable declaration,
calls `authoring.Remove`, rebuilds the tape, rejects fatal projection conflicts,
and records `removed` plus `unshadowed` changes. Only then does
`manifestedit.PlanFile` map the removed declaration to its owned array element.
The guarded atomic writer checks that source bytes have not changed since
planning and preserves BOM, line endings, mode, and unrelated source. No-op
plans do not write or churn mtimes.

After a successful source write, remove runs the same update preflight and
commit path as add/update. Resolver failure rolls the manifest back when safe.
As with M69c, a final lock write failure retains authored intent and reports the
update diagnostic so rerunning update can converge. The lockfile is never
edited manually.

## Unshadow and remains-required semantics

Removing an effective explicit declaration can reveal a lower-precedence
declaration even when both constraints are equal. The result records that
declaration as `newlyEffective` and reports its concept/template provenance.
This is an authoring transition, not a value-only diff.

`stillRequired` and `stillDeclared` describe the selected package after tape
rebuild. `stillResolved` describes the post-update lock. Therefore removing a
direct dependency can produce any of these truthful outcomes:

- no effective declaration and no resolved artifact;
- a concept/template declaration becomes effective again;
- no selected-package declaration remains, but the artifact stays resolved
  transitively;
- no selected-package declaration remains, but another workspace package keeps
  the artifact resolved.

Lock-entry persistence is never presented as proof that the selected package
still declares the dependency.

## Dry-run, JSON, and no-op behavior

Dry-run performs manifest loading, declaration selection, pure semantic
removal, tape rebuilding, unshadow classification, and source projection. It
writes no manifest, lockfile, or store artifact. Because the current update API
loads TypeScript source from disk, dry-run does not fabricate a temporary
manifest write; post-removal resolved status is explicitly marked unknown.

JSON exposes stable semantic fields including package, source, kind, removed
constraint, target and manifest, declaration/manifest/lock changes,
still-declared/required/resolved state, resolved-status knowledge,
lock-package removal, newly effective declaration, provenance, dry-run, and
diagnostics. Raw AST and tape internals are not serialized.

The JSON performance object records manifest-load, semantic-removal,
projection, update, and total milliseconds plus metadata and tarball request
counts. Registry counts cover the memoized normal-update path; selection and
projection themselves make no registry requests.

Repeated removal and derived-only matches do not invoke update or registry
selection and do not rewrite files. Remove itself performs no registry query;
only normal update may request metadata required by the resulting graph.

## Validation coverage

Application tests cover package-selector parsing, real add/remove/update lock
convergence, repeated remove, re-add convergence, dry-run concept unshadowing,
optional ambiguity narrowing, workspace targeting, derived-only no-op, and
direct-removal with transitive lock persistence. Existing M69b projector tests
continue to cover CRLF/BOM preservation, guarded writes, locked files, and
source roundtrips.

## Deferred M69e work

Richer source and lock diffs, additional kind UX, source/registry selection,
and broader workspace convenience remain M69e work. M69d does not add JSR,
compiler changes, a reverse-graph subsystem, or an `rm` alias.
