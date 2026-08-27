# M70b JSR add and multi-registry authoring

## Status

M70b makes the M70a registry backend architecture available through the normal
dependency-editing workflow. A native project can author npm and JSR roots in
one manifest and resolve them through one mixed graph, lockfile, store, and
materializer.

```text
tspack add zod
tspack add @std/path --source jsr
```

npm remains the default. Source selection is explicit and bounded to `npm` or
`jsr`; TSPack never retries a failed lookup against another registry. There is
no `--jsr` alias, registry discovery lottery, project-wide JSR mode, Deno
requirement, or npm/Deno subprocess.

## Application path

Both sources use the M69 editing architecture:

```text
CLI source parsing
  -> typed project.AddDependencyRequest
  -> selected RegistryBackend metadata
  -> stable version / constraint policy
  -> source-qualified Authoring IR edit
  -> dependency tape
  -> guarded manifest projection
  -> ordinary mixed-source RunUpdate
  -> exact ts-lock.toml + shared content store
```

The selection, preflight, and committed update share source-qualified memoized
backend calls. A simple successful add needs one metadata request and one
artifact request. Repeating an equivalent editable declaration returns before
backend creation and performs zero registry or artifact requests.

## Version and package-spec policy

Scoped package parsing is source-neutral. `@std/path`, `@std/path@^1`,
`@scope/foo`, and `@scope/foo@1.2.3` preserve the scope/name boundary. An
unqualified add ignores prereleases, selects the highest stable SemVer release,
and authors `^<selectedVersion>`. An explicit valid constraint is preserved and
may select a prerelease.

Metadata and selection failures identify the source-qualified package, such as
`jsr:@std/path`. Unsupported sources list `npm` and `jsr` and fail before
manifest loading or network access.

## Canonical authoring and identity

The existing manifest helper is canonical:

```tsx
dep(jsr("@std/path", "^1.1.6"))
```

The frontend API, generated manifest declarations, editor SDK, Authoring IR,
resolver, lockfile, `why`, and removal all retain `jsr:@std/path` as semantic
identity. JSR's npm-compatibility artifact name (`@jsr/std__path`) is confined
to the backend/materializer boundary and is never authored as npm intent.

When npm and JSR declarations have the same logical name, add gives the later
declaration a source-qualified dependency key so both remain editable and
resolvable. Replacement selectors include source-qualified identity as well as
declaration provenance; changing the JSR constraint cannot replace the npm
declaration. Unqualified removal remains ambiguous and requires `--source`.

## Update, store, and materialization

After projection, add invokes the ordinary update twice: a dry preflight and the
committed update. JSR-to-JSR and JSR-to-npm transitives are discovered by the
M70a backends, not by CLI code. Lock IDs and edges retain their real sources.

`update` captures verified artifacts in the shared store. `sync` now treats
direct `package:dependency` lock edges as materialization roots in addition to
target and tool edges, so dependencies authored by add are actually consumable.
Transitive-only dependencies remain nested and do not become phantom roots.

## Node and TypeScript imports

JSR compatibility artifacts contain JavaScript, declarations, and exports data.
The current Node materializer installs `jsr:@scope/package` at
`node_modules/@jsr/scope__package`. Application code therefore imports the
Node-compatible name:

```ts
import { join } from "@jsr/std__path";
```

This form resolves under TypeScript `NodeNext`. TSPack does not rewrite
application source, and the compatibility import spelling does not change
manifest or lock identity.

## Authority, optionality, and targeting

`--optional`, `--kind peer`, `--package`, and current-directory package
inference are orthogonal to source selection. package.json-authoritative
incremental projects remain non-editable: TSPack does not invent an npm alias or
package.json encoding for native JSR intent.

## Security, audit, and performance

JSR resolution executes neither package code nor lifecycle scripts. JSR
packages have no npm lifecycle capabilities; npm transitives retain npm
capability metadata and blocked execution policy. OSV has no JSR ecosystem
identifier, so audit checks npm packages and reports JSR packages as
`not-checked`.

The existing performance pipeline attributes default HTTP work as
`npm.metadata`, `npm.tarball`, `jsr.metadata`, and `jsr.tarball`. Add JSON keeps
the stable aggregate metadata/artifact request counts and contains no raw
registry response data.

## M70c and M70d handoff

M70b also closes two straightforward cross-source usability gaps: same-name
replacement selectors are source-qualified, and direct dependency lock edges
are materialization roots. Exotic alias/URL forms and additional export or peer
edge cases remain evidence-driven M70c work.

M70d remains the home for registry allowlists, preferences, mirrors, fallback,
corporate feeds, air-gapped cache policy, and source trust policy. None of those
policies are inferred or implemented by M70b.
