# Ecosystem/backend seam spike

M57b keeps `manifest.tsx` as the universal manifest frontend while naming the backend axes that should not be collapsed into one npm-shaped model forever.

## Axes

- **Manifest frontend language:** TypeScript/TSX. This should remain special because manifests are authored and collected through `manifest.tsx`.
- **Project/package ecosystem:** npm today. PyPI and future indexes/mirrors are future design vocabulary only.
- **Backend:** TypeScript/npm resolution and materialization today. Future backends might include Python-family/PyPI with `uv` delegation, but M57b does not implement that.
- **Runtime family:** JavaScript today; Python-family later. Python-family intentionally covers CPython, PyPy, Cython/native extension builds, Triton, Mojo, Numba, JAX-style staged computation, and similar Python-compatible or Python-like systems.
- **Runtime implementation:** `nodejs`, `bun`, and `deno` today; future reserved examples include `cpython`, `pypy`, `mojo`, and `triton`.
- **Environment/materialization strategy:** `node_modules` today; future reserved examples include `uvManaged`, `systemInterpreter`, `hermetic`, and possibly venv-like layouts under the hood.
- **Version/range scheme:** npm semver/ranges today; PEP 440/specifiers are reserved for future Python-family package work.
- **Security/build risk model:** npm lifecycle scripts today; Python-family build backends, native extensions, wheels/sdists, external indexes, and missing hashes later.

## Already relatively language-neutral

- Dependency intent separates `dep`, `peer`, `tool`, `type`, `test`, and `workspace` from dependency source.
- Workspace/package graph construction mostly consumes normalized manifest IR rather than TS syntax.
- Boundary and import-scan checks reason about package declarations and file imports, although the scanner is JS/TS-oriented.
- Template concepts are becoming metadata-driven rather than hardcoded per template.
- Run targets are conceptually generic process declarations, even though the supported runtimes are JavaScript/system launchers today.

## Current npm/TypeScript assumptions inventory

- **Manifest package model:** package names and versions use npm-like names and semver-looking package versions; targets describe TypeScript/JavaScript package outputs with `entry`, `runtime`, `types`, `exports`, peers, and deps.
- **Dependency sources:** manifest validation accepts `npm`, `git`, `path`, and `workspace`; `pypi` is intentionally invalid.
- **Version/range parsing:** resolver, outdated, and update policy use npm registry metadata plus Masterminds semver constraints/comparison.
- **Update/outdated policy:** only `source.kind == "npm"` gets registry checks; non-registry sources are skipped or warned.
- **Resolver/registry client:** `internal/resolver` is npm registry metadata and tarball oriented.
- **Lockfile encoding/validation:** lockfile package sources are `npm`, `git`, `path`, and `workspace`; npm packages require version plus integrity/hash.
- **Sync/materialization/store:** package IDs, tarball hashes, store layout, and `node_modules` materialization are npm/TSPack oriented.
- **Run target runtime profiles:** workspace profiles are `nodejs`, `bun`, and `deno`; run launchers include `node`, `bun`, `deno`, and `system`.
- **Security lifecycle model:** capability detection and acknowledgment are centered on npm lifecycle scripts.
- **Pack/package output:** pack output is package.json/npm oriented, including `main`, `types`, `exports`, `peerDependencies`, and `peerDependenciesMeta`.
- **Check/import scanning/boundaries:** source scanning is JS/TS import oriented and package boundary diagnostics use npm dependency declarations.
- **Migrate/package.json logic:** migration consumes `package.json`, npm dependency buckets, package-lock evidence, npm scripts, and lifecycle names.
- **Templates/concepts:** current built-ins are TypeScript/npm project shapes.
- **CLI help/docs:** commands describe current TSPack/npm lockfile, update, sync, pack, check, run, and security behavior; no Python-family product surface exists.

## Proposed seam

M57b adds a tiny internal vocabulary package (`internal/ecosystem`) with:

- `EcosystemID` (`npm`; `pypi` reserved only),
- `BackendKind` (`typescript-npm`; `python-pypi` reserved only),
- `RuntimeFamily` (`javascript`; `python-family` reserved only),
- `RuntimeImplementation` (`nodejs`, `bun`, `deno`; Python-family examples such as `cpython`, `pypy`, `mojo`, and `triton` reserved only),
- `EnvironmentKind` (`nodeModules`; Python-family examples such as `uvManaged`, `pythonVenv`, `systemInterpreter`, and `hermetic` reserved only),
- `ExecutionMode` (`interpreted`; examples such as `nativeExtension`, `jit`, `stagedGpu`, and `packageBuild` reserved only),
- `VersionScheme` (`npmSemver`; `pep440` reserved only),
- `RangeScheme` (`npmSemverRange`; `pep440Specifier` reserved only), and
- `BackendDescriptor` metadata with production/reserved status.

The only production descriptor is TypeScript/npm. The reserved Python/PyPI descriptor and Python-family vocabulary are not wired into manifest validation, lockfile validation, resolution, sync, pack, run, security, or templates.

This is intentionally not a plugin system. Do not add `Resolve`, `Materialize`, `Pack`, `Run`, or `Security` methods until real backend behavior needs a small interface with clear ownership.

## Python-family environment thesis

Future Python-family support should not be framed as a single flat runtime named `python`, and it should not make users manage virtual environments as the project model.

The user-facing contract should be:

1. `manifest.tsx` declares the environment contract.
2. `tspack update` resolves it.
3. `tspack sync` materializes it.
4. `tspack run` executes inside it.
5. `tspack check` verifies it.

A future backend may use virtual environments, uv environments, caches, or interpreter-specific layouts internally. Users should not need to create/activate a venv, guess which `pip` belongs to which interpreter, debug editor-vs-terminal interpreter drift, or run tools outside the project environment. Venv is an implementation detail, not the TSPack product abstraction.

## Non-goals

- No Python resolver, PyPI resolution, uv integration, Python templates, pyproject projection, Python runtime execution, or `manifest.py`.
- No `runtime: "python"`, flat Python runtime profile, or production Python-family runtime implementation.
- No lockfile rename, lockfile migration, package-manager behavior changes, release publication, or new CLI product surface.
- No production acceptance of `pypi` manifest sources or lockfile package sources.

## Future Python-family/PyPI plug-in shape

A future Python-family/PyPI backend should own Python-family source validation, PEP 440 parsing/specifiers, build/index security categories, uv or PyPI delegation, pyproject projection, and runtime/tool execution. It should model package ecosystem, runtime family, concrete runtime implementation, environment/materialization strategy, execution/build mode, version/range scheme, and security/build risk separately. Shared code should route through backend descriptors or narrow interfaces so Python-family behavior stays localized instead of scattering `if ecosystem == "pypi"` or `if runtime == "python"` branches across resolver, lockfile, check, pack, run, security, templates, and CLI code.

Security must be redesigned for Python-family systems rather than reusing npm lifecycle acknowledgments. Python-family risks include build backends, source distribution builds, native wheels, native extension compilation, JIT/staged execution, GPU-kernel DSLs, external indexes, and missing hashes.
