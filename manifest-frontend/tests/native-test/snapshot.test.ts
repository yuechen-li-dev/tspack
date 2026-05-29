import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import { createNativeTestReport, formatNativeTestCompactTextReport, formatNativeTestTextReport, listNativeTests, runNativeTestFiles } from '../../src/native-test/index.js';

const importPath = path.resolve('src/native-test/index.ts').replace(/\\/g, '/');

function tempRoot(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'tspack-snapshot-test-'));
}

function writeXtest(root: string, relativePath: string, body: string): string {
  const filePath = path.join(root, relativePath);
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, body, 'utf8');
  return filePath;
}

function snapshotPath(root: string, name: string, extension: 'txt' | 'json'): string {
  return path.join(root, 'src', '__snapshots__', 'button.xtest.tsx', `${name}.snap.${extension}`);
}

describe('native xTest snapshots', () => {
  it('writes missing text snapshots in update mode under sibling __snapshots__ and counts as activity', async () => {
    const root = tempRoot();
    writeXtest(root, 'src/button.xtest.tsx', `
      import { Suite, Fact, expect } from '${importPath}';
      export default (<Suite name="snap"><Fact name="text">{() => {
        expect.snapshotText('btn primary', 'button-primary-class').because('button output should remain stable');
      }}</Fact></Suite>);
    `);

    const withoutUpdate = await runNativeTestFiles({ rootDir: root });
    expect(withoutUpdate.results[0].status).toBe('failed');
    expect((withoutUpdate.results[0].error as { code?: string }).code).toBe('TSPACK_SNAPSHOT_MISSING');
    expect(fs.existsSync(snapshotPath(root, 'button-primary-class', 'txt'))).toBe(false);

    const withUpdate = await runNativeTestFiles({ rootDir: root, updateSnapshots: true });
    expect(withUpdate.results[0].status).toBe('passed');
    expect(fs.readFileSync(snapshotPath(root, 'button-primary-class', 'txt'), 'utf8')).toBe('btn primary\n');
    expect(withUpdate.results[0].snapshots?.[0].path).toBe('src/__snapshots__/button.xtest.tsx/button-primary-class.snap.txt');
  });

  it('matches text snapshots, normalizes CRLF, and reports first differing line on mismatch', async () => {
    const root = tempRoot();
    writeXtest(root, 'src/button.xtest.tsx', `
      import { Suite, Fact, expect } from '${importPath}';
      export default (<Suite name="snap"><Fact name="text">{() => {
        expect.snapshotText('one\\r\\ntwo', 'lines').because('lines should remain stable');
      }}</Fact></Suite>);
    `);
    fs.mkdirSync(path.dirname(snapshotPath(root, 'lines', 'txt')), { recursive: true });
    fs.writeFileSync(snapshotPath(root, 'lines', 'txt'), 'one\r\ntwo\n', 'utf8');

    const matching = await runNativeTestFiles({ rootDir: root });
    expect(matching.results[0].status).toBe('passed');

    fs.writeFileSync(snapshotPath(root, 'lines', 'txt'), 'one\nthree\n', 'utf8');
    const mismatched = await runNativeTestFiles({ rootDir: root });
    const report = createNativeTestReport(mismatched);
    const text = formatNativeTestTextReport(report);
    expect(text).toContain('TSPACK_SNAPSHOT_MISMATCH');
    expect(text).toContain('firstDifferenceLine: 2');
    expect(text).toContain('expectedLine: "three"');
    expect(text).toContain('actualLine: "two"');
  });

  it('updates mismatched snapshots and reports update activity', async () => {
    const root = tempRoot();
    writeXtest(root, 'src/button.xtest.tsx', `
      import { Suite, Fact, expect } from '${importPath}';
      export default (<Suite name="snap"><Fact name="text">{() => {
        expect.snapshotText('new value', 'value').because('value should remain stable');
      }}</Fact></Suite>);
    `);
    fs.mkdirSync(path.dirname(snapshotPath(root, 'value', 'txt')), { recursive: true });
    fs.writeFileSync(snapshotPath(root, 'value', 'txt'), 'old value\n', 'utf8');

    const result = await runNativeTestFiles({ rootDir: root, updateSnapshots: true });
    const report = createNativeTestReport(result);
    const text = formatNativeTestTextReport(report);
    expect(result.results[0].status).toBe('passed');
    expect(fs.readFileSync(snapshotPath(root, 'value', 'txt'), 'utf8')).toBe('new value\n');
    expect(text).toContain('UPDATED src/__snapshots__/button.xtest.tsx/value.snap.txt');
    expect(text).toContain('snapshots updated: 1');
  });

  it('writes deterministic JSON and rejects unsupported JSON values', async () => {
    const root = tempRoot();
    writeXtest(root, 'src/button.xtest.tsx', `
      import { Suite, Fact, expect } from '${importPath}';
      export default (<Suite name="snap">
        <Fact name="json">{() => {
          expect.snapshotJson({ z: 1, a: { b: true, a: null } }, 'data').because('json should remain stable');
        }}</Fact>
        <Fact name="bad">{() => {
          expect.snapshotJson({ bad: undefined }, 'bad').because('unsupported values fail');
        }}</Fact>
      </Suite>);
    `);

    const updated = await runNativeTestFiles({ rootDir: root, filter: 'json', updateSnapshots: true });
    expect(updated.results).toHaveLength(1);
    expect(updated.results[0].status).toBe('passed');
    expect(fs.readFileSync(snapshotPath(root, 'data', 'json'), 'utf8')).toBe(`{\n  "a": {\n    "a": null,\n    "b": true\n  },\n  "z": 1\n}\n`);

    const bad = await runNativeTestFiles({ rootDir: root, filter: 'bad', updateSnapshots: true });
    expect(bad.results[0].status).toBe('failed');
    expect((bad.results[0].error as { code?: string }).code).toBe('TSPACK_SNAPSHOT_JSON_UNSUPPORTED');
  });

  it('rejects invalid names and text non-strings', async () => {
    const root = tempRoot();
    writeXtest(root, 'src/button.xtest.tsx', `
      import { Suite, Fact, expect } from '${importPath}';
      export default (<Suite name="snap">
        <Fact name="bad-name">{() => { expect.snapshotText('x', '../bad').because('names must be safe'); }}</Fact>
        <Fact name="bad-value">{() => { expect.snapshotText(1, 'value').because('text must be string'); }}</Fact>
      </Suite>);
    `);

    const badName = await runNativeTestFiles({ rootDir: root, filter: 'bad-name', updateSnapshots: true });
    expect((badName.results[0].error as { code?: string }).code).toBe('TSPACK_SNAPSHOT_INVALID_NAME');

    const badValue = await runNativeTestFiles({ rootDir: root, filter: 'bad-value', updateSnapshots: true });
    expect((badValue.results[0].error as { code?: string }).code).toBe('TSPACK_SNAPSHOT_TEXT_VALUE_INVALID');
  });

  it('list mode does not write snapshots and filter updates only selected tests', async () => {
    const root = tempRoot();
    writeXtest(root, 'src/button.xtest.tsx', `
      import { Suite, Fact, expect } from '${importPath}';
      export default (<Suite name="snap">
        <Fact name="one">{() => { expect.snapshotText('one', 'one').because('one stable'); }}</Fact>
        <Fact name="two">{() => { expect.snapshotText('two', 'two').because('two stable'); }}</Fact>
      </Suite>);
    `);

    const listed = await listNativeTests({ rootDir: root });
    expect(listed.tests.map((test) => test.id)).toContain('src/button.xtest.tsx::snap/one');
    expect(fs.existsSync(path.join(root, 'src', '__snapshots__'))).toBe(false);

    const updated = await runNativeTestFiles({ rootDir: root, filter: 'one', updateSnapshots: true });
    expect(updated.results).toHaveLength(1);
    expect(fs.existsSync(snapshotPath(root, 'one', 'txt'))).toBe(true);
    expect(fs.existsSync(snapshotPath(root, 'two', 'txt'))).toBe(false);
  });

  it('compact output shows snapshot failures and hides passing snapshot tests', async () => {
    const root = tempRoot();
    writeXtest(root, 'src/button.xtest.tsx', `
      import { Suite, Fact, expect } from '${importPath}';
      export default (<Suite name="snap">
        <Fact name="pass">{() => { expect.snapshotText('pass', 'pass').because('pass stable'); }}</Fact>
        <Fact name="fail">{() => { expect.snapshotText('actual', 'fail').because('fail stable'); }}</Fact>
      </Suite>);
    `);
    fs.mkdirSync(path.dirname(snapshotPath(root, 'pass', 'txt')), { recursive: true });
    fs.writeFileSync(snapshotPath(root, 'pass', 'txt'), 'pass\n', 'utf8');
    fs.writeFileSync(snapshotPath(root, 'fail', 'txt'), 'expected\n', 'utf8');

    const report = createNativeTestReport(await runNativeTestFiles({ rootDir: root }));
    const compact = formatNativeTestCompactTextReport(report);
    expect(compact).toContain('FAIL src/button.xtest.tsx::snap/fail');
    expect(compact).toContain('TSPACK_SNAPSHOT_MISMATCH');
    expect(compact).not.toContain('src/button.xtest.tsx::snap/pass');
  });

  it('Project-backed tests anchor snapshots to the test file directory', async () => {
    const root = tempRoot();
    const fixture = path.join(root, 'fixture');
    fs.mkdirSync(fixture, { recursive: true });
    fs.writeFileSync(path.join(fixture, 'value.txt'), 'sandbox', 'utf8');
    writeXtest(root, 'src/button.xtest.tsx', `
      import { Suite, Fact, Project, expect } from '${importPath}';
      export default (<Suite name="snap"><Fact name="project"><Project from="${fixture.replace(/\\/g, '/')}" />{async ({ project }) => {
        expect.snapshotText(await project.readText('value.txt'), 'project-value').because('project output stable');
      }}</Fact></Suite>);
    `);

    const result = await runNativeTestFiles({ rootDir: root, updateSnapshots: true });
    expect(result.results[0].status).toBe('passed');
    expect(fs.existsSync(snapshotPath(root, 'project-value', 'txt'))).toBe(true);
    expect(fs.existsSync(path.join(fixture, '__snapshots__'))).toBe(false);
  });
});
