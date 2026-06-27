import fs from 'node:fs';
import path from 'node:path';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { discoverNativeTestFiles, discoverNativeTestFile } from './discover.js';
import type { Diagnostic, DiscoveredProphecy, DoomEnvelope, DoomResult, DoomRunResult, NativeDoomRunReport } from './types.js';

export function listNativeProphecies(options: { rootDir: string }): { prophecies: DiscoveredProphecy[]; diagnostics: Diagnostic[] } {
  const root = path.resolve(options.rootDir);
  const discovery = discoverNativeTestFiles({ rootDir: root });
  const prophecies: DiscoveredProphecy[] = [];
  for (const file of discovery.files) {
    if (!file.filePath.endsWith('.prophecy.tsx')) {
      continue;
    }
    const detailed = discoverNativeTestFile(file.filePath);
    prophecies.push(...(detailed.prophecies ?? []).map((entry) => ({ ...entry, filePath: path.relative(root, entry.filePath).split(path.sep).join('/') })));
  }
  prophecies.sort((a, b) => a.id.localeCompare(b.id));
  return { prophecies, diagnostics: discovery.diagnostics };
}

export async function runNativeProphecies(options: { rootDir: string; filter?: string; outDir?: string; listOnly?: boolean }): Promise<DoomRunResult> {
  const listed = listNativeProphecies({ rootDir: options.rootDir });
  const diagnostics = [...listed.diagnostics];
  const selected = listed.prophecies.filter((item) => !options.filter || item.id.includes(options.filter) || item.name.includes(options.filter));
  if (options.filter && selected.length === 0) {
    diagnostics.push({ code: 'TSPACK_DOOM_FILTER_NO_MATCH', message: `doom filter matched no prophecies: ${options.filter}`, file: path.resolve(options.rootDir), severity: 'error' });
  }
  if (options.listOnly) {
    return { prophecies: selected.map((item) => ({ id: item.id, name: item.name, status: 'passed' })), diagnostics };
  }

  const outRoot = path.resolve(options.outDir ?? path.join(options.rootDir, '.tspack', 'doom-artifacts'));
  fs.mkdirSync(outRoot, { recursive: true });
  const results: DoomResult[] = [];
  for (const prophecy of selected) {
    results.push(await runOneProphecy(path.resolve(options.rootDir), prophecy, outRoot));
  }
  return { prophecies: results, diagnostics };
}

async function runOneProphecy(rootDir: string, prophecy: DiscoveredProphecy, outRoot: string): Promise<DoomResult> {
  const folder = sanitizeId(prophecy.id);
  const runDir = path.join(outRoot, folder);
  fs.mkdirSync(runDir, { recursive: true });
  const envelopePath = path.join(runDir, 'envelope.json');
  const stdoutPath = path.join(runDir, 'stdout.txt');
  const stderrPath = path.join(runDir, 'stderr.txt');

  const bridge = path.resolve(
    path.dirname(fileURLToPath(import.meta.url)),
    '../../dist/src/native-test-cli.js',
  );
  const args = [bridge, 'doom-child', '--root', rootDir, '--file', prophecy.filePath, '--id', prophecy.id, '--out', runDir];

  let stdout = '';
  let stderr = '';
  try {
    const outcome = await new Promise<{ code: number | null; signal: NodeJS.Signals | null; timedOut: boolean }>((resolve, reject) => {
      const child = spawn('node', args, { cwd: rootDir, stdio: ['ignore', 'pipe', 'pipe'] });
      let timedOut = false;
      const timeoutHandle = setTimeout(() => {
        timedOut = true;
        child.kill('SIGKILL');
        resolve({ code: null, signal: 'SIGKILL', timedOut: true });
      }, prophecy.cycleTimeSeconds * 1000);
      child.stdout.on('data', (chunk) => {
        stdout += String(chunk);
      });
      child.stderr.on('data', (chunk) => {
        stderr += String(chunk);
      });
      child.on('error', reject);
      child.on('exit', (code, signal) => {
        clearTimeout(timeoutHandle);
        resolve({ code, signal, timedOut });
      });
    });

    fs.writeFileSync(stdoutPath, stdout, 'utf8');
    fs.writeFileSync(stderrPath, stderr, 'utf8');

    if (outcome.timedOut) {
      return {
        id: prophecy.id,
        name: prophecy.name,
        status: 'failed',
        exitCode: outcome.code,
        signal: outcome.signal,
        stdout,
        stderr,
        envelopePath,
        failure: {
          code: 'TSPACK_DOOM_TIMEOUT',
          message: `prophecy timed out after ${prophecy.cycleTimeSeconds} seconds`,
          details: { timeoutSeconds: prophecy.cycleTimeSeconds },
        },
      };
    }
    if (outcome.code === 0 && !outcome.signal) {
      return { id: prophecy.id, name: prophecy.name, status: 'failed', exitCode: outcome.code, signal: outcome.signal, stdout, stderr, envelopePath, failure: { code: 'TSPACK_DOOM_DID_NOT_TERMINATE', message: 'prophecy body completed normally' } };
    }
    if (!fs.existsSync(envelopePath)) {
      return { id: prophecy.id, name: prophecy.name, status: 'failed', exitCode: outcome.code, signal: outcome.signal, stdout, stderr, envelopePath, failure: { code: 'TSPACK_DOOM_ENVELOPE_MISSING', message: 'doom envelope missing' } };
    }
    let envelope: DoomEnvelope;
    try {
      envelope = JSON.parse(fs.readFileSync(envelopePath, 'utf8')) as DoomEnvelope;
    } catch (error) {
      return { id: prophecy.id, name: prophecy.name, status: 'failed', exitCode: outcome.code, signal: outcome.signal, stdout, stderr, envelopePath, failure: { code: 'TSPACK_DOOM_ENVELOPE_INVALID', message: (error as Error).message } };
    }
    if (envelope.foretell?.reason !== prophecy.foretell.reason) {
      return { id: prophecy.id, name: prophecy.name, status: 'failed', exitCode: outcome.code, signal: outcome.signal, stdout, stderr, envelopePath, envelope, failure: { code: 'TSPACK_DOOM_ENVELOPE_MISMATCH', message: 'doom envelope foretell reason mismatch' } };
    }
    return { id: prophecy.id, name: prophecy.name, status: 'passed', exitCode: outcome.code, signal: outcome.signal, stdout, stderr, envelopePath, envelope };
  } catch (error) {
    return { id: prophecy.id, name: prophecy.name, status: 'failed', failure: { code: 'TSPACK_DOOM_CHILD_LAUNCH_FAILED', message: (error as Error).message } };
  }
}

export function createNativeDoomReport(result: DoomRunResult): NativeDoomRunReport {
  const prophecies = [...result.prophecies].sort((a, b) => a.id.localeCompare(b.id));
  return {
    summary: { total: prophecies.length, passed: prophecies.filter((p) => p.status === 'passed').length, failed: prophecies.filter((p) => p.status === 'failed').length, skipped: prophecies.filter((p) => p.status === 'skipped').length, diagnostics: result.diagnostics.length },
    prophecies,
    diagnostics: [...result.diagnostics],
  };
}

export function formatNativeDoomTextReport(report: NativeDoomRunReport): string { const lines = ['TSPack doom', '']; for (const item of report.prophecies) { lines.push(`${item.status === 'passed' ? 'PASS' : item.status === 'failed' ? 'FAIL' : 'SKIP'} ${item.id}`); if (item.envelope?.foretell?.reason) lines.push(`  foretold: ${item.envelope.foretell.reason}`); if (item.signal) lines.push(`  exit: signal ${item.signal}`); if (item.exitCode !== undefined && item.exitCode !== null) lines.push(`  exit: code ${item.exitCode}`); if (item.envelopePath) lines.push(`  envelope: ${item.envelopePath}`); if (item.failure?.code) lines.push(`  code: ${item.failure.code}`); lines.push(''); } lines.push('Summary:'); lines.push(`  total: ${report.summary.total}`); lines.push(`  passed: ${report.summary.passed}`); lines.push(`  failed: ${report.summary.failed}`); lines.push(`  skipped: ${report.summary.skipped}`); lines.push(`  diagnostics: ${report.summary.diagnostics}`); return `${lines.join('\n')}\n`; }
export function formatNativeDoomJsonReport(report: NativeDoomRunReport): string { return `${JSON.stringify(report, null, 2)}\n`; }
export function nativeDoomExitCode(report: NativeDoomRunReport): 0 | 1 { return report.summary.failed > 0 || report.diagnostics.some((d) => (d.severity ?? 'error') === 'error') ? 1 : 0; }

function sanitizeId(id: string): string {
  return id.replaceAll(/[^a-zA-Z0-9._-]/g, '_');
}
