# Performance Harness

TSPack now has a developer-facing performance harness for `update` and `sync`.

The goal is repeatable local measurement for:

- cold `update`
- first `sync`
- warm `sync`
- `update --dry-run`
- phase timings and request/capture/materialization counters
- optional Go CPU and heap profiles

This is a developer workflow. Normal commands stay quiet unless perf env vars are enabled.

## Quick start

From the repository root on Windows:

```powershell
.\tools\Bench-TSPack.ps1 -Template react -Runs 3 -Profile
```

That script:

- builds `tspack` once by default
- creates fresh temp projects from `tspack init`
- uses explicit per-run `--store` paths under `dist/bench/...`
- captures per-scenario perf JSON
- optionally captures CPU and heap profiles
- writes logs and a `summary.json`

Use `-UseGoRun` if you want to execute through `go run` instead of building a binary.

## Resolver concurrency

Cold `update` now uses deterministic parallel resolver preparation:

- workers fetch package facts concurrently
- commit stays serial and deterministic
- lockfile bytes should stay identical between `TSPACK_RESOLVE_JOBS=1` and the default

The default resolver worker count is `24`. Override it with:

```powershell
$env:TSPACK_RESOLVE_JOBS = "1"
.\tools\Bench-TSPack.ps1 -Template react -Runs 1

$env:TSPACK_RESOLVE_JOBS = "24"
.\tools\Bench-TSPack.ps1 -Template react -Runs 1

Remove-Item Env:\TSPACK_RESOLVE_JOBS
```

Store population remains separately controlled by `TSPACK_STORE_JOBS`.

## Scenarios

The default `suite` expands to:

- `cold-update`
- `first-sync`
- `warm-sync`
- `dry-run-update`

Examples:

```powershell
.\tools\Bench-TSPack.ps1 -Scenario cold-update -Runs 2
.\tools\Bench-TSPack.ps1 -Scenario warm-sync -Runs 5 -StoreJobs 24
```

## Perf JSON

Perf tracing is env-driven and off by default.

Supported env vars:

```text
TSPACK_TRACE_PERF=1
TSPACK_TRACE_PERF_JSON=dist/bench/perf.json
TSPACK_CPU_PROFILE=dist/profiles/update.cpu.pprof
TSPACK_MEM_PROFILE=dist/profiles/update.mem.pprof
```

`TSPACK_TRACE_PERF=1` prints a concise stderr summary after the command.

`TSPACK_TRACE_PERF_JSON` writes a structured report without affecting stdout JSON payloads.

The perf report includes:

- total command time
- named phase timings
- resolve job count
- resolve frontier count
- max resolve frontier width
- prepared package count
- committed package count
- resolver worker error count
- metadata request count
- metadata memoization cache hit count
- tarball request count
- artifacts captured during resolve
- artifacts already in store during capture
- artifacts needing store population
- store population skip/fetch counts
- sync hydration skip/fetch counts
- materialization marker hit/miss/mismatch/corrupt counts
- materialization noop / forced-rematerialization flags
- skipped package/file/directory counts on noop sync
- materialization marker write count
- materialized package/file/directory counts
- hardlink vs copy-fallback counts
- logical materialized bytes
- bytes actually copied
- optional HTTP request-kind, host, and status-code counts

## pprof

You can collect profiles directly without the bench script:

```powershell
$env:TSPACK_TRACE_PERF_JSON = "dist/perf/update.json"
$env:TSPACK_CPU_PROFILE = "dist/profiles/update.cpu.pprof"
$env:TSPACK_MEM_PROFILE = "dist/profiles/update.mem.pprof"
go run ./cmd/tspack update --root . --store .tspack/store
```

Inspect profiles with:

```powershell
go tool pprof dist/profiles/update.cpu.pprof
go tool pprof -http=:0 dist/profiles/update.cpu.pprof
```

The harness does not expose an HTTP pprof server.

## Store safety

The benchmark harness does not delete the user’s real store.

Instead, it uses explicit isolated store paths via the existing `--store` flag:

```text
--store dist/bench/<timestamp>/run-01/react-store
```

That keeps cold-run testing safe and reversible.

## Disk usage

The current script reports `node_modules` apparent file bytes per scenario.

Hardlink effectiveness is primarily tracked through the materialization counters in perf JSON:

- `hardlinkCount`
- `copyFallbackCount`
- `logicalBytesMaterialized`
- `bytesCopied`
- `materializationNoop`
- `materializationSkippedFiles`

Cross-platform deduped physical-size accounting is still intentionally conservative.

## TypeScript-side microbenchmarks

TSPack’s native benchmark support is still useful for TypeScript/runtime microbenchmarks:

```powershell
tspack bench --root .
tspack bench --root . --json
```

That path is complementary, not a replacement for this harness:

- `tspack bench` measures JS-side benchmark callbacks
- it does not break down Go `update`/`sync` phases
- it does not currently provide pprof capture

Use it when you want to dogfood benchmark ergonomics for `.benchmark.tsx` files or isolate TypeScript-side work.
