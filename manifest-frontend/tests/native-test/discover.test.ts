import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import { discoverNativeTestFile, discoverNativeTestFiles } from '../../src/native-test/discover';

function makeDir(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'native-discover-'));
}

function write(root: string, rel: string, source: string): string {
  const file = path.join(root, rel);
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, source);
  return file;
}

describe('native test discovery', () => {
  it('discovers valid/invalid and ignores conventional test/spec files', () => {
    const root = makeDir();
    write(root, 'a.valid.tsx', 'export default (<Suite name="s"><Valid name="ok">{() => {}}</Valid></Suite>);');
    write(root, 'b.invalid.tsx', 'export default (<Suite name="s"><Invalid name="bad">{() => {}}</Invalid></Suite>);');
    write(root, 'c.test.tsx', 'export default null;');
    write(root, 'd.spec.tsx', 'export default null;');

    const result = discoverNativeTestFiles({ rootDir: root });
    expect(result.files.map((f) => path.basename(f.filePath))).toEqual(['a.valid.tsx', 'b.invalid.tsx']);
    expect(result.files.flatMap((f) => f.tests.map((t) => t.id))).toEqual([
      'a.valid.tsx::s/valid/ok',
      'b.invalid.tsx::s/invalid/bad',
    ]);
  });

  it('enforces file-kind element restrictions and required name/body', () => {
    const root = makeDir();
    const validWithInvalid = write(root, 'vi.valid.tsx', 'export default (<Suite name="s"><Invalid name="bad">{() => {}}</Invalid></Suite>);');
    const invalidWithValid = write(root, 'iv.invalid.tsx', 'export default (<Suite name="s"><Valid name="bad">{() => {}}</Valid></Suite>);');
    const xtestWithValid = write(root, 'x.xtest.tsx', 'export default (<Suite name="s"><Valid name="bad">{() => {}}</Valid></Suite>);');
    const validWithFact = write(root, 'vf.valid.tsx', 'export default (<Suite name="s"><Fact name="bad">{() => {}}</Fact></Suite>);');
    const missingValidBody = write(root, 'vb.valid.tsx', 'export default (<Suite name="s"><Valid name="n" /></Suite>);');
    const missingInvalidBody = write(root, 'ib.invalid.tsx', 'export default (<Suite name="s"><Invalid name="n" /></Suite>);');
    const missingNames = write(root, 'mn.valid.tsx', 'export default (<Suite name="s"><Valid>{() => {}}</Valid></Suite>);');

    const codes = [
      ...discoverNativeTestFile(validWithInvalid).diagnostics.map((d) => d.code),
      ...discoverNativeTestFile(invalidWithValid).diagnostics.map((d) => d.code),
      ...discoverNativeTestFile(xtestWithValid).diagnostics.map((d) => d.code),
      ...discoverNativeTestFile(validWithFact).diagnostics.map((d) => d.code),
      ...discoverNativeTestFile(missingValidBody).diagnostics.map((d) => d.code),
      ...discoverNativeTestFile(missingInvalidBody).diagnostics.map((d) => d.code),
      ...discoverNativeTestFile(missingNames).diagnostics.map((d) => d.code),
    ];

    expect(codes.filter((code) => code === 'TSPACK_TEST_INVALID_FILE_KIND_ELEMENT').length).toBeGreaterThanOrEqual(4);
    expect(codes).toContain('TSPACK_TEST_MISSING_BODY');
    expect(codes).toContain('TSPACK_TEST_INVALID_NAME');
  });

  it('discovery does not execute valid/invalid callback bodies', () => {
    const root = makeDir();
    const marker = path.join(root, 'marker.txt');
    write(root, 'n.valid.tsx', `
      import fs from 'node:fs';
      export default (<Suite name="s"><Valid name="x">{() => { fs.writeFileSync('${marker.split(path.sep).join('/')}', 'ran'); }}</Valid></Suite>);
    `);
    discoverNativeTestFiles({ rootDir: root });
    expect(fs.existsSync(marker)).toBe(false);
  });
});

it('discovers Project metadata across executable units and validates declarations', () => {
  const root = makeDir();
  const fixture = path.join(root, 'fixture');
  fs.mkdirSync(fixture);
  const file = write(root, 'm.xtest.tsx', `
    export default (
      <Suite name="s">
        <Fact name="f"><Project from="fixture" name="pf" keepOnFailure={true} />{() => {}}</Fact>
        <Theory name="t"><Project from="fixture" keepOnFailure={false} /><Case n={1} />{() => {}}</Theory>
        <Artifact name="a" path="a.txt"><Project from="fixture" />{() => {}}</Artifact>
      </Suite>
    );
  `);
  const validFile = write(root, 'm.valid.tsx', 'export default (<Suite name="s"><Valid name="v"><Project from="fixture" />{() => {}}</Valid></Suite>);');
  const invalidFile = write(root, 'm.invalid.tsx', 'export default (<Suite name="s"><Invalid name="i"><Project from="fixture" />{() => {}}</Invalid></Suite>);');

  const xt = discoverNativeTestFile(file);
  expect(xt.facts[0].project).toEqual({ from: 'fixture', name: 'pf', keepOnFailure: true });
  expect(xt.theories[0].project).toEqual({ from: 'fixture', keepOnFailure: false, name: undefined });
  expect(xt.standaloneArtifacts[0].project).toEqual({ from: 'fixture', keepOnFailure: false, name: undefined });
  expect(discoverNativeTestFile(validFile).invariants[0].project?.from).toBe('fixture');
  expect(discoverNativeTestFile(invalidFile).invariants[0].project?.from).toBe('fixture');

  const bad = write(root, 'bad.xtest.tsx', `
    const dyn = 'fixture'; const dynName = 'n'; const keep = true;
    export default (<Suite name="s">
      <Project from="fixture" />
      <Fact name="f"><Project from={dyn} name={dynName} keepOnFailure={keep} /><Project from="fixture" />{() => {}}</Fact>
    </Suite>);
  `);
  const badDiags = discoverNativeTestFile(bad).diagnostics.map((d) => d.code);
  expect(badDiags).toContain('TSPACK_PROJECT_FIXTURE_DUPLICATE');
});

it('discovery does not copy fixture directory or create sandbox temp paths', () => {
  const root = makeDir();
  const fixture = path.join(root, 'fixture');
  const nested = path.join(fixture, 'nested');
  fs.mkdirSync(nested, { recursive: true });
  fs.writeFileSync(path.join(nested, 'in.txt'), 'x');
  write(root, 'no-run.xtest.tsx', 'export default (<Suite name="s"><Fact name="f"><Project from="fixture" />{() => {}}</Fact></Suite>);');
  const before = new Set(fs.readdirSync(os.tmpdir()));
  discoverNativeTestFiles({ rootDir: root });
  const after = new Set(fs.readdirSync(os.tmpdir()));
  expect(fs.existsSync(path.join(root, 'generated.txt'))).toBe(false);
  expect([...after].filter((n) => !before.has(n) && n.includes('tspack-project-')).length).toBe(0);
});

it('reports invalid Project from path', () => {
  const root = makeDir();
  write(root, 'bad-path.xtest.tsx', 'export default (<Suite name="s"><Fact name="f"><Project from="../escape" />{() => {}}</Fact></Suite>);');
  const codes = discoverNativeTestFiles({ rootDir: root }).diagnostics.map((d) => d.code);
  expect(codes).toContain('TSPACK_PROJECT_FIXTURE_INVALID_PATH');
});
