import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import { discoverNativeTestFile } from '../../src/native-test/discover';

function withFile(source: string): string {
  const file = path.join(os.tmpdir(), `native-${Math.random().toString(16).slice(2)}.tspack.test.tsx`);
  fs.writeFileSync(file, source);
  return file;
}

describe('native test discovery', () => {
  it('discovers suite fact and theory cases deterministically', () => {
    const file = withFile(`export default (<Suite name="math"><Fact name="add">{() => {}}</Fact><Theory name="len"><Case input="a" expected={1} /><Case input="ab" expected={2} />{({ input, expected }) => {}}</Theory></Suite>);`);
    const one = discoverNativeTestFile(file);
    const two = discoverNativeTestFile(file);
    expect(one.tests).toEqual(['math/add', 'math/len[0]', 'math/len[1]']);
    expect(one.tests).toEqual(two.tests);
    expect(one.diagnostics).toEqual([]);
  });

  it('rejects unknown element, missing name, spread, dynamic and missing body', () => {
    const file = withFile(`const n = 'x'; export default (<Suite name="s"><Blah /><Fact>{() => {}}</Fact><Fact {...x} name={n}>{() => {}}</Fact><Fact name="ok"></Fact></Suite>);`);
    const result = discoverNativeTestFile(file);
    expect(result.diagnostics.map((d) => d.code)).toContain('TSPACK_TEST_UNKNOWN_ELEMENT');
    expect(result.diagnostics.map((d) => d.code)).toContain('TSPACK_TEST_INVALID_NAME');
    expect(result.diagnostics.map((d) => d.code)).toContain('TSPACK_TEST_FORBIDDEN_SPREAD');
    expect(result.diagnostics.map((d) => d.code)).toContain('TSPACK_TEST_MISSING_BODY');
  });

  it('does not execute callback body during discovery', () => {
    const file = withFile(`export default (<Suite name="safe"><Fact name="boom">{() => { throw new Error('should not run'); }}</Fact></Suite>);`);
    const result = discoverNativeTestFile(file);
    expect(result.tests).toEqual(['safe/boom']);
  });
});
