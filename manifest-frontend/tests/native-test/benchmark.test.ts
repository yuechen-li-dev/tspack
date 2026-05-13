import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import { createNativeBenchmarkReport, formatNativeBenchmarkJsonReport, formatNativeBenchmarkTextReport, nativeBenchmarkExitCode, runNativeBenchmarks } from '../../src/native-test';

const importPath = path.resolve(process.cwd(), 'src/native-test/index.ts').replace(/\\/g, '/');

describe('native benchmarks', () => {
  it('runs benchmark and reports metrics', async () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'bench-'));
    fs.writeFileSync(path.join(root, 'a.benchmark.tsx'), `
      import { Suite, Benchmark } from '${importPath}';
      export default (<Suite name="s"><Benchmark name="b">{({ bench }) => { bench.measure(() => { let x = 1 + 1; return x; }); }}</Benchmark></Suite>);
    `);
    const run = await runNativeBenchmarks({ rootDir: root });
    expect(run.benchmarks[0].status).toBe('passed');
    expect(run.benchmarks[0].totalMs).toBeGreaterThanOrEqual(0);
    const report = createNativeBenchmarkReport(run);
    expect(formatNativeBenchmarkTextReport(report)).toContain('TSPack benchmarks');
    expect(JSON.parse(formatNativeBenchmarkJsonReport(report)).summary.total).toBe(1);
    expect(nativeBenchmarkExitCode(report)).toBe(0);
  });
});
