import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import { assert, createNativeDoomReport, discoverNativeTestFile, formatNativeDoomJsonReport, formatNativeDoomTextReport, listNativeProphecies, nativeDoomExitCode, runNativeBenchmarks, runNativeProphecies, runNativeTestFiles } from '../../src/native-test';

function makeDir(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'tspack-doom-'));
}

function nativeImportPath(): string {
  return path.resolve(process.cwd(), 'src/native-test/index.ts').split(path.sep).join('/');
}
function installDoomBridgeStub(): () => void {
  const bridgePath = path.resolve(process.cwd(), 'dist/src/native-test-cli.js');
  fs.mkdirSync(path.dirname(bridgePath), { recursive: true });
  const script = `#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
const args = process.argv.slice(2);
if (args[0] !== 'doom-child') process.exit(2);
const id = args[args.indexOf('--id') + 1];
const out = args[args.indexOf('--out') + 1];
const name = id.split('/').at(-1) ?? '';
const reasonMap = { exit7: 'exit', throw: 'throw', return: 'return', hang: 'hang', a: 'r', b: 'r2', missingEnvelope: 'm', badEnvelope: 'b', mismatchEnvelope: 'good' };
fs.mkdirSync(out, { recursive: true });
if (name === 'badEnvelope') {
  fs.writeFileSync(path.join(out, 'envelope.json'), '{bad-json');
} else if (name !== 'missingEnvelope') {
  const reason = name === 'mismatchEnvelope' ? 'wrong' : (reasonMap[name] ?? 'r');
  fs.writeFileSync(path.join(out, 'envelope.json'), JSON.stringify({ prophecyId: id, suiteName: 'doom', name, foretell: { reason }, phase: 'before-doom' }, null, 2));
}
if (name === 'exit7' || name === 'a' || name === 'b' || name === 'missingEnvelope' || name === 'mismatchEnvelope' || name === 'badEnvelope') { console.log('o1'); console.error('e1'); process.exit(7); }
else if (name === 'throw') { throw new Error('boom'); }
else if (name === 'hang') { const lock = new Int32Array(new SharedArrayBuffer(4)); Atomics.wait(lock, 0, 0, 60_000); }
else { process.exit(0); }
`;
  fs.writeFileSync(bridgePath, script, 'utf8');
  return () => {
    if (fs.existsSync(bridgePath)) {
      fs.unlinkSync(bridgePath);
    }
  };
}

describe('doom', () => {
  it('discovers prophecy files and validates declarations', () => {
    const root = makeDir();
    const importPath = nativeImportPath();
    fs.writeFileSync(path.join(root, 'a.prophecy.tsx'), `import { Suite, Prophecy, Foretell, CycleTime } from '${importPath}'; export default (<Suite name="s"><Prophecy name="p"><Foretell reason="r" /><CycleTime seconds={2} />{() => {}}</Prophecy></Suite>);`);
    const found = discoverNativeTestFile(path.join(root, 'a.prophecy.tsx'));
    expect(found.prophecies[0].id).toBe('a.prophecy.tsx::s/prophecy/p');
    expect(found.prophecies[0].cycleTimeSeconds).toBe(2);

    fs.writeFileSync(path.join(root, 'bad.prophecy.tsx'), `import { Suite, Prophecy, Foretell } from '${importPath}'; export default (<Suite name="s"><Prophecy><Foretell reason="" />{() => {}}</Prophecy><Prophecy name="x">{() => {}}</Prophecy></Suite>);`);
    const bad = discoverNativeTestFile(path.join(root, 'bad.prophecy.tsx'));
    expect(bad.diagnostics.some((d) => d.code === 'TSPACK_TEST_INVALID_NAME')).toBe(true);
    expect(bad.diagnostics.some((d) => d.code === 'TSPACK_DOOM_INVALID_FORETELL')).toBe(true);
    expect(bad.diagnostics.some((d) => d.code === 'TSPACK_DOOM_MISSING_FORETELL')).toBe(true);

    fs.writeFileSync(path.join(root, 'kinds.prophecy.tsx'), `import { Suite, Prophecy, Foretell, Fact, Theory, Valid, Invalid, Benchmark, Artifact, CycleTime } from '${importPath}'; export default (<Suite name="s"><Fact name="f">{() => {}}</Fact><Theory name="t">{() => {}}</Theory><Valid name="v">{() => {}}</Valid><Invalid name="i">{() => {}}</Invalid><Benchmark name="b">{() => {}}</Benchmark><Artifact name="a" path="a.txt">{() => {}}</Artifact><Foretell reason="x" /><Prophecy name="p"><Foretell reason="r" /><Foretell reason="r2" />{() => {}}</Prophecy><Prophecy name="dupCycle"><Foretell reason="r" /><CycleTime seconds={1} /><CycleTime seconds={2} />{() => {}}</Prophecy><Prophecy name="q"><Foretell reason="r" /><CycleTime seconds={0} />{() => {}}</Prophecy></Suite>);`);
    const kinds = discoverNativeTestFile(path.join(root, 'kinds.prophecy.tsx'));
    expect(kinds.diagnostics.filter((d) => d.code === 'TSPACK_TEST_INVALID_FILE_KIND_ELEMENT').length).toBeGreaterThanOrEqual(7);
    expect(kinds.diagnostics.some((d) => d.code === 'TSPACK_DOOM_DUPLICATE_FORETELL')).toBe(true);
    expect(kinds.diagnostics.some((d) => d.code === 'TSPACK_TEST_DUPLICATE_CYCLETIME')).toBe(true);
    expect(kinds.diagnostics.some((d) => d.code === 'TSPACK_TEST_INVALID_CYCLETIME')).toBe(true);
  });

  it('lists without executing body and filters isolation from test/bench', async () => {
    const root = makeDir();
    const importPath = nativeImportPath();
    fs.writeFileSync(path.join(root, 'x.prophecy.tsx'), `import fs from 'node:fs'; import path from 'node:path'; import { Suite, Prophecy, Foretell } from '${importPath}'; fs.writeFileSync(path.join('${root.replaceAll('\\', '/')}', 'imported.txt'),'yes'); export default (<Suite name="d"><Prophecy name="a"><Foretell reason="r" />{() => { fs.writeFileSync(path.join('${root.replaceAll('\\', '/')}', 'ran.txt'),'yes'); process.exit(7); }}</Prophecy></Suite>);`);
    const listed = listNativeProphecies({ rootDir: root });
    expect(listed.prophecies).toHaveLength(1);
    expect(fs.existsSync(path.join(root, 'ran.txt'))).toBe(false);

    const testRun = await runNativeTestFiles({ rootDir: root });
    expect(testRun.results).toHaveLength(0);
    const benchRun = await runNativeBenchmarks({ rootDir: root });
    expect(benchRun.benchmarks).toHaveLength(0);

    fs.writeFileSync(path.join(root, 'a.test.tsx'), 'export default 1;');
    fs.writeFileSync(path.join(root, 'b.spec.tsx'), 'export default 1;');
    fs.writeFileSync(path.join(root, 'y.xtest.tsx'), `import { Suite, Prophecy, Fact } from '${importPath}'; export default (<Suite name="x"><Fact name="f">{() => {}}</Fact><Prophecy name="p">{() => {}}</Prophecy></Suite>);`);
    const listed2 = listNativeProphecies({ rootDir: root });
    expect(listed2.prophecies.every((p) => p.filePath.endsWith('.prophecy.tsx'))).toBe(true);
    const xtest = discoverNativeTestFile(path.join(root, 'y.xtest.tsx'));
    expect(xtest.diagnostics.some((d) => d.code === 'TSPACK_TEST_INVALID_FILE_KIND_ELEMENT')).toBe(true);
  });

  it('runs doom cases including timeout and reports', async () => {
    const cleanupBridge = installDoomBridgeStub();
    const root = makeDir();
    const out = path.join(root, 'out');
    const importPath = nativeImportPath();
    fs.writeFileSync(path.join(root, 'doom.prophecy.tsx'), `import fs from 'node:fs'; import path from 'node:path'; import { Suite, Prophecy, Foretell, CycleTime } from '${importPath}';
      export default (<Suite name="doom">
        <Prophecy name="exit7"><Foretell reason="exit" />{() => { console.log('o1'); console.error('e1'); process.exit(7); }}</Prophecy>
        <Prophecy name="throw"><Foretell reason="throw" />{() => { throw new Error('boom'); }}</Prophecy>
        <Prophecy name="return"><Foretell reason="return" />{() => { return; }}</Prophecy>
        <Prophecy name="hang"><Foretell reason="hang" /><CycleTime seconds={1} />{async () => { await new Promise(() => {}); }}</Prophecy>
      </Suite>);`);

    const run = await runNativeProphecies({ rootDir: root, outDir: out });
    const byId = new Map(run.prophecies.map((p) => [p.id, p]));
    expect(byId.get('doom.prophecy.tsx::doom/prophecy/exit7')?.status).toBe('passed');
    expect(byId.get('doom.prophecy.tsx::doom/prophecy/throw')?.status).toBe('passed');
    expect(byId.get('doom.prophecy.tsx::doom/prophecy/return')?.failure?.code).toBe('TSPACK_DOOM_DID_NOT_TERMINATE');
    expect(byId.get('doom.prophecy.tsx::doom/prophecy/hang')?.failure?.code).toBe('TSPACK_DOOM_TIMEOUT');

    const pass = byId.get('doom.prophecy.tsx::doom/prophecy/exit7');
    expect(pass?.stdout?.includes('o1')).toBe(true);
    expect(pass?.stderr?.includes('e1')).toBe(true);
    expect(pass?.envelopePath?.startsWith(path.resolve(out))).toBe(true);

    const report = createNativeDoomReport(run);
    const text = formatNativeDoomTextReport(report);
    const json = formatNativeDoomJsonReport(report);
    expect(text.includes('PASS')).toBe(true);
    expect(text.includes('FAIL')).toBe(true);
    expect(text.includes('envelope:')).toBe(true);
    expect(text.includes('TSPACK_DOOM_DID_NOT_TERMINATE')).toBe(true);
    expect(text.includes('TSPACK_DOOM_TIMEOUT')).toBe(true);
    expect(JSON.parse(json).summary.total).toBe(4);
    expect(nativeDoomExitCode(report)).toBe(1);
    cleanupBridge();
  });

  it('filter and no-match behavior and assert.doom', async () => {
    const cleanupBridge = installDoomBridgeStub();
    const root = makeDir();
    const importPath = nativeImportPath();
    fs.writeFileSync(path.join(root, 'f.prophecy.tsx'), `import { Suite, Prophecy, Foretell } from '${importPath}'; export default (<Suite name="s"><Prophecy name="a"><Foretell reason="r" />{() => { process.exit(7); }}</Prophecy><Prophecy name="b"><Foretell reason="r2" />{() => { process.exit(7); }}</Prophecy></Suite>);`);
    const selected = await runNativeProphecies({ rootDir: root, filter: 'a' });
    expect(selected.prophecies).toHaveLength(1);
    const nomatch = await runNativeProphecies({ rootDir: root, filter: 'zzz' });
    expect(nomatch.diagnostics.some((d) => d.code === 'TSPACK_DOOM_FILTER_NO_MATCH')).toBe(true);

    assert.doom(selected.prophecies[0], { reason: 'r' }, 'doom assertion passes');
    expect(() => assert.doom({ id: 'x', name: 'x', status: 'failed' }, {}, 'must fail')).toThrowError(/TSPACK_ASSERT_DOOM_FAILED|doom failed/);
    expect(() => assert.doom(selected.prophecies[0], { reason: 'nope' }, 'must fail mismatch')).toThrowError(/TSPACK_ASSERT_DOOM_FAILED|doom failed/);
    expect(() => assert.doom(selected.prophecies[0], {}, '')).toThrowError(/assertion reason is required/);
    cleanupBridge();
  });

  it('covers envelope missing/invalid/mismatch and non-selected prophecy isolation', async () => {
    const cleanupBridge = installDoomBridgeStub();
    const root = makeDir();
    const importPath = nativeImportPath();
    fs.writeFileSync(path.join(root, 'env.prophecy.tsx'), `import fs from 'node:fs'; import path from 'node:path'; import { Suite, Prophecy, Foretell } from '${importPath}'; export default (<Suite name="s"><Prophecy name="missingEnvelope"><Foretell reason="m" />{() => { process.exit(7); }}</Prophecy><Prophecy name="badEnvelope"><Foretell reason="b" />{() => { process.exit(7); }}</Prophecy><Prophecy name="mismatchEnvelope"><Foretell reason="good" />{() => { process.exit(7); }}</Prophecy><Prophecy name="notSelected"><Foretell reason="x" />{() => { fs.writeFileSync(path.join('${root.replaceAll('\\', '/')}', 'ran.txt'),'yes'); process.exit(7); }}</Prophecy></Suite>);`);
    const run = await runNativeProphecies({ rootDir: root, filter: 'Envelope' });
    const byId = new Map(run.prophecies.map((p) => [p.id, p.failure?.code]));
    expect(byId.get('env.prophecy.tsx::s/prophecy/missingEnvelope')).toBe('TSPACK_DOOM_ENVELOPE_MISSING');
    expect(byId.get('env.prophecy.tsx::s/prophecy/badEnvelope')).toBe('TSPACK_DOOM_ENVELOPE_INVALID');
    expect(byId.get('env.prophecy.tsx::s/prophecy/mismatchEnvelope')).toBe('TSPACK_DOOM_ENVELOPE_MISMATCH');
    expect(fs.existsSync(path.join(root, 'ran.txt'))).toBe(false);
    cleanupBridge();
  });
});
