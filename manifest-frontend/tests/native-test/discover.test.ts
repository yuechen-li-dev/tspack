import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import { discoverNativeTestFile } from '../../src/native-test/discover';

function withFile(source: string): string {
  const file = path.join(os.tmpdir(), `native-${Math.random().toString(16).slice(2)}.xtest.tsx`);
  fs.writeFileSync(file, source);
  return file;
}

describe('native test discovery', () => {
  it('discovers artifact metadata for fact/theory', () => {
    const file = withFile(`export default (<Suite name="s"><Fact name="f"><Artifact name="report" path="report.json" format="json" />{() => {}}</Fact><Theory name="t"><Artifact name="case" path="case.txt" optional={true}/><Case a={1}/>{() => {}}</Theory></Suite>);`);
    const result = discoverNativeTestFile(file);
    expect(result.facts[0].artifacts[0]).toEqual({ name: 'report', path: 'report.json', format: 'json', required: true });
    expect(result.theories[0].artifacts[0]).toEqual({ name: 'case', path: 'case.txt', required: false });
  });

  it('diagnoses invalid artifact declarations and unsafe paths', () => {
    const file = withFile(`export default (<Suite name="s"><Artifact name="x" path="x.txt"/><Fact name="f"><Artifact name="dup" path="one.txt"/><Artifact name="dup" path="two.txt"/><Artifact name="x" path="../bad"/>{() => {}}</Fact></Suite>);`);
    const result = discoverNativeTestFile(file);
    const codes = result.diagnostics.map((d) => d.code);
    expect(codes).toContain('TSPACK_ARTIFACT_INVALID_DECLARATION');
    expect(codes).toContain('TSPACK_ARTIFACT_INVALID_PATH');
    expect(codes).toContain('TSPACK_ARTIFACT_DUPLICATE_NAME');
  });
});
