import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import { createNativeTestReport, listNativeArtifacts, listNativeTests, runNativeArtifacts, runNativeBenchmarks, runNativeProphecies, runNativeTestFiles } from '../../src/native-test';

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
          <Valid name="noError alias">{() => { expect.noError({ diagnostics: [] }).because('singular alias works in valid'); }}</Valid>
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
          <Invalid name="preflight noError">{() => { expect.noError([{ code: 'W1', severity: 'warning' }]).because('preflight can still be clean'); }}</Invalid>
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
    expect(statusById.get('fixtures.valid.tsx::v/valid/noError alias')).toBe('passed');
    expect(statusById.get('fixtures.valid.tsx::v/valid/assert fail')).toBe('failed');
    expect(statusById.get('fixtures.valid.tsx::v/valid/skip')).toBe('skipped');
    expect(statusById.get('fixtures.valid.tsx::v/valid/async')).toBe('passed');
    expect(statusById.get('fixtures.valid.tsx::v/valid/pending because')).toBe('failed');

    expect(statusById.get('fixtures.invalid.tsx::i/invalid/error match')).toBe('passed');
    expect(statusById.get('fixtures.invalid.tsx::i/invalid/preflight noError')).toBe('passed');
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

it('loaded xtest/valid/invalid/artifact files can use Project context and filter avoids import side effects', async () => {
  const root = makeDir();
  const importPath = nativeImportPath();
  const fixture = path.join(root, 'fixture');
  const fixturePath = fixture.split(path.sep).join('/');
  fs.mkdirSync(fixture);
  fs.writeFileSync(path.join(fixture, 'manifest.json'), '{"code":"E_MATCH"}');

  fs.writeFileSync(path.join(root, 'with-project.xtest.tsx'), `
    import { Suite, Fact, Theory, Case, Project, assert, expect } from '${importPath}';
    export default (<Suite name="x">
      <Fact name="f"><Project from="${fixturePath}" />{async ({ project }) => { await project.writeText('gen.txt', 'ok', 'write'); expect.noError([]).because('fact can use noError'); assert.equal(await project.readText('gen.txt'), 'ok', 'read'); }}</Fact>
      <Fact name="lgtm-only">{() => { assert.LGTM({ diagnostics: [] }, 'lgtm-only fact is meaningful'); }}</Fact>
      <Theory name="theory-lgtm"><Case name="one" /><Case name="two" />{() => { assert.LGTM([{ code: 'W1', severity: 'warning' }], 'theory case can use LGTM'); }}</Theory>
    </Suite>);
  `);
  fs.writeFileSync(path.join(root, 'with-project.valid.tsx'), `
    import { Suite, Valid, Project, expect } from '${importPath}';
    export default (<Suite name="v"><Valid name="ok"><Project from="${fixturePath}" />{async ({ project }) => { const x = await project.readJson('manifest.json'); expect.noErrors([]).because(String(x.code)); }}</Valid></Suite>);
  `);
  fs.writeFileSync(path.join(root, 'with-project.invalid.tsx'), `
    import { Suite, Invalid, Project, expect } from '${importPath}';
    export default (<Suite name="i"><Invalid name="bad"><Project from="${fixturePath}" />{async ({ project }) => { const x = await project.readJson('manifest.json'); expect.error([{ code: x.code }], 'E_MATCH').because('match'); }}</Invalid></Suite>);
  `);
  fs.writeFileSync(path.join(root, 'with-project-artifact.xtest.tsx'), `
    import { Suite, Artifact, Project } from '${importPath}';
    export default (<Suite name="a"><Artifact name="one" path="a.txt"><Project from="${fixturePath}" />{async ({ artifact, project }) => { await project.writeText('local.txt', 'x', 'write'); await artifact.writeText('one', 'ok', 'artifact'); }}</Artifact></Suite>);
  `);

  fs.writeFileSync(path.join(root, 'not-selected.xtest.tsx'), `
    import fs from 'node:fs';
    import path from 'node:path';
    import { Suite, Fact } from '${importPath}';
    fs.writeFileSync(path.join('${root.split(path.sep).join('/')}', 'imported.txt'), 'yes');
    export default (<Suite name="n"><Fact name="f">{() => {}}</Fact></Suite>);
  `);

  const run = await runNativeTestFiles({ rootDir: root, filter: 'with-project' });
  expect(run.results.every((r) => r.status === 'passed')).toBe(true);
  expect(fs.existsSync(path.join(root, 'imported.txt'))).toBe(false);

  const artifacts = await runNativeArtifacts({ rootDir: root, filter: 'one' });
  expect(artifacts.artifacts[0].status).toBe('passed');
});

it('loaded command helpers and context exclusion matrix', async () => {
  const root = makeDir();
  const importPath = nativeImportPath();
  const fixture = path.join(root, 'fixture');
  fs.mkdirSync(path.join(fixture, 'sub'), { recursive: true });
  fs.writeFileSync(path.join(root, 'm.xtest.tsx'), `
    import { Suite, Fact, Theory, Case, Valid, Invalid, Artifact, Project, assert } from '${importPath}';
    export default (<Suite name="m">
      <Fact name="with"><Project from="${fixture.replace(/\\/g, '/')}" />{async ({ command }) => { const r = await command.run(['node','-e','process.exit(3)'], 'fact'); assert.exitCode(r, 3, 'fact code'); }}</Fact>
      <Fact name="without">{({ command }) => { assert.true(command === undefined, 'no project no command fact'); }}</Fact>
      <Theory name="with"><Project from="${fixture.replace(/\\/g, '/')}" /><Case n={1} />{async (_d, { command }) => { const r = await command.run(['node','-e','console.log(1)'], 'theory'); assert.exitCode(r, 0, 'theory code'); }}</Theory>
      <Theory name="without"><Case n={1} />{(_d, { command }) => { assert.true(command === undefined, 'no project no command theory'); }}</Theory>
      <Valid name="with"><Project from="${fixture.replace(/\\/g, '/')}" />{async ({ command }) => { const r = await command.run(['node','-e','process.exit(0)'], 'valid'); assert.exitCode(r, 0, 'valid code'); }}</Valid>
      <Valid name="without">{({ command }) => { assert.true(command === undefined, 'no project no command valid'); }}</Valid>
      <Invalid name="with"><Project from="${fixture.replace(/\\/g, '/')}" />{async ({ command }) => { const r = await command.run(['node','-e','process.exit(0)'], 'invalid'); assert.exitCode(r, 0, 'invalid code'); }}</Invalid>
      <Invalid name="without">{({ command }) => { assert.true(command === undefined, 'no project no command invalid'); }}</Invalid>
      <Artifact name="a" path="a.txt"><Project from="${fixture.replace(/\\/g, '/')}" />{async ({ artifact, command }) => { const r = await command.run(['node','-e','console.log(2)'], 'artifact'); assert.exitCode(r, 0, 'artifact code'); await artifact.writeText('a', 'ok', 'artifact'); }}</Artifact>
    </Suite>);
  `);
  fs.writeFileSync(path.join(root, 'm.benchmark.tsx'), `
    import { Suite, Benchmark, Project } from '${importPath}';
    export default (<Suite name="m"><Benchmark name="b"><Project from="${fixture.replace(/\\/g, '/')}" />{({ command, bench }) => { bench.check(() => { if (command !== undefined) throw new Error('command must be undefined'); }); bench.measure(() => {}); }}</Benchmark></Suite>);
  `);
  fs.writeFileSync(path.join(root, 'p.prophecy.tsx'), `
    import { Suite, Prophecy, Foretell } from '${importPath}';
    export default (<Suite name="p"><Prophecy name="x"><Foretell reason="r" />{(ctx = {}) => { if (ctx.command !== undefined) { throw new Error('command should be undefined'); } process.exit(7); }}</Prophecy></Suite>);
  `);
  const run = await runNativeTestFiles({ rootDir: root });
  expect(run.results.every((r) => r.status === 'passed')).toBe(true);
  const art = await runNativeArtifacts({ rootDir: root });
  expect(art.artifacts[0].status).toBe('passed');
  const bench = await runNativeBenchmarks({ rootDir: root });
  expect(bench.benchmarks[0].status).toBe('passed');
  const doom = await runNativeProphecies({ rootDir: root });
  expect(doom.prophecies).toHaveLength(1);
});
