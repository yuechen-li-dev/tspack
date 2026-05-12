import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import { discoverNativeTestFile, discoverNativeTestFiles, runNativeTestFiles } from '../../src/native-test';

function makeDir(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'tspack-native-files-'));
}

describe('native file discovery and execution', () => {
  it('discovers only .xtest.tsx files in deterministic order', () => {
    const root = makeDir();
    fs.writeFileSync(path.join(root, 'b.xtest.tsx'), 'export default (<Suite name="s"><Fact name="b">{() => {}}</Fact></Suite>);');
    fs.writeFileSync(path.join(root, 'a.xtest.tsx'), 'export default (<Suite name="s"><Fact name="a">{() => {}}</Fact></Suite>);');
    fs.writeFileSync(path.join(root, 'ignored.test.tsx'), 'export default null;');
    fs.writeFileSync(path.join(root, 'ignored.spec.tsx'), 'export default null;');
    fs.writeFileSync(path.join(root, 'ignored.tsx'), 'export default null;');

    const result = discoverNativeTestFiles({ rootDir: root });
    expect(result.files.map((f) => path.basename(f.filePath))).toEqual(['a.xtest.tsx', 'b.xtest.tsx']);
    expect(result.files.flatMap((f) => f.tests.map((t) => t.id))).toEqual(['a.xtest.tsx::s/a', 'b.xtest.tsx::s/b']);
  });

  it('returns non-native file diagnostic for explicit file discovery', () => {
    const root = makeDir();
    const file = path.join(root, 'not-native.tsx');
    fs.writeFileSync(file, 'export default null;');
    const result = discoverNativeTestFile(file);
    expect(result.diagnostics.some((d) => d.code === 'TSPACK_TEST_NON_NATIVE_FILE')).toBe(true);
  });

  it('supports listOnly without executing callback bodies', async () => {
    const root = path.resolve(process.cwd(), 'tests/native-test/fixtures');
    const result = await runNativeTestFiles({ rootDir: root, listOnly: true });
    expect(result.results.some((r) => r.id.includes('side-effect.xtest.tsx::side/body'))).toBe(true);
  });

  it('loads .xtest.tsx that imports skip and reports skipped statuses', async () => {
    const root = path.resolve(process.cwd(), 'tests/native-test/fixtures');
    const result = await runNativeTestFiles({ rootDir: root, files: ['skip.xtest.tsx'] });
    expect(result.results.map((r) => r.status)).toEqual(['skipped', 'skipped', 'passed']);
    expect(result.results[0].skipReason).toBe('demonstrates runtime conditional skip');
    expect(result.results[1].skipReason).toBe('case 1 intentionally skipped');
  });

  it('runs loaded fact and theory tests, including async and failures', async () => {
    const root = makeDir();
    fs.writeFileSync(path.join(root, 'run.xtest.tsx'), `
      import { Suite, Fact, Theory, Case, assert, expect } from '${path.resolve(process.cwd(), 'src/native-test/index.ts').replace(/\\/g, '/')}';
      export default (
        <Suite name="exec">
          <Fact name="async ok">{async () => { await Promise.resolve(); assert.true(true, 'async body should pass'); }}</Fact>
          <Fact name="assert fail">{() => { assert.equal(1, 2, 'failing assertion reason'); }}</Fact>
          <Fact name="missing because">{() => { expect(1).toBe(1); }}</Fact>
          <Theory name="len"><Case input="x" expected={1} />{({ input, expected }) => { assert.equal(input.length, expected, 'theory works'); }}</Theory>
        </Suite>
      );
    `);

    const result = await runNativeTestFiles({ rootDir: root });
    expect(result.results.map((r) => r.id)).toEqual([
      'run.xtest.tsx::exec/async ok',
      'run.xtest.tsx::exec/assert fail',
      'run.xtest.tsx::exec/missing because',
      'run.xtest.tsx::exec/len[0]',
    ]);
    expect(result.results.map((r) => r.status)).toEqual(['passed', 'failed', 'failed', 'passed']);
  });
});
