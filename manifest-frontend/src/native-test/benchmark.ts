import { performance } from 'node:perf_hooks';
import path from 'node:path';
import { discoverNativeTestFiles } from './discover.js';
import { loadRuntimeSuiteForFile } from './runtime-load.js';
import { isSkipSignal } from './skip.js';
import type { BenchmarkResult, BenchmarkRunResult, Diagnostic, DiscoveredBenchmark, RunBenchmarksOptions } from './types.js';

type RuntimeNode = { __tag: string; props: Record<string, unknown>; children: unknown[] };

type BenchContext = {
  checks: Array<() => unknown | Promise<unknown>>;
  measureFn?: () => unknown | Promise<unknown>;
};

export function listNativeBenchmarks(options: { rootDir: string }): { benchmarks: DiscoveredBenchmark[]; diagnostics: Diagnostic[] } {
  const discovered = discoverNativeTestFiles({ rootDir: options.rootDir });
  const benchmarks: DiscoveredBenchmark[] = [];
  const diagnostics: Diagnostic[] = [...discovered.diagnostics];
  for (const file of discovered.files) {
    benchmarks.push(...file.benchmarks);
  }
  benchmarks.sort((a, b) => a.id.localeCompare(b.id));
  return { benchmarks, diagnostics };
}

export async function runNativeBenchmarks(options: RunBenchmarksOptions): Promise<BenchmarkRunResult> {
  const listed = listNativeBenchmarks({ rootDir: options.rootDir });
  const diagnostics: Diagnostic[] = [...listed.diagnostics];
  const selected = listed.benchmarks.filter((entry) => !options.filter || entry.id.includes(options.filter) || entry.name.includes(options.filter));
  if (options.filter && selected.length === 0) {
    diagnostics.push({ code: 'TSPACK_BENCHMARK_FILTER_NO_MATCH', message: `benchmark filter matched no benchmarks: ${options.filter}`, file: path.resolve(options.rootDir), severity: 'error' });
  }
  if (options.listOnly) {
    return { benchmarks: selected.map((b) => ({ id: b.id, name: b.name, status: 'passed', iterations: b.iterations, warmup: b.warmup })), diagnostics };
  }

  const grouped = new Map<string, DiscoveredBenchmark[]>();
  for (const bench of selected) {
    const abs = path.resolve(options.rootDir, bench.filePath);
    if (!grouped.has(abs)) grouped.set(abs, []);
    grouped.get(abs)?.push(bench);
  }

  const results: BenchmarkResult[] = [];
  for (const [filePath, fileBenches] of grouped) {
    const root = (await loadRuntimeSuiteForFile(filePath)) as RuntimeNode;
    const suiteName = String(root.props.name ?? '');
    for (const item of fileBenches) {
      const node = root.children.find((c) => isNode(c) && c.__tag === 'Benchmark' && String(c.props.name ?? '') === item.name) as RuntimeNode | undefined;
      if (!node) continue;
      results.push(await runOneBenchmark(item, suiteName, node, options.defaultCycleTimeSeconds ?? item.cycleTimeSeconds ?? 60));
    }
  }

  return { benchmarks: results.sort((a, b) => a.id.localeCompare(b.id)), diagnostics };
}

async function runOneBenchmark(item: DiscoveredBenchmark, suiteName: string, node: RuntimeNode, defaultCycle: number): Promise<BenchmarkResult> {
  const context: BenchContext = { checks: [] };
  const benchApi = {
    check(fn: () => unknown | Promise<unknown>) { context.checks.push(fn); },
    measure(fn: () => unknown | Promise<unknown>) {
      if (context.measureFn) {
        const err = new Error('duplicate measure registration');
        (err as Error & { code?: string }).code = 'TSPACK_BENCHMARK_DUPLICATE_MEASURE';
        throw err;
      }
      context.measureFn = fn;
    },
  };
  const callback = node.children.find((c) => typeof c === 'function') as ((ctx: { bench: typeof benchApi }) => unknown) | undefined;
  try {
    await Promise.resolve(callback?.({ bench: benchApi }));
    if (!context.measureFn) {
      return { id: item.id, name: item.name, status: 'failed', iterations: item.iterations, warmup: item.warmup, failure: { code: 'TSPACK_BENCHMARK_MISSING_MEASURE', message: 'benchmark must register exactly one bench.measure(...)' } };
    }
    for (const check of context.checks) await Promise.resolve(check());
    for (let i = 0; i < item.warmup; i += 1) await Promise.resolve(context.measureFn());
    const cycle = Number(node.children.find((e) => isNode(e) && e.__tag === 'CycleTime') ? (node.children.find((e) => isNode(e) && e.__tag === 'CycleTime') as RuntimeNode).props.seconds : defaultCycle);
    const timeoutAt = performance.now() + cycle * 1000;
    const durations: number[] = [];
    for (let i = 0; i < item.iterations; i += 1) {
      if (performance.now() > timeoutAt) {
        return { id: item.id, name: item.name, status: 'failed', iterations: item.iterations, warmup: item.warmup, failure: { code: 'TSPACK_BENCHMARK_TIMEOUT', message: `benchmark timed out after ${cycle} seconds` } };
      }
      const started = performance.now();
      await Promise.resolve(context.measureFn());
      durations.push(performance.now() - started);
    }
    return buildStats(item, durations);
  } catch (error) {
    if (isSkipSignal(error)) return { id: item.id, name: item.name, status: 'skipped', iterations: item.iterations, warmup: item.warmup, skipReason: error.skipReason };
    const e = error as Error & { code?: string };
    return { id: item.id, name: item.name, status: 'failed', iterations: item.iterations, warmup: item.warmup, failure: { code: e.code, message: e.message } };
  }
}

function buildStats(item: DiscoveredBenchmark, durations: number[]): BenchmarkResult {
  const sorted = [...durations].sort((a, b) => a - b);
  const totalMs = durations.reduce((a, b) => a + b, 0);
  const meanMs = totalMs / durations.length;
  const medianMs = sorted[Math.floor(sorted.length / 2)] ?? 0;
  const p95Ms = sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * 0.95))] ?? 0;
  return { id: item.id, name: item.name, status: 'passed', iterations: item.iterations, warmup: item.warmup, totalMs, meanMs, minMs: sorted[0] ?? 0, maxMs: sorted[sorted.length - 1] ?? 0, medianMs, p95Ms, opsPerSecond: totalMs > 0 ? (item.iterations / totalMs) * 1000 : undefined };
}

function isNode(value: unknown): value is RuntimeNode { return typeof value === 'object' && value !== null && '__tag' in (value as Record<string, unknown>); }
