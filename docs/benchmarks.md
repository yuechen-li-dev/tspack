# Native benchmarks (M19f)

Native benchmarks use `*.benchmark.tsx` files and run with `tspack bench`.

## Declarations
- `<Benchmark name="...">...</Benchmark>`
- `<Iterations count={1000} />` default `1000`
- `<Warmup count={100} />` default `100`
- `<CycleTime seconds={60} />` default `60`

Inside benchmark callback:
- `bench.check(fn)` optional preflight checks.
- `bench.measure(fn)` required exactly once.

Assertions are optional for benchmarks. `bench.measure` registration is the benchmark contract.

## Command
- `tspack bench --root .`
- `tspack bench --root . --list`
- `tspack bench --root . --filter parse`
- `tspack bench --root . --json`

`--profile` is not implemented in M19f.

## Caveats
- JS JIT and GC can affect results.
- Very tiny operations need future batching support.
- Timeout detection cannot preempt hard sync infinite loops without future worker/process isolation.

## Non-goals
- profiling
- baseline regression comparison
- death tests
- coverage
- watch
- Vitest benchmark backend
