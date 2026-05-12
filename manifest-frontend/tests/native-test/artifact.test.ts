import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import {
  createNativeArtifactReport,
  formatNativeArtifactJsonReport,
  formatNativeArtifactTextReport,
  listNativeArtifacts,
  nativeArtifactExitCode,
  runNativeArtifacts,
  runNativeTestFiles,
} from '../../src/native-test';

function makeDir(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'tspack-artifact-test-'));
}

describe('standalone artifact flow', () => {
  it('lists standalone artifacts only and does not execute bodies', async () => {
    const root = makeDir();
    fs.writeFileSync(path.join(root, 'a.xtest.tsx'), `
      import fs from 'node:fs';
      import path from 'node:path';
      import { Suite, Artifact, Fact } from '${path.resolve(process.cwd(), 'src/native-test/index.ts').replace(/\\/g, '/')}';
      export default (<Suite name="s"><Artifact name="manifest" path="manifest.json">{() => { fs.writeFileSync(path.join('${root.replace(/\\/g, '/')}', 'ran.txt'), 'x'); }}</Artifact><Fact name="f"><Artifact name="test-art" path="t.json" />{() => {}}</Fact></Suite>);
    `);
    const listed = await listNativeArtifacts({ rootDir: root });
    expect(listed.artifacts).toHaveLength(1);
    expect(listed.artifacts[0].id.includes('::s/artifact/manifest')).toBe(true);
    expect(fs.existsSync(path.join(root, 'ran.txt'))).toBe(false);
  });

  it('runs json/text/bytes and reports deterministic text/json', async () => {
    const root = makeDir();
    const out = makeDir();
    fs.writeFileSync(path.join(root, 'w.xtest.tsx'), `
      import { Suite, Artifact } from '${path.resolve(process.cwd(), 'src/native-test/index.ts').replace(/\\/g, '/')}';
      export default (<Suite name="gen"><Artifact name="a" path="a.json">{async ({ artifact }) => { await artifact.writeJson('a', { ok: true }, 'json reason'); }}</Artifact><Artifact name="b" path="b.txt">{async ({ artifact }) => { await artifact.writeText('b', 'hello', 'text reason'); }}</Artifact><Artifact name="c" path="c.bin">{async ({ artifact }) => { await artifact.writeBytes('c', new Uint8Array([1,2,3]), 'bytes reason'); }}</Artifact></Suite>);
    `);
    const run = await runNativeArtifacts({ rootDir: root, artifactRoot: out });
    expect(run.artifacts.map((a) => a.status)).toEqual(['passed', 'passed', 'passed']);
    expect(run.artifacts.every((a) => a.artifact?.outputPath.includes(out))).toBe(true);
    const report = createNativeArtifactReport(run);
    const text1 = formatNativeArtifactTextReport(report);
    const text2 = formatNativeArtifactTextReport(report);
    const json1 = formatNativeArtifactJsonReport(report);
    const json2 = formatNativeArtifactJsonReport(report);
    expect(text1).toBe(text2);
    expect(json1).toBe(json2);
    expect(nativeArtifactExitCode(report)).toBe(0);
  });

  it('handles required-not-written, unknown, duplicate, reason required, skip, assertion, expect', async () => {
    const root = makeDir();
    fs.writeFileSync(path.join(root, 'f.xtest.tsx'), `
      import { Suite, Artifact, skip, assert, expect } from '${path.resolve(process.cwd(), 'src/native-test/index.ts').replace(/\\/g, '/')}';
      export default (<Suite name="s"><Artifact name="required" path="r.txt">{() => {}}</Artifact><Artifact name="unknown" path="u.txt">{({ artifact }) => artifact.writeText('zzz', 'v', 'r')}</Artifact><Artifact name="dup" path="d.txt">{async ({ artifact }) => { await artifact.writeText('dup', 'a', 'r'); await artifact.writeText('dup', 'b', 'r'); }}</Artifact><Artifact name="reason" path="rr.txt">{({ artifact }) => artifact.writeText('reason', 'a', '')}</Artifact><Artifact name="skip" path="s.txt">{() => skip('later')}</Artifact><Artifact name="assert" path="a.txt">{() => assert.true(false, 'no')}</Artifact><Artifact name="expect" path="e.txt">{() => { expect(1).toBe(1); }}</Artifact></Suite>);
    `);
    const run = await runNativeArtifacts({ rootDir: root });
    const codeByName = new Map(run.artifacts.map((a) => [a.name, a.failure?.code ?? a.status]));
    expect(codeByName.get('required')).toBe('TSPACK_ARTIFACT_REQUIRED_NOT_WRITTEN');
    expect(codeByName.get('unknown')).toBe('TSPACK_ARTIFACT_UNKNOWN');
    expect(codeByName.get('dup')).toBe('TSPACK_ARTIFACT_ALREADY_WRITTEN');
    expect(codeByName.get('reason')).toBe('TSPACK_ARTIFACT_REASON_REQUIRED');
    expect(codeByName.get('skip')).toBe('skipped');
    expect(codeByName.get('assert')).toBe('TSPACK_ASSERT_FAILURE');
    expect(codeByName.get('expect')).toBe('TSPACK_EXPECT_BECAUSE_REQUIRED');
  });

  it('filtering and mode separation behave correctly', async () => {
    const root = makeDir();
    fs.writeFileSync(path.join(root, 'm.xtest.tsx'), `
      import fs from 'node:fs';
      import path from 'node:path';
      import { Suite, Artifact, Fact } from '${path.resolve(process.cwd(), 'src/native-test/index.ts').replace(/\\/g, '/')}';
      export default (<Suite name="m"><Artifact name="only-art" path="a.txt">{({ artifact }) => artifact.writeText('only-art', 'ok', 'artifact ran')}</Artifact><Fact name="only-fact">{() => { fs.writeFileSync(path.join('${root.replace(/\\/g, '/')}', 'fact.txt'), 'fact'); }}</Fact></Suite>);
    `);
    const filtered = await runNativeArtifacts({ rootDir: root, filter: 'only-art' });
    expect(filtered.artifacts).toHaveLength(1);
    const none = await runNativeArtifacts({ rootDir: root, filter: 'nope' });
    expect(none.diagnostics.some((d) => d.code === 'TSPACK_ARTIFACT_FILTER_NO_MATCH')).toBe(true);
    await runNativeArtifacts({ rootDir: root });
    expect(fs.existsSync(path.join(root, 'fact.txt'))).toBe(false);
    await runNativeTestFiles({ rootDir: root });
    expect(fs.existsSync(path.join(root, 'fact.txt'))).toBe(true);
  });
});
