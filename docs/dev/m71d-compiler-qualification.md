# M71d compiler qualification lab

## Outcome

M71d is **Outcome B — meaningful progression**. All four core execution
strategies now have valid warm-kernel and cold-start measurements on this host:
Node/V8 and Perry across the ten-workload independent suite, and Copeland/V8,
Copeland/RyuJIT, and Copeland/NativeAOT across five same-post-static-MIR
workloads. The exact generated C# host used for RyuJIT is the input to the
NativeAOT publish, so that comparison isolates runtime strategy rather than
source or algorithm.

The remaining blockers are semantic, not missing stopwatch code. Copeland does
not currently expose general mutable indexed arrays or runtime JSON
parse/stringify, and its `static` facility specializes authoring templates and
compile-time data rather than runtime functions. Filling those cells with host
C# would violate the same-source requirement. Peak RSS is also not in the
primary artifact because polling these short-lived processes perturbs the very
startup and kernel measurements being studied. Those deviations prevent an
Outcome A claim.

The headline result is not “compiler X won.” It is:

- V8 turns dynamic TypeScript feedback into excellent hot loops, but pays the
  largest process startup cost here.
- RyuJIT is strongest on Copeland record and string workloads after tiered OSR
  reaches optimized Tier 1; it is not universally faster than V8.
- NativeAOT wins cold start and deployment size in this matrix, while trailing
  RyuJIT on four of five warm Copeland workloads.
- Perry wins neither independent warm throughput nor this host's reproduced
  published `array_read` cell. Its startup remains much better than Node's, but
  NativeAOT is faster still on the comparable tiny managed host.
- Perry 0.5.1220 still produces a wrong repeated-convolution result. That row is
  excluded from performance comparison.

## Baselines and host

Both repositories began clean on `main`. All prescribed broad validation
passed before benchmark code changed.

| Item | Value |
|---|---|
| TSPack | `5961e45`, `tspack v0.1.8` |
| Copeland | `1f5e3c1` |
| OS | Windows 11 Pro 10.0.26200, x64 |
| CPU | AMD Ryzen 7 7700X, 8 cores / 16 logical processors |
| RAM | 33,412,775,936 bytes |
| Power mode | Windows Balanced; no affinity pinning |
| Node / V8 | Node 26.2.0 / V8 14.6.202.34-node.20 |
| Bun | Not installed; omitted |
| .NET | SDK 10.0.302; runtime 10.0.11; win-x64 |
| Perry | npm and GitHub release 0.5.1220 |
| LLVM | clang 22.1.3 |

The current Perry release was checked independently through npm's `latest`
dist-tag and GitHub's latest release endpoint. Both resolve to 0.5.1220. The
[npm package](https://www.npmjs.com/package/%40perryts/perry) and
[GitHub release](https://github.com/PerryTS/perry/releases/tag/v0.5.1220) are
the version under test; no Perry patch or cherry-pick was used.

## Benchmark semantics and methodology

The independent suite retains the M71b compiler-neutral TypeScript source and
its ten required families:

| Workload | Input and output contract | Allocation shape |
|---|---|---|
| M71a integer loop | 12,000,000 deterministic integer updates; exact int32 checksum | none in loop |
| Floating point | 2,000,000 `sin`/`sqrt` updates; floored checksum | none in loop |
| 5x5 convolution | deterministic 512x512x4 byte image, fixed integer kernel; exact checksum | two byte arrays + coefficient array per run |
| Primitive array R/W | 1,000,000 int32 values, 12 write/read passes; exact checksum | one typed array per run |
| Array of records | 200,000 `{x,y,z}` records, eight read passes; exact checksum | record array per run |
| Allocation churn | 20 generations of 80,000 records; exact checksum | intentionally heavy |
| Byte hash | deterministic 4,000,000-byte FNV-like loop; exact int32 hash | one byte array per run |
| JSON roundtrip | 12,000 records, eight parse/stringify passes; exact total text length | parser/serializer owned |
| Branch-heavy | 16,000,000 LCG-driven branches; exact int32 checksum | none in loop |
| Function calls | 8,000,000 `mix` calls; exact int32 checksum | none in loop |

Each selected independent workload runs in one long-lived process with five
warmups and eleven internally timed samples. Eleven samples satisfy the
expensive-workload floor; this study does not inflate runtime merely to reach a
round number. The Copeland suite uses ten warmups and twenty internally timed
samples. Median is primary; the artifact also stores min, p05, p95, max, MAD,
standard deviation, and every raw sample.

Cold startup is a separate ten-fresh-process lane. Compile/publish time and
artifact sizes are separate again. Normal production flags are used: ordinary
Node, Copeland production JS, Release framework-dependent .NET with default
tiering/dynamic PGO, Release NativeAOT, and Perry with fast math off and
`fpContract=off`.

TSPack's `*.benchmark.tsx` facility was evaluated and deliberately not used for
the cross-runtime clock. It measures a single in-process JavaScript callback,
and `command` is unavailable to `Benchmark` units. Timing process launches in a
TSPack callback would conflate startup and kernel time. M71d therefore keeps
compiler-specific orchestration under `benchmarks/m71d` and requires every
runtime to time its own warmed kernel.

## Correctness gate

All valid rows match their lane reference. Copeland JS, RyuJIT, and NativeAOT
match for all five common workloads. Perry matches Node on nine of ten
independent workloads. The convolution row is retained as a correctness failure
with zero accepted samples and no summary statistics.

| Workload | Node checksum | Perry checksum | Status |
|---|---:|---:|---|
| M71a integer | -898,670,720 | -898,670,720 | pass |
| Floating point | 13,258,651,682 | 13,258,651,682 | pass |
| 5x5 convolution | -374,673,474 | 1,253,226,460 | **fail; performance excluded** |
| Primitive array | 630 | 630 | pass |
| Records | -1,063,537,152 | -1,063,537,152 | pass |
| Allocation churn | 659,414,260 | 659,414,260 | pass |
| Byte hash | -1,668,388,411 | -1,668,388,411 | pass |
| JSON | 5,985,800 | 5,985,800 | pass |
| Branch-heavy | 1,522,022,784 | 1,522,022,784 | pass |
| Function calls | 511,506,573 | 511,506,573 | pass |

## Warm independent suite

| Workload | Node/V8 median | Perry median | Perry / Node | Classification |
|---|---:|---:|---:|---|
| M71a integer | 4.85 ms | 78.88 ms | 16.26x | severe loss |
| Floating point | 16.52 ms | 46.62 ms | 2.82x | severe loss |
| 5x5 convolution | 24.48 ms | excluded | — | correctness fail |
| Primitive array R/W | 10.79 ms | 161.46 ms | 14.96x | severe loss |
| Array of records | 3.35 ms | 1,101.96 ms | 328.9x | severe loss |
| Allocation churn | 12.82 ms | 1,356.56 ms | 105.8x | severe loss |
| Byte hash | 6.25 ms | 163.60 ms | 26.18x | severe loss |
| JSON roundtrip | 25.87 ms | 148.93 ms | 5.76x | severe loss |
| Branch-heavy | 35.42 ms | 102.15 ms | 2.88x | severe loss |
| Function calls | 7.31 ms | 51.44 ms | 7.04x | severe loss |

The classification bands are fixed: strong win at least 1.5x faster, rough
parity within 10%, moderate loss 1.1–2x, and severe loss at least 2x.

## Same Copeland semantics: V8, RyuJIT, NativeAOT

| Workload | V8 | RyuJIT | NativeAOT | Winner | Why |
|---|---:|---:|---:|---|---|
| Numeric kernel | 34.29 ms | 31.83 ms | 33.48 ms | rough parity | scalar modulo/branch loop; optimized Tier 1 and AOT generate similar work |
| Machina layout subset | 3.51 ms | 2.31 ms | 4.39 ms | RyuJIT | stable record shapes and hot calls; runtime feedback beats AOT here |
| Typed reducer batch | 8.05 ms | 8.72 ms | 12.21 ms | V8 / RyuJIT parity | both JITs specialize the stable state/event flow; AOT retains more fixed overhead |
| Record-array transform | 11.45 ms | 5.40 ms | 7.80 ms | RyuJIT | direct typed record fields and tiered optimization; 2.12x over V8 |
| String processing | 45.24 ms | 10.27 ms | 13.91 ms | RyuJIT | generated C# uses typed invariant integer formatting and CLR strings; 4.40x over V8 |

RyuJIT reached `Tier1-OSR with Synthesized PGO` for the numeric `Run` method in
a separate diagnostic run. The optimized body was 131 bytes versus 223 bytes
for instrumented Tier 0. Production defaults were retained: tiered compilation
and dynamic PGO enabled, no ReadyToRun publish, profiler, debugger, or COMPlus
timing overrides.

NativeAOT is close to RyuJIT on the scalar numeric loop but loses 1.35–1.90x on
the other four rows. That is evidence for runtime specialization, not a general
law that JIT always beats AOT. NativeAOT's role is clearer in startup and
deployment.

## Cold start

| Lane | Median | p95 |
|---|---:|---:|
| NativeAOT | 11.43 ms | 37.98 ms |
| Perry | 19.02 ms | 22.07 ms |
| RyuJIT managed app | 28.40 ms | 29.55 ms |
| Node | 54.25 ms | 70.35 ms |
| Bun | unavailable | — |

NativeAOT is the cold-start winner. Perry remains about 2.85x faster to launch
than Node, which is a real advantage for a short-lived native command. It does
not imply hot-loop acceleration. The NativeAOT p95 includes one host-noise tail;
the median remains primary.

## Compile/build time and artifacts

| Operation | Observed time |
|---|---:|
| Copeland five-workload emit/build/run orchestration | 17.35 s |
| Perry clean compile | 0.94 s |
| Perry incremental compile | 0.79 s |
| NativeAOT publish per tiny workload | 3.50–3.83 s |

The Copeland total is not a single compiler-only phase; it includes five MIR
compilations, two backend emissions, generated-host builds, correctness probes,
cold launches, warmups, and measurements. It must not be compared directly to
Perry's single-source compile time.

| Artifact | Size |
|---|---:|
| Perry executable | 11,906,048 bytes |
| NativeAOT executable / publish footprint | 976,896–978,432 bytes |
| Copeland managed main assembly | 9,728–12,800 bytes |
| Copeland generated JS host | 1,960–10,533 bytes |

Managed and JS sizes assume an installed runtime. NativeAOT and Perry are
self-contained native deployment artifacts, so their footprints are the more
direct comparison. NativeAOT is about 12.2x smaller here.

## Allocation and GC observations

The managed and NativeAOT hosts report per-thread allocated bytes and collection
counts across the twenty measured rounds. Their totals are nearly identical,
showing that switching JIT/AOT did not change Copeland's allocation semantics:

| Copeland workload | Allocated bytes | Gen0 / Gen1 / Gen2 |
|---|---:|---:|
| Numeric | ~2.3 KiB | 0 / 0 / 0 |
| Machina | ~312.3 MB | 19 / 0 / 0 |
| Reducer | ~2.03 GB | 122 / 0 / 0 |
| Record transform | ~960.0 MB | 58 / 0 / 0 |
| String processing | ~1.52 GB | 91 / 0 / 0 |

This is direct evidence that neither RyuJIT nor NativeAOT scalar-replaces most
of these emitted immutable records/strings. V8 heap deltas are included only as
coarse before/after signals; negative deltas are possible when GC runs and are
not allocated-byte measurements. Perry's LLVM for object workloads retains
array pushes, object allocation, property feedback/guard/fallback calls, and
write barriers, which explains its much larger losses.

## Generated-code observations

### V8

Separate `--trace-opt --trace-deopt` runs confirm Maglev and TurboFan OSR for
the M71a integer loop. Convolution also reaches TurboFan, but its nested loop
exits and typed-array construction/access cause repeated deoptimizations for
insufficient construct, binary-operation, and named-access feedback. Diagnostic
flags were not used in primary timings.

### RyuJIT and NativeAOT

RyuJIT's diagnostic summary proves optimized Tier 1 OSR rather than Tier 0 was
the steady-state numeric path. The generated C# is ordinary typed code: `int`
loops and fields, `double` for Copeland number/float, arrays with CLR indexing,
and invariant `ToString` in the string workload. NativeAOT was published from
that exact host with `RID=win-x64`, `PublishAot=true`, `SelfContained=true`,
`InvariantGlobalization=true`, `DebugType=None`, and stripped symbols.

The stripped NativeAOT image contains ordinary scalar/SSE instructions in the
diagnostic disassembly, but no symbol-preserving evidence sufficient to
attribute a vector loop to the Copeland kernel. This report therefore does not
claim NativeAOT vectorization.

### Perry LLVM and helper density

The retained 424 KiB LLVM module contains no `vector.body`, vectorized-loop
metadata, or workload-attributable vector IR. Major calls remaining in hot
functions include:

| Function | Important retained helpers |
|---|---|
| Integer loop | shadow-frame/slot bookkeeping only; otherwise scalar arithmetic |
| Convolution | typed-array get/set and dynamic-index helpers, byte-array allocators, three dynamic length calls, 36 shadow-slot stores |
| Primitive array | typed-array get/set plus allocation; one `llvm.assume` |
| Records | array push/get, six field loads, property-feedback guard/pass/fail/fallback machinery, barriers |
| Allocation churn | object allocation, array push/get, six field loads, property feedback, barriers |
| JSON | full JSON parse/stringify helpers, object/array allocation and pushes, barriers |

The compiler's own lowering report recorded 127 boxes, 40 unbox/coercions, 21
dynamic fallbacks, 14 emitted write barriers, only two bounds eliminations, zero
scalar replacements, and zero selected typed paths for the full module. That is
the “why” behind the 5.8x–329x losses: LLVM receives language-runtime calls and
dynamic boundaries that it cannot legally optimize away.

## Every >=2x delta

- Perry integer, branch, and call losses retain boxed function ABI/shadow-frame
  boundaries and do not recover V8's feedback-directed OSR specialization.
- Perry float is 2.82x slower because the default comparison preserves strict
  FP semantics; no fast-math reassociation is allowed, and math helpers remain.
- Typed-array/byte losses retain typed-array helpers and bounds/dynamic-index
  paths; the emitted module shows no useful vector loop.
- Record/allocation losses are dominated by heap allocation, array pushes,
  field guards/fallbacks, barriers, and no recorded scalar replacement.
- JSON is library/runtime dominated: Perry calls its full JSON parser and
  stringifier and retains object/array allocation services.
- RyuJIT's 2.12x record and 4.40x string wins use typed CLR representations,
  direct fields, tiered PGO, and CLR library implementations. Those wins do not
  imply CLR should become Copeland's default.
- NativeAOT's 3.25x string win over Copeland/V8 comes from the same typed C# and
  CLR-native string formatting semantics; it still trails RyuJIT by 1.35x.

## M71a continuity

| Lane | Median / historical estimate | Comparable status |
|---|---:|---|
| Node/V8 | 4.85 ms | exact independent source |
| ScriptC historical | about 228 ms (about 47x Node) | M71a historical |
| Perry | 78.88 ms | exact independent source; 16.26x Node |
| Copeland/V8 | unavailable | current Copeland numeric kernel is not the M71a algorithm |
| Copeland/RyuJIT | unavailable | same reason |
| Copeland/NativeAOT | unavailable | same reason |

The Copeland numeric row is intentionally not relabeled as M71a. It is a
different modulo/branch kernel with an iteration parameter; manufacturing a
continuity table from it would be algorithm drift.

## Convolution failure and upstream repro

`artifacts/m71d/perry-convolution-repro/repro.ts` is a deterministic standalone
reduction. Node returns `-374673474` for every invocation. Perry's first result
also matches, but after five warmups and eleven measured repetitions the final
checksum is `1253226460`. The repro contains only the convolution and its
measurement envelope.

The failure persists with Perry's typed-array fast path disabled, with
`--no-auto-optimize`, and with native-region verification/lowering explanation.
It is not fast-math related. Single and shorter repeated runs can remain
correct, so invocation count/allocation lifetime is part of the reduced trigger.
No Perry issue was opened.

## Perry published benchmark cross-check

The exact v0.5.1220 `array_read` and `loop_overhead` sources were copied under
`artifacts/m71d/perry-published-crosscheck` and run eleven times per runtime,
using each program's internal `Date.now()` measurement and exact checksum.

| Published cell | Published Apple M1 result | This Windows x64 Node | This Windows x64 Perry | Finding |
|---|---:|---:|---:|---|
| `array_read` claimed win | Perry 11 ms, Node 14 ms | 7 ms | 79 ms | checksum passes, claimed win does not reproduce |
| `loop_overhead` published loss | Perry 97 ms, Node 53 ms | 31 ms | 55 ms | direction reproduces |

The upstream report is from Perry 0.5.908 on Apple M1 Max, while this run uses
0.5.1220 on Windows/x64. The reversal is therefore a version/platform result,
not proof that the published measurement was false. It does prove the published
win cannot be generalized to current Perry on this host. The upstream
[methodology and result table](https://raw.githubusercontent.com/PerryTS/perry/v0.5.1220/benchmarks/polyglot/RESULTS.md)
is the comparison source.

## Static specialization

No static-specialized performance row is reported. Copeland's implemented
`static` surface specializes template/authoring evaluation and compile-time
data; it is not a runtime function-specialization annotation. A separate table
or generated-host algorithm would change the runtime representation and would
not be the ordinary convolution source. Precomputing output is forbidden.

This is a negative language-surface result, not evidence that compile-time
specialization is unhelpful. A future fair convolution row requires a Copeland
runtime array law plus a supported way for static coefficient/offset data to
survive into the same post-static MIR consumed by JS and C#.

## Recommendations

- **Perry:** do not adopt as a hot-loop accelerator on this evidence. It remains
  potentially useful for short-lived one-shot native commands once correctness
  is established for the workload. The convolution failure blocks that class
  of image kernel today.
- **NativeAOT:** adopt as an explicit deployment/cold-start option. It produces
  a roughly 0.98 MB self-contained artifact and the best cold median, but does
  not replace RyuJIT for maximum steady-state throughput.
- **RyuJIT:** retain as a workload-dependent backend. It wins the record and
  string rows strongly, but this does not justify making CLR the default.
- **JavaScript/V8 default:** evidence still supports V8 as Copeland's default.
  It is competitive with RyuJIT on two rows, wins the reducer row narrowly,
  preserves the broadest language/runtime surface, and avoids AOT publish cost.
  Backend choice remains workload-dependent as established in M71c.
- **TSPack:** keep generic target materialization/provenance in TSPack and lab
  logic under `benchmarks/`, `artifacts/`, and `docs/dev/`. Do not expand the
  production compiler-target IR for benchmark-specific telemetry.

## Artifacts and deviations

- Machine-readable samples: `artifacts/m71d/perf-results.json`
- Reproducible orchestrator: `benchmarks/m71d/run.mjs`
- Perry repro: `artifacts/m71d/perry-convolution-repro/`
- Published cross-check sources: `artifacts/m71d/perry-published-crosscheck/`

Deviations from the requested Outcome A matrix:

1. Bun is not installed.
2. Copeland has no honest mutable typed-array or runtime JSON implementation,
   so the ten-family Copeland matrix is incomplete.
3. No runtime `static` specialization surface exists for the requested ordinary
   versus static benchmark pair.
4. Peak RSS was not measured without a low-perturbation cross-runtime monitor.
5. NativeAOT symbols are stripped in the production artifact, limiting
   function-attributable disassembly.
6. The exact M71a source is not currently accepted by Copeland, so Copeland
   numbers are excluded from the continuity row.

Recommended M72: add a benchmark-neutral process telemetry contract that can
capture child peak RSS without polling, then define Copeland runtime array
mutation and static-data lowering laws before extending this matrix. Do not add
benchmark-only compiler intrinsics.

## Validation

Post-change validation passed:

- `go test ./cmd/... ./internal/... ./tools/... -count=1 -timeout 420s`
- manifest frontend build, manifest API typecheck, and tests: 209 passed,
  2 skipped
- VS Code extension compile and tests: 35 passed
- `tspack compat diff` and `tspack audit`: up to date, 117 npm packages
  checked, no known vulnerabilities
- `dotnet test Copeland.slnx --configuration Debug --no-restore`: all projects
  passed, including JS backend, C# backend, static evaluation, CLI, MSBuild, and
  target configuration coverage
- the M71d runner's NativeAOT publish/checksum/warm paths passed for all five
  Copeland workloads
- `node --check benchmarks/m71d/run.mjs`
- machine-readable artifact content gate: schema 1, 35 workload rows, two
  published cross-check rows
- `git diff --check` in both repositories
