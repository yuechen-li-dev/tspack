# M71b Perry performance qualification

M71b is **Outcome B — meaningful progression**. A project-managed Perry target
now works through the unchanged generic M71 `CompilerTarget`, and nine of ten
independent warm workloads have exact Node/Perry checksum parity. The requested
Copeland/RyuJIT comparison is not yet valid: current Copeland `tscl build` emits
JavaScript, while its C# backend has no project-shaped executable/RyuJIT contract
for the TSPack adapter. Perry also miscompiles the required 5×5 convolution on
this host. Those are concrete blockers, so this report does not manufacture
Copeland numbers or a convolution speed result.

The usable conclusion is already negative for selective throughput adoption:
Perry 0.5.1220 lost every valid warm-kernel row to warmed Node 26/V8. Perry did
win fresh-process startup, with a roughly 16–18 ms median versus Node's roughly
55–56 ms. That makes startup-sensitive native commands the only demonstrated
candidate class; it does not make Perry a useful accelerator for these hot
kernels.

## Baselines and environment

TSPack began clean on `main` at `119385d` and Copeland began clean on `main` at
`ea53d9049bbdc70d582a6a9b139c44c0c22d93ee`. Before edits, the broad TSPack Go
suite, manifest frontend build/typecheck/tests (209 passed, 2 skipped), and VS
Code compile/tests (35 passed) all succeeded.

| Item | Recorded value |
|---|---|
| OS / architecture | Windows 11 Pro 10.0.26200, x64 |
| CPU | AMD Ryzen 7 7700X, 8 cores / 16 logical processors |
| Power mode | Windows Balanced; no affinity pinning |
| Node | 26.2.0 |
| Bun | unavailable; omitted rather than substituted |
| Perry | npm `@perryts/perry` 0.5.1220 |
| Perry source audited | `PerryTS/perry` `f9890759c53f29449ac97320af615757a9111ff2` |
| LLVM used by Perry | clang 22.1.3 |
| Copeland | `ea53d9049bbdc70d582a6a9b139c44c0c22d93ee` |
| .NET | SDK 10.0.302, runtime 10.0.11, tiering defaults unchanged |
| Rust reference tool | rustc 1.95.0; not run because Perry did not produce a surprising win |

The npm version and audited `main` were current together on 2026-08-27, but
Perry's installed executable reports only its semver, not a source commit. The
source audit therefore records the repository commit separately rather than
claiming an unprovable binary-to-commit identity.

## Perry architecture audit

The native path is:

```text
TypeScript/JavaScript
  -> SWC-backed perry-parser
  -> structured perry-hir
  -> perry-transform HIR passes
  -> perry-codegen representation-aware lowering
  -> LLVM IR and LLVM optimization
  -> native object
  -> Perry runtime/stdlib static libraries
  -> executable, dylib, or staticlib
```

The CLI entrypoint is `crates/perry/src/main.rs`; the compile orchestration is
under `crates/perry/src/commands/compile`. Native codegen lives in
`crates/perry-codegen`, language HIR and analysis in `crates/perry-hir`, HIR
rewrites in `crates/perry-transform`, and the custom runtime/collector in
`crates/perry-runtime`. The web and Wasm emitters are separate crates and are
not the native execution path used here.

Perry's HIR is a rich structured semantic IR, not a general Perry-owned
CFG/SSA optimizer. LLVM IR is the general CFG/SSA layer. Perry nevertheless has
substantial language-specific mid-end work before LLVM:

| Optimization | Current evidence and boundary |
|---|---|
| Representation selection / unboxing | Present. `NativeRep` includes raw integer widths, `f64`, booleans, typed-array elements, buffer views, POD records, handles, and boxed `JsValue`/bits. Repsel has its own promotion census and verifier. |
| Escape/scalar replacement | Present but targeted. Aggregate scalar replacement, closure-local work, POD/native-record paths, and specialized loop escape analysis exist; this is not evidence of a universal object SROA pass. |
| Shape specialization | Present. Typed shapes, element-shape facts, property feedback, dense/packed array facts, and specialized field/index paths are explicit. |
| Loop specialization/versioning | Present for recognized structural families, with guard/fallback machinery and static-loop unrolling. |
| Bounds-check elimination | Targeted native buffer/typed-array paths carry `BoundsState`; generic/dynamic index helpers remain when proof fails. |
| Write-barrier elimination | Targeted pointer-free/native/scalar stores can avoid barriers; ordinary heap object writes retain correctness barriers. |
| Inlining | Perry HIR function/method inliner plus LLVM inlining. The HIR inliner rejects async, generator, capture, recursion, dynamic-`this`, and oversized cases. |
| Constant propagation | Local HIR folding, property CSE, static-loop unrolling, and LLVM constant propagation exist. No separate general Perry SSA SCCP was found. |
| Vectorization preparation | Specialized/native loop lowering can expose LLVM-friendly loops. It is workload-dependent, not guaranteed. |

The runtime is not “no runtime” in the implementation sense. Values may use
NaN-boxed `double` ABI slots, while proven regions use native reps. The GC is a
generational collector with an Eden nursery, copying survivor spaces, old
generation, write barriers/remembered sets, and full mark-sweep fallback.
Generated roots use target-aware LLVM statepoints where supported and a shadow
stack elsewhere; runtime Rust uses explicit handle scopes. The Windows npm
artifact links prebuilt `perry_runtime.lib` and stdlib components.

This distinction matters: LLVM can optimize the arithmetic and control flow it
receives, but it cannot remove language-runtime calls unless Perry proves the
corresponding representation, bounds, shape, and lifetime facts first.

## CompilerTarget integration and ownership

The generic target IR was unchanged. M71b adds only:

- compiler/language identities `perry` and `perry-ts`;
- a `PerryAdapter` with a versioned `perry-v1` payload;
- `perry.json`, which owns target, output type, fast-math, FP contraction,
  type-check, auto-optimization, codegen, and feature switches;
- bounded manifest validation and the CLI build dispatch;
- a real mixed fixture at `fixtures/perry-hotpath-m71b`.

TSPack still owns explicit input expansion, overlap/import rejection, package
bindings, tool selection, descriptors, graph order, staging, atomic artifact
replacement, and cache identity. The fixture proves `src/app/** -> tsc` and
`src/hot/** -> Perry`; the tsc projection excludes Perry-owned inputs, and the
app depends on the native artifact rather than importing hot source.

The cache fingerprint contains the exact Perry version, Perry config hash,
owned source hashes, visible package bindings, runtime/platform, output mode,
and declared outputs. It does not contain unrelated app files. A Perry source
change rebuilt the artifact; the immediate identical build was a cache hit.

Perry 0.5.1220's Windows npm package stores its runtime as a compressed library.
`perry doctor` materializes it, but `perry compile` did not automatically search
the reported cache directory in this installation. TSPack therefore invokes
the exact project-managed Perry binary's `doctor`, verifies the reported file,
and lets the adapter set Perry's documented `PERRY_RUNTIME_DIR` only for the
compile process. This uses no global `perry` from `PATH` and works offline once
the npm package is materialized.

## Bridge choice and granularity

Perry documents executable, `dylib`, and `staticlib` outputs. Libraries expose
`perry_module_init`; they do not expose a stable Node-callable kernel ABI.
Inventing N-API glue or binding private runtime symbols would turn this
qualification into a new FFI system. M71b therefore uses a coarse native
executable sidecar, the least artificial stable bridge for an existing Node
application.

The boundary probe launches a fresh process, parses a JSON argv payload,
normalizes it, checksums it, and returns JSON. It therefore measures cold start
plus serialization rather than pretending to be an in-process call:

| Payload request | Node median | Perry median |
|---:|---:|---:|
| 0 B | 56.11 ms | 18.28 ms |
| 256 B | 55.64 ms | 15.90 ms |
| 4 KiB | 55.12 ms | 15.81 ms |
| 16 KiB | 55.41 ms | 16.65 ms |

Within this range startup dominates payload scaling. A coarse Perry sidecar
should batch at least about 180 ms of useful Perry work to keep an 18 ms launch
under 10% of total time. Because Perry lost the valid kernels, this is a
boundary rule, not an adoption recommendation.

## Independent benchmark methodology

`fixtures/perry-hotpath-m71b/src/hot/bench.ts` is independent of Perry's
published suite. It covers the required ten families: the M71a integer loop,
floating point, 5×5 RGBA convolution, primitive typed-array reads/writes,
records/property access, allocation churn, byte hashing, JSON roundtrip,
branch-heavy code, and function-call abstraction.

Each workload runs in its own process to prevent one allocation-heavy kernel
from contaminating another. Timing is still steady-state and in-process: one
untimed correctness execution, five warmups, then eleven measured executions.
Node therefore reaches Maglev/TurboFan before its samples; `--trace-opt`
confirmed TurboFan OSR for the integer loop. Perry is AOT but receives the same
stabilization executions. Median is primary; min, p95, max, standard deviation,
and all raw samples are in `artifacts/m71b/perf-results.json`.

Integer/byte checksums require exact equality. A row keeps zero performance
samples if it is internally unstable or differs from Node. This rule excluded
the convolution rather than reporting a fast wrong answer.

## Results

| Workload | Node median | Perry median | Perry / Node | Classification |
|---|---:|---:|---:|---|
| M71a integer loop | 4.85 ms | 81.57 ms | 16.83× slower | severe loss |
| floating point | 16.55 ms | 44.14 ms | 2.67× slower | severe loss |
| image convolution 5×5 | 24.30 ms | invalid | — | correctness failure |
| primitive array R/W | 10.84 ms | 178.40 ms | 16.46× slower | severe loss |
| array of records | 2.59 ms | 1066.92 ms | 412.67× slower | severe loss |
| allocation churn | 12.29 ms | 1253.71 ms | 102.05× slower | severe loss |
| byte hash | 6.31 ms | 171.69 ms | 27.21× slower | severe loss |
| JSON roundtrip | 25.52 ms | 152.17 ms | 5.96× slower | severe loss |
| branch-heavy | 35.50 ms | 102.84 ms | 2.90× slower | severe loss |
| function calls | 7.30 ms | 54.15 ms | 7.42× slower | severe loss |

Thresholds were: strong win at least 1.5× faster, parity within ±10%, and
severe loss at least 2× slower. Perry has no win, parity, or merely modest loss
in the valid independent rows. The Perry executable is 11,903,488 bytes. Node
uses the separately installed runtime, so that size is not directly comparable.

The exact or semantically continuous M71a integer class remains a severe Perry
loss: 81.57 ms versus warmed Node's 4.85 ms. ScriptC's historical M71a result
was about 47× slower than Node; Perry improves materially over ScriptC but is
still 16.83× slower here. Perry does not solve that optimization class on this
source shape.

RSS and GC counts were not retained: Windows' external peak-working-set sample
would cover the entire process, while Perry exposed no stable program-level GC
telemetry contract shared with Node. Wall-time allocation results are valid,
but memory/collector attribution remains a deviation rather than guessed data.

## Generated-code explanation

Perry's `--trace hir,llvm --focus convolution --no-cache` showed the semantic
structure survived into HIR, including all five nested loops. The emitted LLVM
function nevertheless contained 52 calls and no vector-body/vector-type
markers. Hot accesses retained `js_typed_array_index_get_dynamic`,
`js_typed_array_get`, `js_typed_array_index_set_dynamic`, `js_uint8array_get`,
`js_uint8array_set`, length helpers, allocation helpers, and shadow-root calls.
There was no evidence of LLVM vectorization. The convolution checksum was
`-374673474` on Node and `1253226460` on Perry, so its timing is invalid; the
dynamic helper path is also the leading correctness suspect.

The same IR audit counted runtime calls in representative functions: integer
loop 5, floating point 8, records 60, allocation churn 60, byte hash 15,
branch-heavy 8, and function-call workload 6. The integer loop's calls are
mostly shadow-root frame/slot maintenance outside the arithmetic loop, whereas
the byte/record/object paths retain helpers in hot operations. No inspected
function contained an LLVM vector loop marker.

These deltas are consequently not “LLVM is slow.” Perry's HIR and codegen did
not expose helper-free, vectorizable, representation-stable loops for these
shapes. LLVM optimized the IR it received. The 400× record and 100× allocation
losses align with Perry's own current engine plan, which identifies object
construction, typed-shape layout work, write barriers, and feedback bookkeeping
as dominant costs. V8 instead specialized the live shapes and hot loops at run
time. Perry's language mid-end/runtime is therefore the controlling factor in
both wins and losses; LLVM is the final optimizer, not the source of semantic
facts.

## Copeland/RyuJIT and specialization blocker

Copeland is present and healthy enough to audit, but its current TSPack M71
adapter deliberately advertises JavaScript emission. `tscl build` produces a
Node/browser JavaScript project. The repository has a C# backend and MSBuild SDK
integration, but no equivalent project descriptor that emits and executes a
standalone RyuJIT benchmark from TSPack's target graph. Building that contract
would be a separate Copeland compiler-target milestone, not a benchmark fixture.

For the same reason, no honest ordinary/static-specialized Copeland convolution
number is reported. Copeland's implemented `static` surface is bounded template
and artifact construction (`static if`, `static for`, `static match`), not a
general runtime-function partial evaluator. It could generate invariant source
structure, but doing so before a real CLR execution contract exists would test
a bespoke generator rather than the requested ordinary-versus-specialized
Copeland compilation strategy.

## Adoption recommendation and deviations

Do not adopt Perry 0.5.1220 as a selective throughput compiler for these
numeric, byte, typed-array, record, allocation, JSON, branch, or abstraction
hot paths on Windows x64. If a project values a roughly 16–18 ms native cold
start and can accept an 11.4 MB artifact, qualify a complete coarse command;
do not make fine-grained process calls. Keep targets bounded until checksum
parity passes, especially for typed-array convolution.

The generic M71 boundary survived unchanged. Incremental/cache behavior and
project-managed acquisition work. The executable sidecar is explicit and
offline after npm/runtime materialization. Perry diagnostics are passed through;
TSPack only owns missing tool/config/runtime/output, invalid target, and
ownership diagnostics.

Deviations are Bun unavailable; Copeland/RyuJIT and static-specialized runs
blocked by the missing CLR project execution contract; convolution rejected for
wrong output; RSS/GC telemetry omitted; and upstream published benchmarks not
rerun after the independent suite because the required cross-runtime comparison
was already blocked and the independent primary workload failed correctness.
No Perry or Copeland compiler was patched.

Recommended M71c: add a Copeland `nativeDotnetExecutable` project contract that
compiles ordinary Copeland TS through the existing C# backend/MSBuild path,
defines a warm long-lived benchmark protocol, and only then add the ordinary
and bounded-static convolution variants. Separately reduce the Perry
convolution to an upstream-quality typed-array miscompile report before any
further performance claim.

## Validation

The final TSPack tree passed:

- `go test ./cmd/... ./internal/... ./tools/... -count=1 -timeout 420s`;
- manifest frontend build and manifest-API typecheck;
- manifest frontend tests: 209 passed, 2 environment-dependent browser tests skipped;
- VS Code compile and tests: 35 passed;
- `tspack compat diff --root .` after writing the changed canonical declaration;
- `tspack audit --root .`: 117 locked npm packages checked, no known vulnerabilities;
- mixed-fixture Perry build, cache hit, tsc app build, and real sidecar execution;
- `git diff --check`.

Copeland was not modified, but its requested broad validation was run anyway:
`dotnet test Copeland.slnx --configuration Debug --no-restore` passed all
reported projects, including 1,043 compiler tests, 244 C# backend tests, 181
JavaScript backend tests, 65 CLI tests, and 11 MSBuild tests. Copeland remained
clean and `git diff --check` passed.
