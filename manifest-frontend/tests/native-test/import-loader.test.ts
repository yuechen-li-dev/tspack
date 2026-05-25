import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { expect, it } from 'vitest';
import { runNativeArtifacts, runNativeBenchmarks, runNativeProphecies, runNativeTestFiles } from '../../src/native-test';

function makeDir(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'native-imports-'));
}

function nativeImportPath(): string {
  return path.resolve(process.cwd(), 'src/native-test/index.ts').replace(/\\/g, '/');
}

it('loads relative ts/tsx/js/jsx imports with extensionless/index/re-export and side effects', async () => {
  const root = makeDir();
  const native = nativeImportPath();
  fs.mkdirSync(path.join(root, 'src', 'deep'), { recursive: true });
  fs.writeFileSync(path.join(root, 'src', 'math.ts'), 'export const add = (a:number,b:number) => a+b;');
  fs.writeFileSync(path.join(root, 'src', 'default.tsx'), 'export default function v(){ return 4; }');
  fs.writeFileSync(path.join(root, 'src', 'side.js'), 'globalThis.__side = (globalThis.__side ?? 0) + 1;');
  fs.writeFileSync(path.join(root, 'src', 'deep', 'index.ts'), 'export * from "../math";');
  fs.writeFileSync(path.join(root, 'src', 'barrel.ts'), 'export { add } from "./deep";');
  fs.writeFileSync(path.join(root, 'src', 'nested.ts'), 'import { add } from "./barrel"; export const n = add(2,3);');
  fs.writeFileSync(path.join(root, 'a.xtest.tsx'), `import { Suite, Fact, assert } from '${native}'; import './src/side'; import { n } from './src/nested'; import d from './src/default.tsx'; export default (<Suite name="s"><Fact name="ok">{() => { assert.equal(n + d(), 9, 'ok'); assert.equal((globalThis as any).__side, 1, 'side'); }}</Fact></Suite>);`);

  const run = await runNativeTestFiles({ rootDir: root });
  expect(run.results[0]?.status).toBe('passed');
});

it('reports missing and outside-root imports and supports artifact/benchmark/prophecy paths', async () => {
  const root = makeDir();
  const native = nativeImportPath();
  fs.mkdirSync(path.join(root, 'src'), { recursive: true });
  fs.writeFileSync(path.join(root, 'src', 'ok.ts'), 'export const ok = () => 1;');
  fs.writeFileSync(path.join(root, 'x.valid.tsx'), `import { Suite, Valid, assert } from '${native}'; import { ok } from './src/ok'; export default (<Suite name="v"><Valid name="x">{() => { assert.equal(ok(), 1, 'ok'); }}</Valid></Suite>);`);
  fs.writeFileSync(path.join(root, 'y.invalid.tsx'), `import { Suite, Invalid, expect } from '${native}'; import { ok } from './src/ok'; export default (<Suite name="i"><Invalid name="x">{() => { expect.error([{ code: ok() ? 'E' : 'X' }], 'E').because('ok'); }}</Invalid></Suite>);`);
  fs.writeFileSync(path.join(root, 'z.xtest.tsx'), `import { Suite, Artifact } from '${native}'; import { ok } from './src/ok'; export default (<Suite name="a"><Artifact name="o" path="o.txt">{async ({ artifact }) => { await artifact.writeText('o', String(ok()), 'ok'); }}</Artifact></Suite>);`);
  fs.writeFileSync(path.join(root, 'b.benchmark.tsx'), `import { Suite, Benchmark } from '${native}'; import { ok } from './src/ok'; export default (<Suite name="b"><Benchmark name="x">{({ bench }) => { bench.measure(() => ok()); }}</Benchmark></Suite>);`);
  fs.writeFileSync(path.join(root, 'p.prophecy.tsx'), `import { Suite, Prophecy, Foretell } from '${native}'; import { ok } from './src/ok'; export default (<Suite name="p"><Prophecy name="x"><Foretell reason="r" />{() => { if (ok()===1) { throw new Error('boom'); } }}</Prophecy></Suite>);`);
  const run = await runNativeTestFiles({ rootDir: root });
  expect(run.results.length).toBeGreaterThanOrEqual(2);
  expect((await runNativeArtifacts({ rootDir: root })).artifacts[0].status).toBe('passed');
  expect((await runNativeBenchmarks({ rootDir: root })).benchmarks[0].status).toBe('passed');
  expect((await runNativeProphecies({ rootDir: root })).prophecies).toHaveLength(1);

  fs.writeFileSync(path.join(root, 'missing.xtest.tsx'), `import { Suite, Fact } from '${native}'; import { no } from './src/nope'; export default (<Suite name="m"><Fact name="x">{() => { void no; }}</Fact></Suite>);`);
  const missing = await runNativeTestFiles({ rootDir: root, files: ['missing.xtest.tsx'] });
  expect(missing.diagnostics.some((d) => d.code === 'TSPACK_TEST_MODULE_LOAD_FAILED' && d.message.includes('TSPACK_TEST_IMPORT_NOT_FOUND'))).toBe(true);

  fs.writeFileSync(path.join(root, 'outside.xtest.tsx'), `import { Suite, Fact } from '${native}'; import '../outside'; export default (<Suite name="o"><Fact name="x">{() => {}}</Fact></Suite>);`);
  const outside = await runNativeTestFiles({ rootDir: root, files: ['outside.xtest.tsx'] });
  expect(outside.diagnostics.some((d) => d.message.includes('TSPACK_TEST_IMPORT_OUTSIDE_ROOT') || d.message.includes('TSPACK_TEST_IMPORT_NOT_FOUND'))).toBe(true);
});
