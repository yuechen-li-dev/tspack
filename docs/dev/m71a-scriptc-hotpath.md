# M71a ScriptC hot-path targets

## Status and product boundary

M71a proves that an ordinary TypeScript project can assign one bounded source
set to ScriptC without making ScriptC the package-wide compiler. The supported
bridge is a native executable sidecar consumed as an explicit target artifact.

> ScriptC is a target compiler, not a project-wide mode.

> TSPack lets projects adopt ScriptC where native compilation is useful while
> leaving ordinary TypeScript on ordinary tooling.

ScriptC ownership is a compiler boundary, not merely a code-generation
preference. A normal TypeScript target may not import ScriptC-owned source. It
must consume the artifact through a coarse protocol such as the batched JSON
sidecar used by the M71a fixture.

## Why the sidecar bridge

ScriptC 0.0.35 exposes several distinct artifacts:

- `build --lib --profile` produces a host-callable static C archive with
  profile-declared exports and an optional contract sidecar. This is the right
  interface for a native C/C++/Swift/Kotlin host, but Node cannot load it
  without an N-API/addon layer.
- `--emit=obj --print=native-link-info` produces a program object defining
  `main`, plus `scriptc.native-link-info.v1`. It is not a host-callable library,
  and the helper lane is currently macOS 15+ arm64.
- WASI produces an executable WASI Preview 1 module. It does not provide the
  simple host-callable function interface needed for this Node fixture.
- a native executable works on Windows x86_64 through ScriptC's documented
  `zigcc` plus `x86_64-windows-gnu` target.

Inventing N-API glue or reproducing ScriptC's private runtime ABI would be a
larger, less trustworthy subsystem. M71a therefore uses a sidecar and makes its
cost visible. The call must be batched: one native process per tiny function
call is explicitly outside the useful boundary.

Copeland's interop work informed this choice. Its durable pattern is a bounded,
versioned contract carrying resolved project truth, direct compiler-owned
binding/lowering, and a narrow host projection. It does not mix source
semantics, discover a second dependency graph, or use reflection/stringly
runtime lookup. The ScriptC target follows the same law with a versioned
`scriptc-v1` compiler payload and an explicit artifact dependency.

## Manifest and config ownership

The project manifest owns source and artifact selection:

```tsx
<Targets rows={[
  {
    name: "app",
    compiler: "tsc",
    inputs: ["src/app/**"],
    dependsOn: ["hotpath"],
    entry: "src/app/main.ts",
    runtime: "dist/app/main.js",
  },
  {
    name: "hotpath",
    language: "scriptc",
    compiler: "scriptc",
    compilerConfig: "scriptc.json",
    inputs: ["src/hot/**"],
    artifact: "nativeExecutable",
    entry: "src/hot/compute.ts",
    runtime: "dist/hotpath.exe",
  },
]} />
```

The ScriptC config owns ScriptC-specific choices:

```json
{
  "schemaVersion": 1,
  "backend": "llvm",
  "optimization": "release",
  "dynamic": false,
  "cc": "zigcc",
  "target": "x86_64-windows-gnu"
}
```

`backend`, `optimization`, `dynamic`, `npmStatic`, `cc`, and `target` are never
generic `CompilerTarget` fields. The adapter carries them in the bounded
`scriptc-v1` payload and translates them to ScriptC's CLI/environment contract.
Unknown config fields fail rather than being silently ignored. `npmStatic`
accepts explicit package names only; `auto` is rejected because it would make
target package visibility depend on compiler discovery. Provenance-source
fetching is not exposed because it would create an independent network/source
authority during compilation.

## Ownership and build graph

Every ScriptC target requires explicit `inputs`. TSPack expands all explicit
target patterns before building and rejects a file owned by more than one
target with `TSPACK_COMPILER_SOURCE_OVERLAP`. Declaration order never resolves
an overlap. Relative imports crossing different compiler owners fail with
`TSPACK_COMPILER_CROSS_TARGET_SOURCE_IMPORT`.

`dependsOn` is topologically ordered. Building `app` first builds or reuses the
`hotpath` artifact, then runs `tsc`. Cycles and unknown target references fail.
The normal `tsconfig.json` remains compiler-owned. TSPack writes a small derived
config under `.tspack/compiler-configs` whose rebased `exclude` list removes
ScriptC-owned inputs from TypeScript's include expansion. Direct imports would
still pull an excluded file into a tsc program, which is why the separate
cross-compiler import diagnostic is required.

Projects with no ScriptC target retain the existing tsc path and do not invoke,
scan for, or require ScriptC.

## Adapter, tool, package, and artifact truth

The ScriptC adapter advertises only the exercised surfaces: parse, type-check,
native executable emission, compiler-owned config, static coverage, and
explicit dynamic fallback. It does not advertise object, library, Wasm, run,
or external-link-recipe support through TSPack M71a even though ScriptC has
some of those standalone surfaces.

ScriptC is a normal `tool(npm("scriptc", "0.0.35"))` dependency selected by
`<Tools>`. `update` resolves it through normal source policy, `sync` materializes
it, and the adapter only invokes the resulting `node_modules/.bin/scriptc`.
An explicit target `compilerPath` is reported as `source: path`; it is never
silently mixed with the managed tool.

Only target-reachable `deps` become compiler package bindings. `npmStatic` must
name an explicit visible package. Normal dependencies remain TSPack-resolved;
the adapter does not run `npm install` or create a second package graph.

The compiler descriptor records exact ScriptC version, tool authority/path,
config and source fingerprints, visible package bindings, target triple,
capabilities, native executable output, and coverage metadata output. ScriptC
coverage text is preserved byte-for-byte under `.tspack/compiler-metadata`.
Native-link metadata is not claimed for the sidecar lane; it belongs to a future
object/library integration that actually requests it.

Builds write to a same-directory staging path and replace the declared
executable only after ScriptC succeeds. A failed coverage or build leaves the
last good artifact unchanged. Cache identity includes the descriptor (tool
version, config, owned source fingerprints, visible packages, runtime target,
outputs) plus the host OS/architecture. Unrelated app files are absent, so they
do not rebuild the hot path.

## Static policy and diagnostics

Static-only is the default. TSPack runs `scriptc coverage` before a cold build,
stores the report, and inspects ScriptC's own `runs with --dynamic` evidence.
Coverage exit status alone is insufficient: ScriptC intentionally returns
success for a report containing dynamic-only sites. A static target with such
sites fails as `TSPACK_COMPILER_STATIC_COVERAGE_REQUIRED` and retains ScriptC's
SC diagnostic/code frame. Setting `dynamic: true` is the only opt-in to the
embedded engine.

This policy makes acceleration intent honest. It does not reinterpret or repair
unsupported ScriptC semantics.

## Real dogfood and measurements

`fixtures/scriptc-hotpath-m71a` is an existing-TypeScript-shaped project. Its
Node app is built by TypeScript 5.9.3. Only `src/hot/compute.ts` is built by the
managed `scriptc@0.0.35` tool. The app spawns the artifact once, receives one
flat JSON record, compares four checksums, and reports kernel and boundary time.

Windows x64 proof used Node 26.2.0, Zig 0.16.0, and the documented
`x86_64-windows-gnu` ScriptC target. Host-native clang is not a supported
Windows lane: it failed in ScriptC runtime compilation on missing `ssize_t` and
`clock_gettime`; TSPack does not patch around that compiler-owned problem.

Observed runs (developer workstation, not a controlled benchmark):

| Measurement | Result |
|---|---:|
| parity | four of four checksums equal |
| Node/V8 integer kernel | 22.9–23.9 ms |
| ScriptC kernel | 1109–1125 ms |
| total sidecar call | 1192–1196 ms |
| process/JSON boundary | 67–87 ms |
| first managed mixed build | about 6.4 s after tool materialization |
| warm mixed build | about 1.5 s including `go run` startup and tsc |

This workload does **not** demonstrate a speedup. ScriptC was roughly 47x slower
inside the measured kernel, before boundary cost. M71a therefore proves the
selective compiler/project boundary, parity, isolation, and measurement path;
it does not claim that current ScriptC accelerates this kernel.

Granularity guidance remains clear: use one coarse batch, flat scalar/string/
byte-like data, and enough work to amortize roughly 70–90 ms of process/JSON
overhead on this machine. Tiny calls are unsuitable. Before adopting a hot path,
measure that exact workload; do not infer speedup from native output alone.

## Incremental, offline, and deterministic evidence

- adding an unrelated normal TypeScript source printed `Cached ...:hotpath`
  while rebuilding only the tsc target;
- a dynamic-only replacement failed with
  `TSPACK_COMPILER_STATIC_COVERAGE_REQUIRED` and preserved the executable's
  SHA-256;
- after `update` and `sync`, a cached hot-path build succeeded with HTTP(S)
  proxies pointed at an unreachable local endpoint;
- a repeated exact-version update reported `+0 -0` and left `ts-lock.toml`
  byte-identical;
- TypeScript `--listEmittedFiles` listed only `dist/app/*`, proving the rebased
  projection did not emit `src/hot/compute.ts`.

## Outcome and M71b

M71a is **Outcome B — meaningful progression**. Selective ScriptC targets work
through the real compiler, source ownership and project isolation are enforced,
tool/package authority stays with TSPack, parity passes, and the performance
boundary is measured. The current bridge is platform/toolchain-specific and the
tested workload is not accelerated.

Recommended M71b: use ScriptC's documented library profile and contract sidecar
with a host that can consume a C archive directly, or wait for a ScriptC-provided
Node/Wasm host-callable export contract. Preserve coarse calls and generated,
versioned boundary metadata. Do not build a general N-API subsystem or consume
private `scr_*` runtime symbols merely to make Node load an archive.
