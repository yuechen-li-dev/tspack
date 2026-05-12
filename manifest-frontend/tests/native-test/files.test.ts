import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import { createNativeTestReport, listNativeArtifacts, listNativeTests, runNativeTestFiles } from '../../src/native-test';

function makeDir(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'tspack-native-files-'));
}

function nativeImportPath(): string {
  return path.resolve(process.cwd(), 'src/native-test/index.ts').split(path.sep).join('/');
}

describe('native valid/invalid file execution', () => {
  it('runs valid/invalid pass/fail/async/skip and filter behavior', async () => {
    const root = makeDir();
    const importPath = nativeImportPath();

    fs.writeFileSync(path.join(root, 'fixtures.valid.tsx'), `
      import { Suite, Valid, skip, assert, expect } from '${importPath}';
      export default (
        <Suite name="v">
          <Valid name="pass">{() => { expect.noErrors([]).because('no errors'); }}</Valid>
          <Valid name="assert fail">{() => { assert.equal(1, 2, 'should fail'); }}</Valid>
          <Valid name="skip">{() => { skip('skip valid'); }}</Valid>
          <Valid name="async">{async () => { await Promise.resolve(); assert.true(true, 'async valid'); }}</Valid>
          <Valid name="pending because">{() => { expect(1).toBe(1); }}</Valid>
        </Suite>
      );
    `);

    fs.writeFileSync(path.join(root, 'fixtures.invalid.tsx'), `
      import { Suite, Invalid, skip, assert, expect } from '${importPath}';
      export default (
        <Suite name="i">
          <Invalid name="error match">{() => { expect.error([{ code: 'E_MATCH' }], 'E_MATCH').because('must match'); }}</Invalid>
          <Invalid name="error missing">{() => { expect.error([{ code: 'E_OTHER' }], 'E_EXPECTED').because('must fail'); }}</Invalid>
          <Invalid name="noErrors fail">{() => { expect.noErrors([{ code: 'E', severity: 'error' }]).because('error should fail'); }}</Invalid>
          <Invalid name="skip">{() => { skip('skip invalid'); }}</Invalid>
          <Invalid name="async">{async () => { await Promise.resolve(); assert.true(true, 'async invalid'); }}</Invalid>
          <Invalid name="pending because">{() => { expect(1).toBe(1); }}</Invalid>
        </Suite>
      );
    `);

    fs.writeFileSync(path.join(root, 'artifact.xtest.tsx'), `
      import { Suite, Artifact } from '${importPath}';
      export default (<Suite name="art"><Artifact name="standalone" path="a.txt">{async ({ artifact }) => { await artifact.writeText('standalone', 'ok', 'proof'); }}</Artifact></Suite>);
    `);

    const run = await runNativeTestFiles({ rootDir: root });
    const statusById = new Map(run.results.map((entry) => [entry.id, entry.status]));

    expect(statusById.get('fixtures.valid.tsx::v/valid/pass')).toBe('passed');
    expect(statusById.get('fixtures.valid.tsx::v/valid/assert fail')).toBe('failed');
    expect(statusById.get('fixtures.valid.tsx::v/valid/skip')).toBe('skipped');
    expect(statusById.get('fixtures.valid.tsx::v/valid/async')).toBe('passed');
    expect(statusById.get('fixtures.valid.tsx::v/valid/pending because')).toBe('failed');

    expect(statusById.get('fixtures.invalid.tsx::i/invalid/error match')).toBe('passed');
    expect(statusById.get('fixtures.invalid.tsx::i/invalid/error missing')).toBe('failed');
    expect(statusById.get('fixtures.invalid.tsx::i/invalid/noErrors fail')).toBe('failed');
    expect(statusById.get('fixtures.invalid.tsx::i/invalid/skip')).toBe('skipped');
    expect(statusById.get('fixtures.invalid.tsx::i/invalid/async')).toBe('passed');
    expect(statusById.get('fixtures.invalid.tsx::i/invalid/pending because')).toBe('failed');

    const filterValid = await runNativeTestFiles({ rootDir: root, filter: 'valid/pass' });
    expect(filterValid.results.map((entry) => entry.id)).toEqual(['fixtures.valid.tsx::v/valid/pass']);

    const filterInvalid = await runNativeTestFiles({ rootDir: root, filter: 'error match' });
    expect(filterInvalid.results.map((entry) => entry.id)).toEqual(['fixtures.invalid.tsx::i/invalid/error match']);

    const noMatch = await runNativeTestFiles({ rootDir: root, filter: 'not-there' });
    const report = createNativeTestReport(noMatch);
    expect(report.diagnostics.some((entry) => entry.code === 'TSPACK_TEST_FILTER_NO_MATCH')).toBe(true);

    expect(run.results.some((entry) => entry.id.includes('/artifact/'))).toBe(false);
  });

  it('list APIs include valid/invalid and artifact list excludes them', async () => {
    const root = makeDir();
    const importPath = nativeImportPath();
    fs.writeFileSync(path.join(root, 'v.valid.tsx'), `import { Suite, Valid } from '${importPath}'; export default (<Suite name="s"><Valid name="ok">{() => {}}</Valid></Suite>);`);
    fs.writeFileSync(path.join(root, 'i.invalid.tsx'), `import { Suite, Invalid } from '${importPath}'; export default (<Suite name="s"><Invalid name="bad">{() => {}}</Invalid></Suite>);`);
    fs.writeFileSync(path.join(root, 'x.xtest.tsx'), `import { Suite, Artifact } from '${importPath}'; export default (<Suite name="s"><Artifact name="a" path="a.txt">{() => {}}</Artifact></Suite>);`);

    const listedTests = await listNativeTests({ rootDir: root });
    expect(listedTests.tests.some((entry) => entry.id.includes('v.valid.tsx::s/valid/ok'))).toBe(true);
    expect(listedTests.tests.some((entry) => entry.id.includes('i.invalid.tsx::s/invalid/bad'))).toBe(true);

    const listedArtifacts = await listNativeArtifacts({ rootDir: root });
    expect(listedArtifacts.artifacts.every((entry) => !entry.id.includes('/valid/') && !entry.id.includes('/invalid/'))).toBe(true);
  });
});
