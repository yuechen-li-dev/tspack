# M71c Copeland backend targets

M71c removes the project-facing assumption that `tscl` means JavaScript. The
generic compiler target still carries language, compiler, runtime, outputs,
capabilities, tool identity, config reference, inputs/packages, and an opaque
payload. No Copeland lowering option was added to generic IR.

The sibling Copeland compiler owns named `tscl.targets` in `tsconfig.tsx`.
TSPack selects the same name in `manifest.tsx`, projects the generic artifact
needed by the graph, invokes the exact declared `tscl`, transports compiler
diagnostics, and verifies the materialized entry. Build output exposes:

```text
language=copeland-ts compiler=tscl@<version>
backend=<javascript|csharp> runtime=<node|browser|ryujit|nativeaot|wasm>
artifact=<javaScript|managedExecutable|nativeExecutable|wasmModule>
```

`fixtures/copeland-targets-m71c` is the same-source proof. On the M71c Windows
x64 host all four builds succeeded. JS/Node, CLR/RyuJIT, and NativeAOT ran and
printed `Copeland M71c parity`. The installed .NET 10 WASM workload produced a
real runtime bundle containing `dotnet.native.wasm`, `dotnet.js`, the managed
entry assembly, runtime config, and dependencies. Full browser automation is
outside this bounded compiler/artifact proof.

The managed artifact is launched with `dotnet <assembly>` and carries its
runtime config/deps files. NativeAOT is directly executable and its cache key
contains the explicit RID. WASM is a generic module plus web/runtime bundle;
TSPack does not call the artifact “Blazor” because this implementation is a
plain .NET `browser-wasm` publish rather than a Blazor application model.

The .NET SDK and WASM workload are system prerequisites in M71c. TSPack does
not acquire or change them. Missing workload/tool failures retain the original
`dotnet publish` evidence and an actionable Copeland target diagnostic.

Backend selection is workload-dependent. JavaScript/Node remains the default,
and neither RyuJIT nor NativeAOT is a fast mode. `CtsJitM0` already implements
one-process warmup and repeated internal kernel timing, so M71b can add the new
managed artifact as its Copeland/RyuJIT lane. The Perry 5x5 convolution
miscompile was not reduced in M71c; it remains an independent upstream-repro
task.
