import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import {
  createNativeTestReport,
  formatNativeTestJsonReport,
  formatNativeTestTextReport,
  listNativeTests,
  nativeTestExitCode,
  runNativeTestFiles,
} from '../../src/native-test';

function makeDir(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'tspack-native-report-'));
}

describe('native test listing/filter/report', () => {
  it('lists facts and theory cases without execution', async () => {
    const root = path.resolve(process.cwd(), 'tests/native-test/fixtures');
    const listed = await listNativeTests({ rootDir: root });
    expect(listed.tests.some((test) => test.id.includes('side-effect.xtest.tsx::side/body'))).toBe(true);
    expect(listed.tests.some((test) => test.kind === 'theory-case')).toBe(true);
  });

  it('filter no match is diagnostic and exit code 1', async () => {
    const root = path.resolve(process.cwd(), 'tests/native-test/fixtures');
    const result = await runNativeTestFiles({ rootDir: root, filter: 'no-such-test' });
    const report = createNativeTestReport(result);
    expect(result.results).toHaveLength(0);
    expect(report.diagnostics.some((entry) => entry.code === 'TSPACK_TEST_FILTER_NO_MATCH')).toBe(true);
    expect(nativeTestExitCode(report)).toBe(1);
  });

  it('filters before import and provides deterministic reports', async () => {
    const root = makeDir();
    const sideEffectFile = path.join(root, 'side-effect.xtest.tsx');
    const targetFile = path.join(root, 'target.xtest.tsx');

    fs.writeFileSync(sideEffectFile, `
      import fs from 'node:fs';
      import path from 'node:path';
      import { Suite, Fact } from '${path.resolve(process.cwd(), 'src/native-test/index.ts').replace(/\\/g, '/')}';
      fs.writeFileSync(path.join('${root.replace(/\\/g, '/')}', 'executed.txt'), 'ran');
      export default (<Suite name="side"><Fact name="boom">{() => {}}</Fact></Suite>);
    `);

    fs.writeFileSync(targetFile, `
      import { Suite, Fact, Theory, Case, assert, skip, Artifact } from '${path.resolve(process.cwd(), 'src/native-test/index.ts').replace(/\\/g, '/')}';
      export default (
        <Suite name="report">
          <Fact name="pass">
            <Artifact name="out" path="out.json" format="json" />
            {async ({ artifact }) => { await artifact.writeJson('out', { ok: true }, 'proof'); assert.true(true, 'pass'); }}
          </Fact>
          <Fact name="skip">{() => { skip('not now'); }}</Fact>
          <Fact name="near fail">{() => { assert.near(3.2, 3.14159, 0.001, 'circle'); }}</Fact>
          <Theory name="many"><Case input={1} /><Case input={2} />{() => { assert.true(true, 'ok'); }}</Theory>
        </Suite>
      );
    `);

    const result = await runNativeTestFiles({ rootDir: root, filter: 'report' });
    expect(fs.existsSync(path.join(root, 'executed.txt'))).toBe(false);

    const report = createNativeTestReport(result);
    const text = formatNativeTestTextReport(report);
    const json1 = formatNativeTestJsonReport(report);
    const json2 = formatNativeTestJsonReport(report);

    expect(text.includes('PASS target.xtest.tsx::report/pass')).toBe(true);
    expect(text.includes('SKIP target.xtest.tsx::report/skip')).toBe(true);
    expect(text.includes('FAIL target.xtest.tsx::report/near fail')).toBe(true);
    expect(text.includes('tolerance')).toBe(true);
    expect(text.includes('difference')).toBe(true);
    expect(text.includes('Artifacts:')).toBe(true);
    expect(JSON.parse(json1).summary.total).toBe(5);
    expect(json1).toBe(json2);
    expect(nativeTestExitCode(report)).toBe(1);
  });
});
