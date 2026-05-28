import path from 'node:path';
import { discoverNativeTestFile, discoverNativeTestFiles } from './discover.js';
import type { ArtifactRunResult, BenchmarkRunResult, Diagnostic, DiscoverFilesResult, DiscoverOptions, FailureInfo, ListedStandaloneArtifact, ListedTest, NativeArtifactRunReport, NativeBenchmarkRunReport, NativeTestRunReport, ReportedTest, RunFilesResult } from './types.js';

export function listDiscoveredTests(discovery: DiscoverFilesResult): ListedTest[] {
  const listed: ListedTest[] = [];
  for (const file of discovery.files) {
    const details = discoverNativeTestFile(file.filePath);
    const relativeFilePath = normalizePath(path.relative(discovery.rootDir, file.filePath));
    for (const fact of details.facts) {
      listed.push({ id: `${relativeFilePath}::${fact.id}`, filePath: file.filePath, suiteName: details.suiteName ?? '', name: fact.name, kind: 'fact', artifacts: fact.artifacts });
    }
    for (const theory of details.theories) {
      for (const entry of theory.cases) {
        listed.push({ id: `${relativeFilePath}::${entry.id}`, filePath: file.filePath, suiteName: details.suiteName ?? '', name: theory.name, kind: 'theory-case', theoryName: theory.name, caseIndex: entry.index, caseData: entry.data, artifacts: theory.artifacts });
      }
    }
    for (const invariant of details.invariants) {
      listed.push({ id: `${relativeFilePath}::${invariant.id}`, filePath: file.filePath, suiteName: details.suiteName ?? '', name: invariant.name, kind: invariant.kind, artifacts: [] });
    }
  }
  listed.sort((a, b) => a.id.localeCompare(b.id));
  return listed;
}

export function listStandaloneArtifacts(discovery: DiscoverFilesResult): ListedStandaloneArtifact[] {
  const listed = discovery.files.flatMap((file) => file.standaloneArtifacts).map((item) => ({ ...item, id: normalizePath(item.id), filePath: normalizePath(item.filePath) }));
  listed.sort((a, b) => a.id.localeCompare(b.id));
  return listed;
}

export async function listNativeTests(options: DiscoverOptions): Promise<{ tests: ListedTest[]; diagnostics: Diagnostic[] }> {
  const discovered = discoverNativeTestFiles(options);
  return { tests: listDiscoveredTests(discovered), diagnostics: discovered.diagnostics };
}

export async function listNativeArtifacts(options: DiscoverOptions): Promise<{ artifacts: ListedStandaloneArtifact[]; diagnostics: Diagnostic[] }> {
  const discovered = discoverNativeTestFiles(options);
  return { artifacts: listStandaloneArtifacts(discovered), diagnostics: discovered.diagnostics };
}

export function createNativeTestReport(result: RunFilesResult): NativeTestRunReport { const tests: ReportedTest[] = result.results.map((entry) => ({ id: entry.id, name: entry.name, filePath: entry.id.split('::')[0], status: entry.status, durationMs: entry.durationMs, failure: entry.error ? normalizeFailure(entry.error) : undefined, skipReason: entry.skipReason, artifacts: entry.artifacts })); tests.sort((a, b) => a.id.localeCompare(b.id)); const summary = { total: tests.length, passed: tests.filter((test) => test.status === 'passed').length, failed: tests.filter((test) => test.status === 'failed').length, skipped: tests.filter((test) => test.status === 'skipped').length, diagnostics: result.diagnostics.length }; return { summary, tests, diagnostics: [...result.diagnostics] }; }

export function createNativeArtifactReport(result: ArtifactRunResult): NativeArtifactRunReport {
  const artifacts = [...result.artifacts].sort((a, b) => a.id.localeCompare(b.id));
  const summary = { total: artifacts.length, passed: artifacts.filter((a) => a.status === 'passed').length, failed: artifacts.filter((a) => a.status === 'failed').length, skipped: artifacts.filter((a) => a.status === 'skipped').length, diagnostics: result.diagnostics.length };
  return { summary, artifacts, diagnostics: [...result.diagnostics] };
}

export function formatNativeTestTextReport(report: NativeTestRunReport): string {
  const lines: string[] = ['Native xTest results', ''];
  for (const test of report.tests) {
    const prefix = test.status === 'passed' ? 'PASS' : test.status === 'failed' ? 'FAIL' : 'SKIP';
    lines.push(`${prefix} ${test.id}`);
    if (test.status === 'failed' && test.failure) {
      if (test.failure.code) lines.push(`  code: ${test.failure.code}`);
      lines.push(`  message: ${test.failure.message}`);
      if (test.failure.reason) lines.push(`  reason: ${test.failure.reason}`);
      if (test.failure.expected !== undefined) lines.push(`  expected: ${JSON.stringify(test.failure.expected)}`);
      if (test.failure.actual !== undefined) lines.push(`  actual: ${JSON.stringify(test.failure.actual)}`);
      if (test.failure.details) {
        for (const key of Object.keys(test.failure.details).sort()) {
          lines.push(`  ${key}: ${JSON.stringify(test.failure.details[key])}`);
        }
      }
    }
    if (test.status === 'skipped' && test.skipReason) lines.push(`  reason: ${test.skipReason}`);
    if (test.artifacts && test.artifacts.length > 0) {
      lines.push('  Artifacts:');
      for (const artifact of test.artifacts) {
        const hash = artifact.hash ? ` ${artifact.hash}` : '';
        lines.push(`    ${artifact.name} -> ${artifact.outputPath}${hash}`);
      }
    }
    lines.push('');
  }
  lines.push('Summary:');
  lines.push(`  total: ${report.summary.total}`);
  lines.push(`  passed: ${report.summary.passed}`);
  lines.push(`  failed: ${report.summary.failed}`);
  lines.push(`  skipped: ${report.summary.skipped}`);
  lines.push(`  diagnostics: ${report.summary.diagnostics}`);
  return `${lines.join('\n')}\n`;
}

export function formatNativeArtifactTextReport(report: NativeArtifactRunReport): string {
  const lines: string[] = ['TSPack artifacts', ''];
  for (const entry of report.artifacts) {
    const prefix = entry.status === 'passed' ? 'PASS' : entry.status === 'failed' ? 'FAIL' : 'SKIP';
    lines.push(`${prefix} ${entry.id}`);
    if (entry.artifact?.written) {
      lines.push(`  output: ${entry.artifact.outputPath}`);
      if (entry.artifact.hash) lines.push(`  hash: ${entry.artifact.hash}`);
      if (entry.artifact.reason) lines.push(`  reason: ${entry.artifact.reason}`);
    }
    if (entry.failure) {
      if (entry.failure.code) lines.push(`  code: ${entry.failure.code}`);
      lines.push(`  message: ${entry.failure.message}`);
      if (entry.failure.reason) lines.push(`  reason: ${entry.failure.reason}`);
    }
    if (entry.skipReason) lines.push(`  reason: ${entry.skipReason}`);
    lines.push('');
  }
  lines.push('Summary:');
  lines.push(`  total: ${report.summary.total}`);
  lines.push(`  passed: ${report.summary.passed}`);
  lines.push(`  failed: ${report.summary.failed}`);
  lines.push(`  skipped: ${report.summary.skipped}`);
  lines.push(`  diagnostics: ${report.summary.diagnostics}`);
  return `${lines.join('\n')}\n`;
}

export function formatNativeTestJsonReport(report: NativeTestRunReport): string { return `${JSON.stringify(report, null, 2)}\n`; }
export function formatNativeArtifactJsonReport(report: NativeArtifactRunReport): string { return `${JSON.stringify(report, null, 2)}\n`; }

export function nativeTestExitCode(report: NativeTestRunReport): 0 | 1 { if (report.summary.failed > 0) return 1; if (report.diagnostics.some((diag) => (diag.severity ?? 'error') === 'error')) return 1; return 0; }
export function nativeArtifactExitCode(report: NativeArtifactRunReport): 0 | 1 { if (report.summary.failed > 0) return 1; if (report.diagnostics.some((diag) => (diag.severity ?? 'error') === 'error')) return 1; return 0; }
export function createNativeBenchmarkReport(result: BenchmarkRunResult): NativeBenchmarkRunReport {
  const benchmarks = [...result.benchmarks].sort((a, b) => a.id.localeCompare(b.id));
  const summary = { total: benchmarks.length, passed: benchmarks.filter((b) => b.status === 'passed').length, failed: benchmarks.filter((b) => b.status === 'failed').length, skipped: benchmarks.filter((b) => b.status === 'skipped').length, diagnostics: result.diagnostics.length };
  return { summary, benchmarks, diagnostics: [...result.diagnostics] };
}
export function formatNativeBenchmarkTextReport(report: NativeBenchmarkRunReport): string {
  const lines: string[] = ['TSPack benchmarks', ''];
  for (const benchmark of report.benchmarks) {
    const prefix = benchmark.status === 'passed' ? 'PASS' : benchmark.status === 'failed' ? 'FAIL' : 'SKIP';
    lines.push(`${prefix} ${benchmark.id}`);
    lines.push(`  iterations: ${benchmark.iterations}`);
    lines.push(`  warmup: ${benchmark.warmup}`);
    if (benchmark.totalMs !== undefined) lines.push(`  total: ${benchmark.totalMs.toFixed(6)} ms`);
    if (benchmark.meanMs !== undefined) lines.push(`  mean: ${benchmark.meanMs.toFixed(6)} ms`);
    if (benchmark.minMs !== undefined) lines.push(`  min: ${benchmark.minMs.toFixed(6)} ms`);
    if (benchmark.maxMs !== undefined) lines.push(`  max: ${benchmark.maxMs.toFixed(6)} ms`);
    if (benchmark.medianMs !== undefined) lines.push(`  median: ${benchmark.medianMs.toFixed(6)} ms`);
    if (benchmark.p95Ms !== undefined) lines.push(`  p95: ${benchmark.p95Ms.toFixed(6)} ms`);
    if (benchmark.opsPerSecond !== undefined) lines.push(`  ops/sec: ${benchmark.opsPerSecond.toFixed(2)}`);
    if (benchmark.failure?.code) lines.push(`  code: ${benchmark.failure.code}`);
    if (benchmark.failure?.message) lines.push(`  message: ${benchmark.failure.message}`);
    if (benchmark.skipReason) lines.push(`  reason: ${benchmark.skipReason}`);
    lines.push('');
  }
  lines.push('Summary:');
  lines.push(`  total: ${report.summary.total}`);
  lines.push(`  passed: ${report.summary.passed}`);
  lines.push(`  failed: ${report.summary.failed}`);
  lines.push(`  skipped: ${report.summary.skipped}`);
  lines.push(`  diagnostics: ${report.summary.diagnostics}`);
  return `${lines.join('\n')}\n`;
}
export function formatNativeBenchmarkJsonReport(report: NativeBenchmarkRunReport): string { return `${JSON.stringify(report, null, 2)}\n`; }
export function nativeBenchmarkExitCode(report: NativeBenchmarkRunReport): 0 | 1 { if (report.summary.failed > 0) return 1; if (report.diagnostics.some((diag) => (diag.severity ?? 'error') === 'error')) return 1; return 0; }

function normalizeFailure(error: Error & { code?: string; reason?: string; assertion?: string; expected?: unknown; actual?: unknown }): FailureInfo {
  const details: Record<string, unknown> = {};
  const anyError = error as Record<string, unknown>;
  if (anyError.tolerance !== undefined) details.tolerance = anyError.tolerance;
  if (anyError.difference !== undefined) details.difference = anyError.difference;
  return { code: error.code, message: error.message, reason: error.reason, assertion: error.assertion, actual: error.actual, expected: error.expected, details: Object.keys(details).length === 0 ? undefined : details };
}

function normalizePath(filePath: string): string {
  let normalized = filePath.replace(/\\/g, '/').split(path.sep).join('/');
  while (normalized.startsWith('./')) {
    normalized = normalized.slice(2);
  }
  return normalized;
}
