# TSPACK-TSCL-SURVEY-M0: Copeland TS compiler integration and native package strategy

## Status

**Complete design survey. No product defaults, package resolution behavior, or
public command has changed.** This record is based on the checked-out TSPack
and Copeland repositories on 2026-07-26.

Copeland TS is a TypeScript implementation and language profile. `tscl` is its
compiler identity; it is not a JavaScript runtime. Node, Bun, Deno, and the
browser remain hosts for emitted JavaScript. Vite remains a browser
development/build host.

## Executive recommendation

Use **NuGet with Copeland metadata conventions** as the canonical distribution
and acquisition system for native Copeland libraries. Keep normal NuGet restore,
framework/RID selection, transitive dependencies, lock files, cache, signing,
and publication. Add one explicit, versioned Copeland package contract file;
do not infer Copeland modules by scanning package contents or installation
paths.

For the first native release, distribute a **CLR binary plus Copeland module
metadata**. Source and MIR are optional later package realizations, not M1
requirements. An optional npm publication contains generated production ESM and
generated JavaScript package metadata; npm is not the canonical representation
of a native Copeland library.

The next implementation should be **COPELAND-NUGET-M1**, not a broad TSPack
rewrite. Existing MSBuild integration already receives NuGet/ProjectReference
assemblies through `ResolveReferences`; the missing native capability is a
versioned package module contract that lets `import` discover Copeland exports.
Once that is proven, **TSPACK-TSCL-M1** should add a deliberately small compiler
selection and Node production-JS path.

## Repositories and evidence inspected

### TSPack

- `internal/manifest/ir.go` defines `Workspace.Runtime` only as `nodejs`,
  `bun`, or `deno`; there is no compiler field or adapter contract. Its package
  `Target.Runtime` is an npm runtime-entrypoint path, not host selection.
- `docs/runtime-profiles.md` and `docs/run.md` establish Node/Bun/Deno as
  process-launch adapters. `RunTarget` may select `system`, `node`, `bun`, or
  `deno`; runtime profile and dependency ownership are intentionally separate.
- `internal/concepts/builtin_registry.generated.go` contains literal `tsc`
  commands for `typescript.library` and makes Vite concepts require TypeScript
  concepts. This is the primary current compiler conflation.
- `manifest-frontend/src/tspack-manifest.d.ts`, `TsConfig.manifestEditor`, and
  compatibility-file handling in `internal/compat` manage
  `tsconfig.tspack.json` and VS Code's TypeScript SDK setting.
- `internal/resolver`, `internal/materialize`, `internal/lockfile`, and
  `docs/lockfile.md` implement TSPack's npm-oriented graph, `ts-lock.toml`,
  store, and `node_modules` materialization. They do not implement NuGet.
- `internal/testcmd/testcmd.go` and `docs/test-command.md` select xTest and
  Vitest only. `docs/pack.md` creates deterministic npm `.tgz` artifacts and
  can generate an in-archive `package.json`.

### Copeland

- `src/Copeland/Copeland.Cli/Program.cs` exposes `copeland compile <file>
  --emit mir|csharp|javascript`, with `diagnostic`, `symbolic`, and
  `production` JavaScript profiles. It is a single-source artifact CLI, not a
  project `tscl build` contract, and it has no JSON diagnostics, watch,
  source-map, package-graph, or multi-module command surface.
- `src/Copeland/Copeland.TS/Compiler/CopelandProjectCompiler.cs` already owns
  local relative module graph construction, named local imports/exports, and
  project MIR. This is the right semantic seam for package-module imports.
- `src/Copeland/Copeland.TS.MSBuild/build/Copeland.TS.Sdk.targets` runs before
  `CoreCompile`, depends on `ResolveReferences`, and passes
  `@(ReferencePath)`/`@(ReferencePathWithRefAssemblies)` to the task. The
  existing `.csproj` route therefore already inherits NuGet and
  `ProjectReference` assembly resolution.
- `docs/decisions/copeland-msbuild-cts-msbuild-m1.md` documents generated
  `.cope` and `.g.cs` under `obj`, ordinary `dotnet build/run/test/publish`,
  and `build`/`buildTransitive` packaging intent for the SDK.
- `src/Copeland/Copeland.TS/Compiler/CopelandNpmContracts.cs` and
  `docs/Copeland/architecture/copeland-npm-import-boundary-cts-npm-m1.md`
  intentionally accept an already-resolved, materialized npm graph. They are
  compiler input, not a package manager. The C# backend also has bounded
  sidecar contracts; TSPack does not yet orchestrate their npm/Node lifecycle.
- `src/Copeland/Copeland.TS.Backend.JavaScript/JavaScriptEmissionProfile.cs`
  has explicit Node and Browser runtime targets, but the current CLI does not
  expose that target selection or emit a project/package layout.

## Current TSPack architecture

TSPack is an npm-oriented project lifecycle manager. A TypeScript/TSX manifest
is evaluated by its Node-based frontend into manifest IR; the Go process
validates intent, resolves and locks package sources, materializes npm packages,
and runs declared process targets. Its package output is an npm-compatible
archive. TSPack makes no general build-compiler invocation today: generated
concepts install tools and declare `RunTargets` such as `tsc` or `vite build`.

The good existing seams are real and small:

```text
manifest intent -> npm graph/ts-lock.toml -> node_modules materialization
                                             -> RunTarget runtime adapter
                                             -> npm pack artifact
```

Node, Bun, and Deno are already correctly close to runtime adapters in the
`run` path. The conceptual problem is that the built-in `typescript.*` concepts
bundle compiler tooling, `tsconfig`, and Vite concepts, while the manifest has
no peer selection point for a non-`tsc` compiler.

TSPack is therefore **not compiler-neutral today**, but it has a tractable
adapter seam. It should not be refactored into a framework before a second
compiler actually needs each operation.

## Disposition A: current TSPack assumptions

| Subsystem | Current assumption | tsc-specific? | Runtime-specific? | Copeland impact | Recommended change |
| --- | --- | --- | --- | --- | --- |
| Manifest IR | No compiler field; workspace runtime is JS-only | Neutral by absence | Yes | Cannot select `tscl` as a peer of `tsc` | Add a small compiler selection at package/output scope; do not overload runtime |
| Built-in concepts | `typescript.*` installs npm `typescript`; library run targets literally execute `tsc -p` | Yes | Indirectly Node-local bin | Must not reuse these commands for `tscl` | Add a separate `copeland-ts.*` concept/adapter; preserve existing concepts |
| `tsconfig.tspack.json` | Generated compatibility/editor file and VS Code TypeScript SDK | Yes | Node/editor tooling | `tscl` should not pretend this is its project truth | Keep only for projects that select `tsc`; introduce a Copeland config contract later |
| Package targets | `entry`, `runtime`, and `types` describe npm export files such as `dist/index.js` and `.d.ts` | Reusable output model | JavaScript package model | Usable for npm compatibility output, not native CLR package truth | Retain for npm only; model native outputs through `.csproj`/NuGet |
| JavaScript emission | Assumes build tools leave files matched by explicit `Publish` policy | Reusable | JavaScript package model | `tscl` production ESM can satisfy it eventually | Require explicit emitted file map; do not infer output directories |
| Declaration files | npm `types`/generated `package.json` conventions | tsc/ecosystem-shaped | No | Copeland contracts are not necessarily `.d.ts` | Make declarations optional JS compatibility output, not native module truth |
| Source maps | No compiler-level source-map handling found | Neutral/absent | No | A future adapter needs source-map locations | Defer until project JS emission exists; make source map output explicit |
| Watch | TSPack watches only native xTest files; it does not own compiler watch | Neutral/absent | Node bridge for xTest | Cannot promise `tscl --watch` | Add a compiler-watch protocol only after one-shot build proof |
| Errors | Go diagnostics plus tool process exit; no standardized compiler diagnostics parser | Reusable seam | No | `tscl` needs stable machine-readable diagnostics | Define JSON-lines or JSON result contract before IDE/watch integration |
| Runtime launch | Explicit Node/Bun/Deno/system `RunTarget` adapters | Neutral | Yes, correctly | Node can run emitted production JS unchanged | Reuse unchanged |
| Browser/Vite | Concepts install Vite and issue `vite build`; Vite requires TypeScript concepts | Yes at concept layer | Browser host | No existing tscl plugin or output hand-off | Defer to a Vite proof after Node; decouple Vite from `typescript.*` concept |
| Tests | xTest and Vitest auto-detection | JavaScript-tooling-specific | Node | Copeland CLR tests are xUnit/dotnet tests | Add target-aware orchestration later; do not route CLR tests through npm |
| Lock/cache | `ts-lock.toml`, npm resolver/store, `node_modules` | npm-specific | No | Cannot represent NuGet restore faithfully | Let `dotnet restore` own native graph and lock |
| Publishing | deterministic npm `.tgz` and generated in-archive `package.json` | npm-specific | No | Useful for optional JS compatibility package | Add a distinct NuGet publisher/orchestrator later; never make npm canonical |
| Workspaces | TSPack workspace/package manifests | package manager-specific | No | Useful application orchestration, not required by `.csproj` | Keep separate from native project graph |

## Minimal compiler/runtime separation

The smallest enduring split is ownership, not a large class hierarchy:

| Concern | TSPack | tscl / Copeland | NuGet | npm | Node/Bun/Deno/Vite |
| --- | --- | --- | --- | --- | --- |
| Dependency resolution | npm graph only | Consumes resolved contracts | Native restore | JS restore | Consume materialized JS graph where applicable |
| Locking | `ts-lock.toml` for npm | Reads resolved inputs | `packages.lock.json`/assets | npm lock inputs/compat graph | None |
| Source compilation/type checking | Selects/invokes compiler | Owns language semantics and diagnostics | None | None | Vite may transform/bundle emitted JS only |
| CLR emission | Orchestrates only if requested later | MIR/C# generation | Resolves framework/package assets | None | `dotnet` hosts assembly |
| JS emission | Materializes/packs output later | Production ESM and source maps | None | Distributes compatibility output | Node/Bun/Deno execute; Vite serves/bundles |
| Package contracts | Carries selected JS contracts | Defines Copeland imports/exports/identity | Carries native contract file | Carries JS export map/contracts | Uses standard host semantics |
| Runtime launch | Declared `RunTarget` | Does not become a runtime | None | None | Own process/host behavior |
| Browser serving/assets | Application orchestration | Emits/reports asset requirements | None | Installs JS assets | Vite/static host owns serving/bundling |
| Tests | Selects target command later | Generates/validates Copeland test output | supplies xUnit packages | supplies JS test packages | dotnet/xUnit, Node test host, or browser harness execute |
| Publishing | Can orchestrate distinct outputs | Generates artifacts/metadata | Native package publication | JS compatibility publication | None |
| Typed Node sidecar | Future lifecycle/package orchestration | Typed call syntax, contracts, generated stubs | Packages CLR side | Provides npm graph | Node executes actual dependency |

M1 only needs explicit data named `compiler`, `runtime`, `browserHost`,
`dependencySource`, and `publisher` at their real ownership points. It does
not need a generic adapter framework. A pragmatic internal shape after evidence
requires it is `CompilerAdapter { tsc, tscl }`, existing `RunTarget` runtime
selection, an optional `BrowserHostAdapter { vite }`, and separate npm/NuGet
publish commands.

## `tscl` selection and minimal adapter

Use `tscl` as the public executable and compiler vocabulary. The existing
binary is named `copeland` and only understands `compile`; it should eventually
provide a `tscl` shim/alias with a project-oriented command family. Do not
rename Node/Bun/Deno selections.

The proposed common-case manifest direction is deliberately additive:

```tsx
<Package name="example" version="1.0.0" kind="app" compiler="tscl">
  <RunTargets rows={[{
    name: "start",
    runtime: "node",
    command: ["node", "artifacts/node/main.js"],
  }]} />
</Package>
```

`compiler="tsc"` is the compatibility default for existing TypeScript concept
templates. `compiler="tscl"` means Copeland TS source and a Copeland compiler
build contract. Native NuGet dependencies remain in the project `.csproj`; the
TSPack manifest should not duplicate them merely to restate `PackageReference`.
An application that has both CLR and Node outputs may declare separate outputs
with their own compiler target/options, but a runtime is still selected by the
launch target, not by the compiler.

The first `tscl` adapter must receive these stable inputs:

- project root, ordered source roots, and logical module paths;
- selected entry module/export and output directory;
- explicit target (`javascript` initially) and JavaScript realization (`node`
  for the first proof);
- already-resolved npm contract graph for bare npm imports;
- resolved Copeland/NuGet package contract paths where present;
- declared assets and their normalized paths;
- output-file manifest, source-map locations, and a machine-readable diagnostic
  result;
- input fingerprint/build key and a declared watch capability.

Neither a generated `tsconfig.json` nor an MSBuild invocation is the correct
Node adapter contract. Generate a Copeland-owned configuration/result file only
if command-line arguments become unwieldy. MSBuild remains the native CLR
integration path. A direct compiler API is appropriate behind the `tscl` CLI,
not as TSPack's first cross-process dependency.

Current evidence means this adapter cannot be a mere executable substitution:
the CLI accepts one source file and lacks project modules, npm contracts,
target selection, source maps, watch, and structured diagnostics. The existing
`CopelandProjectCompiler` is the implementation seam for a future `tscl build`
command.

## Expected host flows

### Node proof

```text
TSPack manifest selects compiler tscl and a Node output
  -> TSPack restores/materializes existing npm graph
  -> TSPack supplies the resolved npm contract projection to tscl build
  -> tscl emits production Node ESM and an output manifest
  -> existing Node RunTarget launches node artifacts/node/main.js
```

Node does not change. The first proof should not require authored
`package.json` or `tsconfig.json` unless a selected external package requires
standard npm metadata; the generated compatibility package manifest is an
output, not compiler input.

### Bun and Deno

The production ESM proof can later reuse the existing Bun and Deno launch
adapters only after `tscl` declares that its realization is host-compatible.
Do not infer Deno support from Node ESM: Deno permissions, import map/`npm:`
behavior, and module resolution remain host-specific. Bun is closer to Node but
still needs a compatibility test. TSPack's existing `runtime="bun"` and
`runtime="deno"` remain unchanged.

### Vite/browser

Vite is not a compiler selection and should not be asked to consume Copeland
source directly in M1. It may consume emitted browser ESM once `tscl` exposes a
browser output map, source maps, and stable watch/invalidation semantics. The
current Vite concepts run `vite build` and assume the TypeScript concept; there
is no plugin, transform API, or HMR hand-off. Therefore defer browser support:

```text
tscl browser build/watch -> emitted ESM + source maps -> Vite serve/bundle
```

The later experiment must decide whether Vite owns module invalidation while
`tscl` watches/updates outputs, or whether a Vite plugin calls a stable tscl
project API. Do not claim that replacing `tsc` is enough for a Vite source tree.

### CLR

```text
.csproj + PackageReference + CopelandCompile
  -> dotnet restore resolves NuGet graph
  -> buildTransitive targets expose Copeland contract paths
  -> Copeland MSBuild task resolves imports and writes obj/Copeland MIR/C#
  -> ordinary Roslyn CoreCompile -> dotnet run/test/publish
```

A native Copeland CLR project can, and usually should, work without TSPack.
That is the existing SDK-style contract and avoids ceremony for small libraries
and applications. TSPack remains valuable for multi-output applications,
JavaScript compatibility graphs, browser assets, workspace orchestration, and
cross-host launch/publishing; it must not replace `dotnet restore` or MSBuild.

## NuGet verdict and restore law

### Verdict: use NuGet with Copeland metadata conventions

NuGet already supplies native package identity/versioning, transitive restore,
framework selection, RID runtime assets, project references, global/offline
caches, lock files, package publication, signing/provenance support, `lib`,
`ref`, `runtimes`, `analyzers`, `contentFiles`, and `build`/
`buildTransitive` package assets. Copeland's checked-in SDK already packages
MSBuild props/targets under `build` and expects normal resolved reference paths.

NuGet does **not** define Copeland authored imports, exported module names,
MIR compatibility, target realizations, nominal identity, or JavaScript
materialization. Those must be explicit Copeland metadata. Thus
`PackageReference` makes package assets available; it never by itself makes a
Copeland `import` valid.

Use normal `dotnet restore`. Native projects may opt into NuGet's
`packages.lock.json` locked mode according to existing .NET policy; authoritative
resolved inputs are `obj/project.assets.json`, `@(ReferencePath)`, and
package-provided build items. TSPack must neither generate PackageReferences nor
parse/reimplement the NuGet solver in M1. It may later invoke `dotnet restore`
and summarize the resulting native closure for application packaging.

There are two intentionally coordinated locks, not a fictional unified lock:

| Graph | Authority | Reproducible input/output |
| --- | --- | --- |
| Native CLR/Copeland | NuGet/dotnet | `.csproj`, `packages.lock.json` when enabled, `project.assets.json` |
| npm compatibility/sidecar | TSPack today | manifest intent plus `ts-lock.toml` and materialized store |

An application manifest may name both graphs and record their selected output
profiles, but it must point to their authoritative locks. A combined release
manifest is a report, not a second resolver or lockfile cathedral.

## Native Copeland NuGet package contract

### M1 package shape: binary plus metadata

```text
lib/net10.0/Example.Copeland.dll
ref/net10.0/Example.Copeland.dll                 (when separately produced)
buildTransitive/Example.Copeland.Copeland.targets
copeland/contract.v1.json
```

The regular assembly/ref assets are selected by NuGet. The build-transitive
target contributes a `CopelandPackageContract` item whose absolute path is
derived from `$(MSBuildThisFileDirectory)`; it does not guess a global NuGet
cache path. `CopelandCompile` receives these items alongside normal resolved
CLR reference paths. The targets may validate that the contract is present and
schema-compatible, but should not compile source or restore packages.

An M1 package does not need `contentFiles`, analyzers, source, or MIR. They are
available NuGet transport choices when a later feature needs them:

```text
copeland/contracts/*.json     versioned module contracts
copeland/mir/*.cope           optional portable MIR realization
copeland/source/**/*.ts       optional source realization
copeland/js/node/**/*.js      optional generated Node ESM realization
copeland/js/browser/**/*.js   optional generated browser ESM realization
```

The first metadata file should be one explicit `contract.v1.json`, not a
scatter of custom MSBuild properties. The proposed bounded shape is:

```json
{
  "schemaVersion": 1,
  "package": { "id": "Example.Copeland" },
  "compiler": { "minimum": "1.0" },
  "assemblies": [{ "tfm": "net10.0", "identity": "Example.Copeland" }],
  "modules": [{
    "specifier": "example/parser",
    "exports": [{ "name": "Parse", "kind": "function", "contract": "contracts/parser.json" }],
    "nominalScope": "Example.Copeland/example/parser",
    "realizations": {
      "clr": { "kind": "binary", "assembly": "Example.Copeland" }
    }
  }]
}
```

NuGet remains the authority for resolved package version and physical assets,
so the contract does not duplicate hashes, transitive dependencies, or install
paths. Future JavaScript and portable entries extend `realizations`, for
example `node`, `browser`, `source`, or `mir`, with normalized package-relative
artifact paths and required compiler capability versions.

### Package mode decision

| Mode | M1 disposition | Rationale |
| --- | --- | --- |
| Binary: assembly + metadata | **Adopt** | Stable CLR consumption, ordinary C#/F# interop, smallest package/import proof |
| Source package | Defer | Better cross-backend optimization/diagnostics, but requires source compatibility and consumer compilation law |
| MIR package | Defer | Potential portable lowering/inlining path, but requires stable MIR schema/compiler-version policy |
| Hybrid | Defer as a later publishing profile | Useful for dual CLR/JS libraries, but would hide too many ABI/versioning decisions in M1 |

This retains nominal identity through the resolved package identity plus
assembly identity and the metadata module/declared-symbol identity. A compiled
consumer must not treat same-spelled types from distinct package assemblies or
module scopes as interchangeable. Contract validation must check that exported
Copeland identities agree with the selected assembly surface; generated C#
continues to reference the assembly identity selected by NuGet.

### Disposition C: package strategy

| Package type | Canonical registry | Canonical artifact | Optional compatibility artifact | Consumer flow |
| --- | --- | --- | --- | --- |
| Native Copeland library | NuGet | CLR assembly plus `contract.v1.json` | Generated ESM/npm package | `PackageReference` plus Copeland `import` contract discovery |
| CLR-only library | NuGet | Standard `lib`/`ref` and optionally a contract only when it exposes Copeland modules | None | `using` and ordinary CLR references; no Copeland import required |
| Ordinary npm dependency | npm/TSPack graph | JavaScript package and its normal metadata | None | TSPack resolves/materializes it; Copeland consumes only declared static npm contracts |
| Dual-target Copeland library | NuGet for native; npm for JS | NuGet binary/contract | Production Node/browser ESM with generated `package.json` | Consumer selects a declared CLR or JS realization; neither registry substitutes for the other |
| Node-sidecar dependency | npm for the executable dependency, NuGet for CLR app/library | npm package plus Copeland boundary contract | Packaged Node sidecar application payload | CLR uses generated typed boundary; Node loads and executes package |
| Browser package | npm | Browser ESM, assets, export conditions | None | Vite/static host consumes generated browser realization |
| Application package | Host-appropriate publish path | `dotnet publish`, Node output, or browser bundle | Optional release manifest describing both closures | Deployment selects one concrete host output, not a library registry package |

## `import`, `using`, and target realization law

Copeland language law remains:

```ts
import { Parse } from "example/parser"; // Copeland module contract
using Example.Runtime;                  // CLR namespace/type visibility
```

For a bare specifier, the compiler resolves in this order only after local
relative-module resolution has ruled out a relative import:

1. Check the resolved Copeland package-contract item map for an exact exported
   module specifier.
2. Validate schema/compiler range, unique module ownership, declared named
   export, and target realization for the selected backend.
3. Bind the module's Copeland contract and preserve its package/module/nominal
   identity in bound state and MIR.
4. For CLR, emit direct references to the selected NuGet assembly; for JS,
   emit only the declared realization's ESM specifier/path.

Existing npm contract lookup remains a separate declared source. A package name
collision between npm and a Copeland NuGet module is an authored ambiguity
diagnostic; selection must be explicit in the compiler configuration/manifest,
never inferred from cache layout. The compiler must issue dedicated diagnostics
for missing contract, duplicate module, incompatible compiler schema, missing
named export, and missing realization. It should reuse existing local-module
and npm-boundary source spans where they identify the import.

`using` does not participate in this module resolver. It uses the existing CLR
metadata resolver over the `ReferencePath` closure. A package may expose both
a Copeland module contract and public CLR namespaces, but their lookup spaces
remain independent.

Target availability is package-declared rather than guessed:

| Realization | Meaning | Consumer behavior |
| --- | --- | --- |
| `clr.binary` | TFM-selected NuGet assembly implements module | Valid only for CLR build with compatible asset |
| `js.node` | Generated Node ESM implementation | Valid only for Node-compatible JS build |
| `js.browser` | Generated browser ESM implementation | Valid only for browser build |
| `source` | Copeland source can be compiled by consumer | Requires compatible compiler/profile and explicit source policy |
| `mir` | Stable portable MIR can be lowered by consumer | Requires exact declared MIR/compiler compatibility |

A package may list any subset. For example, a CLR-only package produces a
targeted error when imported in Node output, rather than a late host failure.
M1 implements `clr.binary` only; metadata reserves the other names without
claiming support.

## npm compatibility and generated `package.json`

npm retains ownership of JavaScript dependencies and JavaScript-compatible
publication artifacts. A native Copeland library is not required to publish to
npm. When it chooses to do so, the producer flow is distinct:

```text
Copeland source -> tscl production ESM + output contract
                -> generated package.json, export map, declarations/contracts,
                   source maps, and assets -> npm package
```

The npm package name, version policy, export map, Node/browser conditions, and
relation to the NuGet package belong in a publication profile. Versions should
normally be released from the same source version but remain independently
addressable registries; a package metadata field may state the corresponding
NuGet ID/version for provenance. JavaScript consumers receive normal ESM and
JavaScript-facing contracts, not a requirement to install the native package.

TSPack's current `pack` implementation already treats a generated in-archive
`package.json` as a deterministic output when authored `package.json` is not
selected. That is the correct direction for a Copeland compatibility package.
It does not override current TSPack adoption behavior, where existing
`package.json` remains authoritative for an adopted JS project. For a new
Copeland publication profile, generated `package.json` is an output artifact;
the Copeland project/publication declaration is source of truth.

## Testing and publishing law

Testing is target-owned:

| Compiler/output | Test owner |
| --- | --- |
| tscl + CLR | ordinary `dotnet test` and xUnit; `.tsxtest` participates through the Copeland SDK |
| tscl + Node | explicitly selected Node/Copeland JS test host after JS materialization |
| tscl + browser | real browser harness, not a Node-only proxy |
| ordinary TSPack project | current xTest/Vitest orchestration |

Do not auto-detect a Copeland CLR project as Vitest/xTest. TSPack may later
orchestrate the selected commands, but it must retain the underlying platform's
test discovery/reporting contract.

Publishing is likewise four explicit outputs:

1. native Copeland library to NuGet;
2. optional JavaScript compatibility library to npm;
3. CLR application through `dotnet publish`;
4. Node or browser application output through its selected host/build process.

An eventual `tspack publish` must require named publication targets (for
example `publish native` and `publish npm`) rather than silently dual-publish.

## Typed CLR-to-Node sidecar: bounded future work

The existing Copeland npm boundary has useful static named-function contracts
and C# sidecar foundations. It intentionally does not make TSPack own package
acquisition or a complete application lifecycle. A follow-up
**CTS-NPM-SIDECAR-M0** should inventory those existing contracts and prove one
end-to-end application bundle before new abstractions.

TSPack would own npm restore/materialization, sidecar output layout, process
startup/reuse, combined application packaging, and lock coordination. Copeland
would own typed contract projection, call syntax, boundary validation,
serialization contract, generated stubs, and diagnostics. Node would own npm
package loading, callbacks/stream behavior where supported, and JavaScript
execution. The discovery must decide async calls, errors, cancellation,
callbacks, streams, initialization, restart policy, serialization limits, and
provenance. It is not part of this survey's implementation scope.

## Exact missing capabilities

### TSPack

- no manifest compiler selection or compiler adapter seam;
- literal `tsc` commands and TypeScript/Vite concept coupling;
- no `tscl` project invocation, output manifest, diagnostic parser, cache key,
  or compiler watch protocol;
- no NuGet dependency source, restore invocation/reporting, or native
  publication target (and it should not implement a NuGet resolver);
- no target-aware test selection for dotnet/xUnit or browser harnesses;
- no browser emitted-ESM/Vite hand-off or source-map invalidation model;
- no Copeland package publication profile or generated compatibility artifact
  contract;
- no application-level npm/CLR sidecar packaging/process orchestration.

### Copeland

- no public `tscl` executable/alias or project `build/run/test/emit` contract;
- CLI is single-source and has no stable JSON diagnostics, project graph input,
  npm graph input, source-map/watch/incremental result protocol, or exposed JS
  runtime target;
- no NuGet Copeland package contract schema, pack target, build-transitive
  contract item, or package-import resolver;
- no target realization metadata/diagnostics for CLR versus Node/browser;
- no source/MIR package compatibility/version policy;
- no generated npm compatibility package/publisher;
- no Vite plugin or emitted-output integration;
- no complete sidecar application lifecycle/packaging model.

## Recommended milestone sequence

1. **COPELAND-NUGET-M1 — native Copeland package contract and
   `PackageReference` import proof.**
2. **TSPACK-TSCL-M1 — compiler selection plus Node production-JS proof.**
3. **TSPACK-TSCL-BROWSER-M1 — emitted browser ESM/Vite materialization proof.**
4. **TSPACK-COPELAND-PUBLISH-M1 — NuGet native publication plus optional
   generated npm compatibility artifact.**
5. **CTS-NPM-SIDECAR-M0 — typed CLR/Node application-boundary discovery.**

NuGet comes first because it completes the missing native module identity law
on top of a functioning `.csproj`/`ResolveReferences` path. TSPack Node work
needs a project-shaped `tscl` contract that does not yet exist; it should build
on, rather than invent independently of, the package contract.

### Exact next milestone: COPELAND-NUGET-M1

Scope:

```text
Producer Copeland SDK project
  -> dotnet pack creates local NuGet package with lib asset,
     contract.v1.json, and buildTransitive item

Consumer Copeland SDK project
  -> PackageReference restores local package
  -> import { Parse } from "example/parser"
  -> CopelandCompile discovers contract through MSBuild item
  -> CLR build/run proves exported function
  -> using Example.Runtime still binds through normal reference metadata
```

Acceptance criteria:

- `dotnet restore --locked-mode` (where the test fixture enables a lock file)
  restores only through normal NuGet machinery;
- contract discovery never scans global NuGet cache paths;
- one M1 binary package exposes one named Copeland module and named export;
- a consumer resolves the import, emits/compiles CLR code, and executes it;
- C# and Copeland `using` interop remains intact;
- a Node-selected compilation receives a clear missing-realization diagnostic
  for the CLR-only package;
- duplicate/missing module and schema-incompatible package diagnostics are
  covered;
- no source/MIR consumption, npm publication, TSPack resolver change, or
  sidecar implementation enters the milestone.

## Risks and deliberate deferrals

- Copeland's local-module graph is real, but package imports are not yet an
  implemented semantic category. Do not reuse npm contract semantics without
  preserving native module and nominal identities.
- The current CLR source model emits generated C# in the consumer project.
  A package binary needs a clear public contract and ABI/version policy before
  source/MIR delivery is attempted.
- Production JavaScript exists, but a one-file CLI is not evidence of npm
  package layout, Node project graph, declaration production, source maps, or
  Vite HMR support.
- NuGet asset choice is TFM/RID-aware; package metadata must not override or
  duplicate that selection. It should describe Copeland semantics only.
- TSPack's `ts-lock.toml` and NuGet lock files solve different graphs. A unified
  resolver would duplicate mature tooling and is expressly deferred.

## Survey conclusion

The design test passes with these bounds: a user can select `tscl` where a
TSPack project would select `tsc`, keep Node/Bun/Deno/Vite as hosts rather than
compiler identities, use NuGet as the native Copeland package system, and
publish npm only for JavaScript compatibility artifacts. That path requires the
explicit Copeland NuGet package contract and a project-shaped `tscl` build
contract; it does not require a new runtime, a second NuGet resolver, or broad
TSPack implementation in M0.
