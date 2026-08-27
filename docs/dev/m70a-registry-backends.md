# M70a registry backends

## Status

M70a adds a source-qualified registry backend contract and direct, read-only JSR
resolution. npm and JSR packages can participate in one resolver frontier, one
`ts-lock.toml`, one content-addressed store, and one materialized Node tree.

JSR support does not install or invoke Deno. A registry is a package source; it
does not select the workspace runtime or package manager.

## Before M70a

The original registry path was structurally npm-specific:

```text
manifest dependency
  -> resolver work item keyed by name + range
  -> NPMRegistryClient.PackageMetadata
  -> npm PackageMetadata / PackageVersion / PackageDist
  -> dist.tarball download and SRI verification
  -> npm tarball package.json inspection
  -> npm lock package
  -> npm-tarball store extraction
  -> node_modules materialization
```

The audit found these assumptions:

| Assumption | Previous owner | Classification | M70a disposition |
| --- | --- | --- | --- |
| `registry.npmjs.org` default | resolver HTTP client | backend-specific | remains the npm backend default |
| npm `versions`, `dist-tags`, `dist.tarball`, and `dist.integrity` fields | resolver | backend-specific | confined behind npm/JSR adapters |
| SemVer constraint selection | npm resolver | normalized resolver concept | shared by all registry backends |
| work and memo keys omit source | resolver | normalized resolver concept | keys include source + name + range |
| transitive maps implicitly inherit npm | resolver | backend-specific | adapters emit source-qualified requirements |
| tarball download and SRI verification | resolver | artifact/security concern | driven by normalized artifact descriptors |
| tarball package.json validation | resolver | artifact/security concern | verifies the backend-declared artifact name |
| `npm-tarball` extraction | store | store/artifact concern | registry tarballs share safe extraction/store |
| npm lifecycle scripts | resolver/security | security concern | npm reports them; JSR reports no install capabilities |
| `npm:name@version` lock IDs | lockfile | normalized provenance | generalized without a format bump |
| npm-only add/outdated/targeted update | project/CLI | application/presentation | explicit M70b follow-up |
| npm-only OSV queries | audit | security/presentation | npm checked; JSR explicitly `not-checked` |

Local `workspace`, `path`, and `git` sources retain their existing resolver path.
They are deliberately not forced into the registry contract.

## Backend contract

`internal/resolver.RegistryBackend` exposes TSPack concepts: source identity,
package versions, source-qualified dependency requirements, artifact
descriptors, artifact retrieval, and request host attribution.

The normalized model is:

```text
RegistryPackageMetadata
  Identity { Source, Name }
  Versions
    RegistryPackageVersion
      Identity
      Version
      Dependencies[] { Identity, Constraint, Kind, Optional }
      Artifact { Kind, URL, Integrity }
      ArtifactPackageName
      Capabilities[]
```

The resolver registry is a small `map[source]RegistryBackend`, not a plugin
system. Unsupported sources produce `TSPACK_REGISTRY_SOURCE_UNSUPPORTED`.

## npm backend

The npm adapter wraps the existing `NPMRegistryClient`. Existing response types,
metadata lookup, optional-dependency behavior, SemVer selection, integrity,
package.json validation, lifecycle capture, parallel frontiers, and
deterministic commit order remain unchanged. Established npm diagnostic codes
are preserved.

## JSR backend

JSR documents a native module API and an npm compatibility API. The compatibility
registry at `https://npm.jsr.io` serves npm-compatible metadata and immutable,
revisioned tarballs containing transpiled JavaScript, generated declarations,
and an exports-aware package.json. M70a uses that endpoint directly because the
current materializer targets Node/TypeScript package trees.

TSPack does not shell out to `deno`, `npm`, `npx`, or another package manager.
HTTP transport, timeouts, pooling, instrumentation, and request headers are
shared with npm.

Authoritative references:

- <https://jsr.io/docs/api>
- <https://jsr.io/docs/npm-compatibility>
- <https://jsr.io/docs/packages>

## Names and dependency normalization

Public identity `jsr:@std/path` maps at the adapter/materializer boundary to the
compatibility name `@jsr/std__path`. Compatibility dependency keys beginning
with `@jsr/` become JSR requirements; ordinary keys become npm requirements.
Thus `jsr:@deno/esbuild-plugin` routes to both `jsr:@deno/loader` and
`npm:esbuild`. Malformed compatibility names fail rather than being dropped.
Exotic npm aliases and URL/import-map forms remain M70c work.

## Version semantics

JSR requires SemVer and excludes yanked versions from compatibility metadata.
TSPack reuses its selector for exact, caret, tilde, ranges, stable versions, and
explicit prereleases. There is no source-specific version fork.

## Artifacts, integrity, and store

Both backends currently emit normalized tarball descriptors, but shared resolver
code no longer reads `dist.tarball`. It verifies advertised SHA-256/SHA-512 SRI,
checks artifact package name/version, computes TSPack's SHA-256 identity, and
captures bytes into the existing store. Lock integrity plus content hash pins
the exact JSR compatibility revision. No second store exists.

`sync` reuses captured content without a registry. Missing content can be
rehydrated for an exact locked JSR version and must match integrity and hash.

## Node and TypeScript compatibility

JSR compatibility artifacts and their compiled imports use names such as
`@jsr/std__path`. Materialization derives that name from locked
`jsr:@std/path`; lock and `why` identity remain native/source-qualified. This
also permits a same-text npm and JSR package to coexist physically:

```text
npm:@scope/foo  -> node_modules/@scope/foo
jsr:@scope/foo  -> node_modules/@jsr/scope__foo
```

The compatibility artifacts contain `.d.ts` files and exports metadata. The
live fixture passes ordinary TypeScript `NodeNext` resolution.

## Security, audit, and capabilities

JSR packages do not inherit npm lifecycle capability semantics. Artifacts are
fetched and materialized without executing package scripts. npm dependencies
reached from JSR retain normal npm capabilities; dogfood demonstrates this with
esbuild's blocked postinstall.

OSV defines npm but not a JSR ecosystem. `tspack audit` scans npm packages and
adds an explicit `not-checked` JSR coverage row. Mixed graphs are never presented
as fully checked by silently omitting JSR.

## Determinism, caching, and performance

Work groups and metadata memo keys include source, package, and constraint. npm
and JSR may run in one parallel frontier while lock commit stays deterministic.
HTTP performance kinds are `npm.metadata`, `npm.tarball`, `jsr.metadata`, and
`jsr.tarball`, with host attribution retained.

Focused fake-registry tests compare repeated mixed lock bytes. The live fixture
also produced identical SHA-256 lock bytes across repeated updates.

## Dogfood matrix

`fixtures/valid/jsr-mixed` records:

- `npm:picocolors@1.1.1` — npm beside JSR roots;
- `jsr:@luca/flag@1.0.1` — leaf package;
- `jsr:@std/path@1.1.6` — exports/types and a JSR transitive;
- `jsr:@deno/esbuild-plugin@1.2.1` — JSR and npm transitives.

The fixture resolves 5 JSR and 28 npm lock packages. Node consumes its
materialized JSR packages, TypeScript resolves their declarations, and the
dogfood machine had no `deno` executable on `PATH`.

## M70 handoff

- **M70b — partially enabled:** `jsr(...)` authoring and source-aware removal
  work, including collision ambiguity; `tspack add --source jsr` and
  source-aware outdated/preflight remain.
- **M70c — partially enabled:** ordinary JSR-to-JSR and JSR-to-npm edges work;
  alias/URL forms, export edge cases, and additional collision UX remain.
- **M70d — still required:** allowlists, preference, mirrors, fallback,
  organization policy, air-gapped configuration, and resilience policy.

Future source policy should build on explicit backend selection rather than a
project-wide registry mode, and must preserve artifact pinning/provenance.
