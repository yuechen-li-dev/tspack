# Perry 0.5.1220 repeated-convolution correctness repro

Environment: Windows 11 Pro 10.0.26200, x64, AMD Ryzen 7 7700X, Perry
0.5.1220, LLVM/clang 22.1.3.

The source creates deterministic RGBA-like `Uint8Array` input, applies a 5x5
integer convolution to the RGB channels, and invokes the kernel once for the
reference, five warmups, and eleven measured repetitions. Node remains stable:

```text
expectedChecksum = -374673474
final checksum    = -374673474
correct           = true
```

Perry's first result matches, but a later invocation changes the result:

```text
expectedChecksum = -374673474
final checksum    = 1253226460
correct           = false
```

Compile and run from this directory:

```powershell
$env:PERRY_RUNTIME_DIR = '<directory reported by perry doctor>'
perry compile repro.ts -o repro.exe --target windows --no-codegen --fp-contract off --no-cache
node repro.ts
.\repro.exe
```

Reduction observations:

- A single invocation and eight direct repeated invocations remain correct.
- The ordinary 5-warmup/11-sample envelope reproduces without any neighboring
  workload executing.
- Disabling Perry's typed-array fast path does not fix the full original case.
- `--no-auto-optimize` does not fix the full original case.
- `--verify-native-regions --explain-lowering` accepts the program but does not
  prevent the wrong result.
- `fastMath` is off and `fpContract` is off; the checksum kernel is integer.

This directory intentionally contains source and notes only. Generated `.exe`,
`.o`, cache, and trace files are not part of the report.
