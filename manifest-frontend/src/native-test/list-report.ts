import path from 'node:path';
import { discoverNativeTestFile, discoverNativeTestFiles } from './discover.js';
import type {
  Diagnostic,
  DiscoverFilesResult,
  DiscoverOptions,
  FailureInfo,
  ListedTest,
  NativeTestRunReport,
  ReportedTest,
  RunFilesResult,
} from './types.js';

export function listDiscoveredTests(discovery: DiscoverFilesResult): ListedTest[] {
  const listed: ListedTest[] = [];

  for (const file of discovery.files) {
    const details = discoverNativeTestFile(file.filePath);
    const filePath = file.filePath;

    for (const fact of details.facts) {
      listed.push({
        id: `${normalizePath(filePath)}::${fact.id}`,
        filePath,
        suiteName: details.suiteName ?? '',
        name: fact.name,
        kind: 'fact',
        artifacts: fact.artifacts,
      });
    }

    for (const theory of details.theories) {
      for (const entry of theory.cases) {
        listed.push({
          id: `${normalizePath(filePath)}::${entry.id}`,
          filePath,
          suiteName: details.suiteName ?? '',
          name: theory.name,
          kind: 'theory-case',
          theoryName: theory.name,
          caseIndex: entry.index,
          caseData: entry.data,
          artifacts: theory.artifacts,
        });
      }
    }
  }

  listed.sort((a, b) => a.id.localeCompare(b.id));
  return listed;
}

export async function listNativeTests(options: DiscoverOptions): Promise<{ tests: ListedTest[]; diagnostics: Diagnostic[] }> {
  const discovered = discoverNativeTestFiles(options);
  return {
    tests: listDiscoveredTests(discovered),
    diagnostics: discovered.diagnostics,
  };
}

export function createNativeTestReport(result: RunFilesResult): NativeTestRunReport {
  const tests: ReportedTest[] = result.results.map((entry) => ({
    id: entry.id,
    name: entry.name,
    filePath: entry.id.split('::')[0],
    status: entry.status,
    durationMs: entry.durationMs,
    failure: entry.error ? normalizeFailure(entry.error) : undefined,
    skipReason: entry.skipReason,
    artifacts: entry.artifacts,
  }));

  tests.sort((a, b) => a.id.localeCompare(b.id));

  const summary = {
    total: tests.length,
    passed: tests.filter((test) => test.status === 'passed').length,
    failed: tests.filter((test) => test.status === 'failed').length,
    skipped: tests.filter((test) => test.status === 'skipped').length,
    diagnostics: result.diagnostics.length,
  };

  return { summary, tests, diagnostics: [...result.diagnostics] };
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
    if (test.status === 'skipped' && test.skipReason) {
      lines.push(`  reason: ${test.skipReason}`);
    }
    if (test.artifacts && test.artifacts.length > 0) {
      lines.push('  Artifacts:');
      for (const artifact of test.artifacts) {
        const hash = artifact.hash ? ` ${artifact.hash}` : '';
        lines.push(`    ${artifact.name} -> ${artifact.outputPath}${hash}`);
      }
    }
    lines.push('');
  }
  if (report.diagnostics.length > 0) {
    lines.push('Diagnostics:');
    for (const diagnostic of report.diagnostics) {
      lines.push(`  ${diagnostic.code}: ${diagnostic.message}`);
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

export function formatNativeTestJsonReport(report: NativeTestRunReport): string {
  return `${JSON.stringify(report, null, 2)}\n`;
}

export function nativeTestExitCode(report: NativeTestRunReport): 0 | 1 {
  if (report.summary.failed > 0) {
    return 1;
  }
  if (report.diagnostics.some((diag) => (diag.severity ?? 'error') === 'error')) {
    return 1;
  }
  return 0;
}

function normalizeFailure(error: Error & { code?: string; reason?: string; assertion?: string; expected?: unknown; actual?: unknown }): FailureInfo {
  const details: Record<string, unknown> = {};
  const anyError = error as Record<string, unknown>;
  if (anyError.tolerance !== undefined) {
    details.tolerance = anyError.tolerance;
  }
  if (anyError.difference !== undefined) {
    details.difference = anyError.difference;
  }

  return {
    code: error.code,
    message: error.message,
    reason: error.reason,
    assertion: error.assertion,
    actual: error.actual,
    expected: error.expected,
    details: Object.keys(details).length === 0 ? undefined : details,
  };
}

function normalizePath(filePath: string): string {
  return filePath.split(path.sep).join('/');
}
