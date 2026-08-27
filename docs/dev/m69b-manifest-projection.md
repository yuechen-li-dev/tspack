# M69b source-preserving manifest projection

## Status

M69b introduced the syntax-aware, source-preserving dependency projector later
used by the public M69c `tspack add` and M69d `tspack remove` commands. Its
completed path is:

```text
manifest frontend AST analysis
        -> owned package dependency island and UTF-8 byte ranges
M69a semantic EditResult
        -> internal/manifestedit source edit plan
        -> before/after source without filesystem mutation
```

The projector never reconstructs a manifest from evaluated IR and never
formats the whole file.

## Source-shape audit

Native manifests currently declare dependency values in several syntactic
forms:

- properties of `defineDeps({ ... })`, referenced as `deps.name`;
- standalone dependency-helper constants such as
  `const vite = tool(npm(...))`;
- dependency helper calls written directly in a package `values` array;
- inline packages in `manifest.tsx`;
- full split-workspace `package.manifest.tsx` contracts;
- concept and template output using either `defineDeps` or standalone helper
  constants;
- incremental `annotatePackage(<PackageAnnotations ... />)` classification
  files.

`defineDeps` is useful authoring syntax, but it is not package-local ownership:
one declaration object can be referenced by multiple packages, `Tools`, and
target rows. Rewriting it for one package could change another source surface.
The package-local source of dependency membership is the existing literal
shape:

```tsx
<Package dependencies={{ values: [deps.react, dep(npm("clsx", "^2"))] }} />
```

Consequently, M69b leaves `defineDeps`, standalone constants, concepts,
targets, and tools byte-identical. A changed or newly explicit declaration can
be rendered inline in the owned `values` array. Unused declarations outside
the island are user source and are deliberately not cleaned up.

## Owned dependency island

An owned dependency island is one `dependencies={{ values: [...] }}` attribute
on a structurally selected `Package`. Its identity includes:

- manifest path;
- literal package name when present;
- native-package versus incremental-annotation authority;
- the dependency attribute range;
- the literal array content range;
- the range of every existing array element;
- an AST-derived insertion point when the attribute is absent;
- the one editable named import from `tspack/manifest`, when present.

TypeScript source positions are converted to UTF-8 byte offsets in the
frontend. This keeps Go source edits correct when a manifest contains Unicode
before the island or a UTF-8 BOM.

## Classification and diagnostics

The frontend classification vocabulary is:

| Status | Meaning | Projection behavior |
| --- | --- | --- |
| `OwnedCanonical` | One selected package has literal `{ values: [...] }` dependencies | Editable subject to M69a authority |
| `OwnedRecognized` | Reserved for a safe normalizable variant | No current producer requires this variant |
| `UserDynamic` | The dependency expression or `values` member is not a literal object/array | Fail with `TSPACK_MANIFEST_DEPENDENCIES_DYNAMIC` |
| `Ambiguous` | More than one package surface matches an unqualified request | Fail with `TSPACK_MANIFEST_DEPENDENCY_ISLAND_AMBIGUOUS` |
| `Unsupported` | No matching package or no safe source range exists | Fail with a not-found or unsafe diagnostic |
| `Absent` | One selected package has no dependency attribute | Insert a canonical literal attribute at the AST closing-token position |

Projection also uses `TSPACK_MANIFEST_EDIT_AUTHORITY_DENIED` when an M69a
change targets generated, concept-owned, observed, derived, or annotation-mode
authority. Execution support remains independent from edit support.

## Projection API

`internal/manifestedit` owns the source-edit plan. `Plan` accepts source text,
manifest/package identity, frontend analysis, and the M69a semantic
`authoring.EditResult`. `PlanFile` reads a manifest and obtains AST metadata
from the existing process-local `internal/manifestfrontend` worker before
calling the pure planner.

The result contains:

- updated source;
- a changed flag;
- explicit byte-ranged `SourceEdit` values;
- the primary semantic add/remove/change records;
- structured diagnostics.

Planning and writing are separate. This is the dry-run seam for later CLI diff
presentation. `WritePlannedFile` is an explicit consumer: it re-reads and
compares the original bytes to reject concurrent source changes, skips an
unchanged plan, preserves file permissions where the platform exposes them,
writes and syncs a temporary file in the same directory, and atomically
replaces the destination. M69b does not add terminal formatting, registry
selection, or lockfile/resolver behavior.

## Rendering and ordering

Existing array elements remain in their exact source form and order. Adds are
appended, matching the explicit-user tape behavior from M69a. Removes delete
only the selected source element and its structurally owned separator. Changes
replace only the selected element with one deterministic inline helper call.

The renderer supports the currently executable npm, git, path, and workspace
sources and the `dep`, `peer`, and `tool` kinds. Object options use a stable
`key` then `optional` order. Missing helper imports are added deterministically
to the existing named `tspack/manifest` import; if there is no single safe
named import surface, projection fails rather than creating or rewriting an
arbitrary import layout.

## Source and comment preservation

Everything outside the island and any required helper import edit is copied
byte-for-byte. Existing elements are not reprinted. Therefore imports not
needed by the new expression, custom declarations, policies, targets,
RunTargets, concepts, blank lines, and unusual but valid source remain
untouched.

Comments before the first entry are island-level and survive. Comments before
an unchanged entry survive. A comment structurally attached to a removed entry
is removed with it. Same-line trailing comments are treated as entry-attached.
The projector does not attempt semantic relocation of ambiguous comments.

## Line endings, BOM, and determinism

New text uses the source file's CRLF or LF convention. Existing bytes,
including a UTF-8 BOM, are retained. Applying identical source edits is
deterministic, and an edit result with no primary semantic changes is a
byte-identical no-op. The real frontend roundtrip test proves add, reload,
remove, and byte-identical restoration.

The pure projection layer does not write files. Its separate guarded writer
does not open the destination for truncation: a locked-file replacement fails
with `TSPACK_MANIFEST_WRITE_FAILED` while the original remains in place. No-op
plans do not write and therefore do not churn mtimes. Windows uses
`MoveFileEx(REPLACE_EXISTING | WRITE_THROUGH)`; other platforms use same-filesystem
rename.

## Package-local and incremental authority

The package name plus manifest path selects the island, so the same API works
for root inline packages and full `package.manifest.tsx` contracts. Multiple
inline packages require a package name and are never selected by first match.

`PackageAnnotations` surfaces are identified separately. They classify an
authoritative package.json during incremental adoption and are not native
dependency write targets. Projection returns
`TSPACK_MANIFEST_EDIT_AUTHORITY_DENIED`; it does not invent package.json
mutation semantics.

## Validation and performance

Coverage includes canonical no-op, add, remove, constraint change, unshadow
evidence, comment survival/removal, custom source preservation, CRLF, absent
insertion, dynamic and ambiguous failures, authority denial, UTF-8 ranges, and
the real frontend/IR/tape/project/reload roundtrip.

Analysis reuses the serialized process-local frontend worker. It creates a
fresh TypeScript `SourceFile` per request and retains only narrow ranges, not
AST objects. Rendering and applying edits are in-process linear source work.

## Deferred M69 work

- public `tspack add` and `tspack remove` lifecycle commands;
- package/source/kind selector UX and remains-required rendering;
- registry policy when no constraint is supplied;
- terminal diff presentation and lifecycle-command integration of the atomic writer;
- optional/dev/package flags and other M69e polish.

JSR, alternate compilers, resolver redesign, lockfile changes, and a general
TypeScript formatter remain out of scope.
