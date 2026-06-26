# M57a: Experimental Python module design pass

Status: **experimental design / not implemented**.

This document is a design pass and tiny prototype sketch for a possible Python project/package domain in TSPack after v0.1.3. It intentionally does not change product behavior.

## Thesis

TSPack should keep one manifest frontend and allow many project/package backends:

```text
manifest.tsx
  -> manifest frontend IR
  -> project/package IR
  -> TypeScript/npm backend today
  -> experimental Python/PyPI backend later
```

The important distinction is that `manifest.tsx` is the typed project contract language. It does **not** mean the managed project must itself be TypeScript. Python support, if pursued, should be declared from `manifest.tsx`, analyzed as manifest data, and projected to Python ecosystem files only as compatibility glue.

There should be no `manifest.py`, no executable Python configuration, and no setup.py-style arbitrary execution surface.

## Non-goals

M57a does not implement:

- a PyPI resolver;
- production Python dependency resolution;
- `uv` integration;
- wheel or sdist building;
- lockfile format migration or `ts-lock.toml` rename;
- a full `pyproject.toml` projector;
- Python security policy enforcement;
- CLI productization;
- release publication;
- new templates outside docs-only examples;
- changes to current TypeScript/npm behavior.

## Proposed manifest API sketch

The API should make Python an ecosystem backend, not a second manifest language. A future design could choose object helpers rather than JSX for ecosystem-specific package declarations so Python package data can sit beside current `Package` declarations without overloading TypeScript target fields.

Conceptual sketch:

```tsx
import {
  PyPI,
  PythonPackage,
  PythonRuntime,
  PythonTool,
  RunTarget,
  Workspace,
} from "tspack/manifest";

export default Workspace({
  name: "python-demo",
  packages: [
    PythonPackage({
      name: "python-demo",
      version: "0.1.0",
      runtime: PythonRuntime({ python: ">=3.11" }),
      dependencies: [
        PyPI("fastapi", ">=0.115,<0.116"),
        PyPI("pydantic", ">=2,<3"),
      ],
      optionalDependencies: {
        dev: [PyPI("httpx", ">=0.28,<0.29")],
      },
      tools: [
        PythonTool(PyPI("ruff", ">=0.8,<0.9")),
        PythonTool(PyPI("pytest", ">=8,<9")),
        PythonTool(PyPI("mypy", ">=1.13,<2")),
      ],
      run: [
        RunTarget("test", ["pytest"]),
        RunTarget("lint", ["ruff", "check", "."]),
        RunTarget("typecheck", ["mypy", "src"]),
      ],
    }),
  ],
});
```

Names are provisional:

- `PythonPackage(...)` makes the backend explicit and avoids implying the existing TypeScript-oriented `Package` target model already works for Python.
- `PyPI(name, specifier)` is preferable to reusing `npm(...)` because Python uses PEP 440 specifiers, not npm semver ranges.
- `PythonRuntime({ python: ">=3.11" })` makes interpreter compatibility first-class.
- `PythonTool(...)` can initially be an intent marker over a PyPI dependency; it should not imply a global tool installation strategy.
- `RunTarget(...)` remains language-neutral in concept, but its runtime/tool lookup path needs a Python execution model before behavior is implemented.

An alternate lower-disruption phase 1 shape is to add a `pypi(name, specifier)` source plus `ecosystem: "pypi"` on a future package IR. That is smaller, but risks making Python fit the current npm-shaped `Package` too early.

## Coexistence with current Package/Dep/Tool model

Current manifests already distinguish dependency intent (`dep`, `peer`, `tool`) from source (`npm`, `git`, `path`, `workspace`). Python can reuse the intent split for runtime dependencies and tools, but peer dependencies do not map directly. Python extras should not be forced through npm peer semantics.

Recommended mapping:

| Python concept | TSPack concept | Notes |
| --- | --- | --- |
| install/runtime requirement | dependency intent | Source kind `pypi`; range scheme `pep440Specifier`. |
| tool such as `ruff`, `pytest`, `mypy`, `pyright`, `uv` | tool intent | Materialized through `uv`/venv later. |
| optional dependency group / extra | optional dependency group | Design as named extras, not npm peer deps. |
| console script entry point | package entry point metadata | Future projection into `[project.scripts]`. |
| build backend | build metadata / security surface | Future policy and projection only. |
| peer dependency | mostly not applicable | Do not model as npm peers unless a real Python use case appears. |

## Manifest frontend impact

The existing manifest frontend can parse only approved imports/helpers/elements from `tspack/manifest`. A Python API can be introduced without a Python runtime language, but it must be added to the approved manifest authoring surface and the JSON IR before it can be executable behavior.

For M57a, docs-only examples are safer than declaration stubs because a stub that typechecks but is not preserved by the collector would imply support that does not exist. A future phase should add parser fixtures that prove Python nodes round-trip into an experimental IR field before exposing editor declarations.

## Modularity and high-locality rule

Future Python work must enter after manifest parsing through a narrow backend seam. The goal is not to scatter `if ecosystem == "pypi"` branches through resolver, lockfile, check, pack, run, security, templates, and CLI code. If a future implementation requires edits across many unrelated packages, stop and design a seam first.

TypeScript remains special at the frontend layer because `manifest.tsx` is the canonical manifest authoring language. That does not make the TypeScript/npm implementation the shape every backend must copy. Python should be an ecosystem/backend module behind shared interfaces, not a second frontend and not a set of conditionals smeared over the npm path.

Preferred direction:

```text
manifest.tsx
  -> manifest frontend IR
  -> language-neutral project/package IR
  -> backend module interface
       - typescript/npm backend
       - experimental python/pypi backend later
       - future ecosystems later
```

Good future shape:

- Python-specific code lives under a high-locality module such as `internal/ecosystems/python` or `internal/backends/python`.
- npm-specific behavior remains localized in a TypeScript/npm backend or existing npm production path.
- shared code talks to backends through small interfaces.
- existing TypeScript/npm tests and behavior stay unchanged unless a deliberate shared seam is introduced.

Bad future shape:

- `pypi` branches scattered through npm resolver code, store code, lockfile validation, `check`, `pack`, `run`, security policy, template selection, and CLI command handlers.
- Python package roots forced into TypeScript target fields.
- Python tools resolved through Node runtime or `node_modules/.bin` code.
- npm lifecycle-script security semantics reused as if they were Python build semantics.

## Backend module seam sketch

A future seam can start smaller than this, but the design center should be a backend module interface with responsibilities like:

```go
type EcosystemBackend interface {
    ID() EcosystemID
    NormalizePackage(input BackendPackageInput) (ProjectPackage, []diag.Diagnostic)
    ValidatePackage(pkg ProjectPackage) []diag.Diagnostic
    VersionScheme() VersionScheme
    RangeScheme() RangeScheme
    Resolve(ctx context.Context, req ResolveRequest) (ResolveResult, []diag.Diagnostic)
    Materialize(ctx context.Context, req MaterializeRequest) []diag.Diagnostic
    Check(ctx context.Context, req CheckRequest) []diag.Diagnostic
    Explain(ctx context.Context, req ExplainRequest) (ExplainResult, []diag.Diagnostic)
    Project(ctx context.Context, req ProjectionRequest) (ProjectionResult, []diag.Diagnostic)
    SecurityModel() SecurityModel
}
```

M57a does not implement this interface. The sketch is here to keep future work modular. A tiny future implementation might begin with only `ID`, package normalization, version/range schemes, validation, and projection hooks, leaving resolution and materialization unsupported for Python.

Proposed responsibility boundaries:

| Responsibility | Shared layer | TypeScript/npm backend | Experimental Python/PyPI backend |
| --- | --- | --- | --- |
| package model normalization | owns common `ProjectPackage` shape | maps npm package targets, exports, JS/TS types | maps Python package roots, modules, extras, entry points |
| dependency source handling | routes by backend/source kind | owns `npm` registry semantics and npm semver ranges | owns `pypi` source semantics and PEP 440 specifiers |
| lockfile encoding/validation | stores ecosystem-aware package records | validates npm tarball/integrity metadata | validates wheel/sdist/hash/tag metadata later |
| resolution/materialization | dispatches to selected backend | owns npm resolver, store, materialization | projection-only or `uv`-backed initially |
| runtime/tool execution | dispatches by runtime/tool environment | owns nodejs/bun/deno and npm bin lookup | owns Python interpreter, `uv run`/venv, Python tool lookup later |
| security policy | aggregates backend security findings | owns npm lifecycle-script capabilities | owns Python build backend, sdist, native wheel, external index, missing hash categories |
| projection | detects drift and writes generated files | owns package.json/TS projections | owns pyproject/tool config/uv provenance projections later |
| templates/concepts | stores concept metadata and composition | owns TS/npm concepts | owns future Python concepts without hardcoded template explosion |

## Internal IR impact

Language-neutral or mostly neutral today:

- `ManifestIR` already has workspace-level metadata plus a package list.
- dependency intents already have `kind`, `source`, and optionality.
- `RunTarget` is command-oriented and can express `pytest`, `ruff`, `mypy`, or `uvicorn` commands.
- lock edges use generic `from`, `to`, `kind`, and `optional` fields.

TypeScript/npm assumptions that need isolation:

- The manifest package IR has TypeScript-oriented targets with `entry`, `runtime`, and `types`; Python packages need module/package roots, import packages, scripts, and build metadata instead.
- manifest source kinds are currently `npm`, `git`, `path`, and `workspace`; `pypi` is not valid yet.
- workspace and run target runtime profiles are JavaScript runtime profiles (`nodejs`, `bun`, `deno`) rather than language runtimes.
- lockfile validation only accepts package sources `npm`, `git`, `path`, and `workspace`.
- `outdated` and update policy use npm registry metadata and Masterminds semver; non-npm sources are skipped or warned.
- security capability modeling is centered on npm lifecycle scripts.
- pack output is npm/package.json-oriented, including `main`, `types`, `exports`, `peerDependencies`, and `peerDependenciesMeta`.
- import scanning and boundary checks are TypeScript/JavaScript source oriented.
- migration logic consumes `package.json` and npm lockfile evidence.
- templates currently create TypeScript-oriented source/config projections.

Future IR should add an ecosystem-aware package layer rather than mutating every field on the current package struct. A compact first step could be:

```text
ProjectPackageIR {
  name
  version
  ecosystem: "npm" | "pypi"
  backendKind: "typescript-npm" | "python-pypi"
  dependencies[]
  tools[]
  runTargets[]
  backend: NpmPackageData | PythonPackageData
}
```

`ecosystem` is useful for package identity and registry behavior. `backendKind` is useful when the same registry has multiple materialization/build backends or when one backend delegates to tools like `uv`.

## Dependency model

Python dependency entries should preserve Python package names and extras without npm normalization. Proposed fields:

```text
PythonDependency {
  key?: string
  kind: "dependency" | "tool" | "optional"
  source: PyPISource
  extras?: string[]
  markers?: string
  optionalGroup?: string
}

PyPISource {
  ecosystem: "pypi"
  name: string
  specifier: string
  index?: string
}
```

Design notes:

- Runtime dependencies map to `dependencies`.
- Tool dependencies map to `tools` and are resolved/materialized in the package environment later.
- Optional dependencies and extras need named groups because Python extras are part of package metadata and user install intent.
- Environment markers such as platform or Python-version markers should be preserved rather than interpreted prematurely.
- Peer dependencies should be omitted until a real Python equivalent is identified.

## Version and range schemes

Python must not use npm semver ranges. Python packages use PEP 440 versions and specifiers such as:

- `>=3.11`
- `~=2.0`
- `==1.2.*`
- `>=0.115,<0.116`

TSPack needs a version abstraction before Python resolution or outdated behavior:

```text
VersionScheme = "npmSemver" | "pep440"
RangeScheme = "npmSemverRange" | "pep440Specifier"
```

Current npm-oriented places include outdated calculation, update policy level comparisons, range parsing, wanted/latest selection, and test fixtures that assume npm semver strings. Python pre-releases, epochs, local versions, post releases, compatible-release specifiers, wildcard equality, and environment markers need a separate implementation or delegation to Python packaging tooling.

## Lockfile considerations

Do not rename `ts-lock.toml` in this milestone. The current file can eventually represent multiple ecosystems if lock entries become ecosystem-aware and validation accepts backend-specific metadata.

Possible future PyPI lock entry shape:

```toml
[[package]]
id = "pypi:fastapi@0.115.6"
source = "pypi"
name = "fastapi"
version = "0.115.6"
index = "https://pypi.org/simple"
requires_python = ">=3.8"

[[package.artifact]]
kind = "wheel"
filename = "fastapi-0.115.6-py3-none-any.whl"
url = "https://files.pythonhosted.org/..."
hash = "sha256:..."
python_tags = ["py3"]
abi_tags = ["none"]
platform_tags = ["any"]

[[package.artifact]]
kind = "sdist"
filename = "fastapi-0.115.6.tar.gz"
hash = "sha256:..."
```

Deferred fields:

- full wheel tag compatibility evaluation;
- per-file hash policy;
- source distribution build backend metadata;
- platform-specific selection;
- imported `uv.lock` provenance;
- index authentication and mirror policy;
- reproducible build attestations.

## Resolution strategy

M57a should not implement PyPI resolution. The preferred path is hybrid and delegation-first:

1. TSPack owns manifest authoring, validation, project policy, diagnostics, and generated projections.
2. `uv` owns PyPI resolution, virtual environment materialization, and Python package installation at first.
3. TSPack imports `uv` lock/report metadata for explanation, check, and security diagnostics.
4. A TSPack-native PyPI resolver should be considered only if policy, cross-ecosystem lock integration, or audit requirements justify the complexity.

This keeps TSPack from becoming another Python package manager while still letting it express a typed project contract. The npm resolver should not learn Python internals; shared orchestration should dispatch to a backend that either reports unsupported Python resolution or delegates to `uv`.

## Projection strategy

Python projection should treat generated files as compatibility glue, not source of truth. Possible projections:

- `pyproject.toml` for `[project]`, dependencies, optional dependencies, scripts, and tool config references;
- `uv.lock` managed by `uv`, not hand-authored by TSPack;
- `requirements.txt` only as compatibility/export output;
- `ruff`, `pytest`, `mypy`, and `pyright` config sections when intentionally generated;
- package entry points via `[project.scripts]`;
- wheel/sdist metadata and build-system settings in later pack phases.

TSPack should avoid becoming a random config pile. Projection should be deterministic, documented, and checkable for drift.

## Security model considerations

Python supply-chain risk does not map 1:1 to npm lifecycle scripts. Future security categories should include:

- PEP 517 build backends and build hooks;
- legacy `setup.py` execution risk;
- source distributions requiring local build execution;
- wheels with native code or narrow platform tags;
- dependency confusion across indexes and mirrors;
- typosquatting and name similarity;
- unpinned or broad transitive dependencies;
- missing hashes or incomplete hash coverage;
- platform-specific wheels that produce different installed artifacts;
- build isolation and backend dependency risk.

Lifecycle acknowledgments should not be blindly reused. Python likely needs capabilities such as `pythonBuildBackend`, `sourceDistributionBuild`, `nativeWheel`, `externalIndex`, and `missingHashes` with explicit audit details. These should live in the Python security model rather than being bolted onto npm lifecycle checks.

## RunTarget considerations

RunTargets are conceptually language-neutral because they are named commands. Python examples:

```tsx
RunTarget("test", ["pytest"])
RunTarget("lint", ["ruff", "check", "."])
RunTarget("format", ["ruff", "format", "."])
RunTarget("typecheck", ["mypy", "src"])
RunTarget("dev", ["uvicorn", "python_demo.app:app", "--reload"])
RunTarget("build", ["python", "-m", "build"])
```

Current behavior has JavaScript runtime assumptions. Python execution should not be bolted into Node runtime code; a future Python runtime/tool model should support:

- `runtime: "python"` or `runtime: { kind: "python", version: ">=3.11" }`;
- command execution through `uv run`, a project venv, or explicit system Python;
- tool lookup from the resolved Python environment rather than `node_modules/.bin`;
- clear diagnostics when the interpreter or environment is missing.

## Template and concept possibilities

Future concept names could include:

- `python.app`
- `python.library`
- `python.cli`
- `python.fastapi`
- `python.pytest`
- `python.ruff`
- `python.mypy`
- `python.pyright`
- `python.uv`
- `package.wheel`
- `package.sdist`

These should be concept-aware template overlays, not a combinatorial set of hardcoded templates. A future `python-cli` template should emit `manifest.tsx` plus a small projected `pyproject.toml` only when projection support exists.

## Implementation phases

### Phase 0: Design only

- Add this document.
- Update the roadmap to describe Python as exploratory and not implemented.

### Phase 1: Manifest frontend API sketch only

- Add experimental TypeScript declaration stubs only after parser/collector preservation exists.
- Add the smallest backend seam types needed to prevent Python conditionals from spreading through npm code.
- Prove `PythonPackage`, `PyPI`, and `PythonRuntime` declarations compile and round-trip into experimental IR.
- Do not add resolver behavior.

### Phase 2: Python template projection prototype

- Prototype `tspack init --template python-cli` or equivalent only as experimental.
- Emit `manifest.tsx` plus `pyproject.toml` projection.
- No PyPI resolution.

### Phase 3: uv-backed experimental update/sync

- Delegate resolution and materialization to `uv`.
- Import `uv` lock/report data into TSPack diagnostics.
- Preserve TSPack as the project contract and policy owner.

### Phase 4: policy/check integration

- Check that projections match the manifest contract.
- Add Python-specific security audit categories for build backends, source distributions, native wheels, hashes, and indexes.

### Phase 5: pack integration

- Verify wheel and sdist artifacts.
- Keep packaging behavior explicit and auditable.

## Open questions

- What is the smallest backend interface that avoids smeared ecosystem conditionals without over-abstracting before Python behavior exists?
- Should the public API use `PythonPackage(...)` or a generic `Package({ ecosystem: "pypi", ... })` once IR is ecosystem-aware?
- Should `PyPI(...)` include extras in the source helper, e.g. `PyPI("fastapi", spec, { extras: [...] })`, or should extras live on the dependency intent?
- How much `pyproject.toml` should TSPack project before it becomes too much config ownership?
- Should `uv.lock` remain external, be imported into `ts-lock.toml`, or be summarized as provenance?
- What is the minimum security model that provides value without pretending all Python build risk is solved?
- How should cross-ecosystem workspaces handle packages that generate TypeScript clients from Python services or vice versa?

## Outcome

Outcome A for M57a is design success: the design is documented, Python is explicitly experimental, no `manifest.py` is introduced, and no current TypeScript/npm behavior changes.
