import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import { Artifact, assert, Case, Fact, runSuite, skip, Suite, Theory } from '../../src/native-test/index';

describe('native runner artifacts', () => {
  it('writes text/json/bytes artifacts and records metadata', async () => {
    const artifactRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'native-artifacts-'));
    const root = Suite({ name: 'math' },
      Fact({ name: 'writes' },
        Artifact({ name: 'txt', path: 'a.txt' }),
        Artifact({ name: 'json', path: 'b.json' }),
        Artifact({ name: 'bin', path: 'c.bin' }),
        async ({ artifact }) => {
          await artifact.writeText('txt', 'hello', 'save text');
          await artifact.writeJson('json', { b: 2, a: 1 }, 'save json');
          await artifact.writeBytes('bin', new Uint8Array([1, 2]), 'save bytes');
        },
      ),
    );
    const results = await runSuite(root, { artifactRoot });
    expect(results[0].status).toBe('passed');
    expect(results[0].artifacts?.every((a) => a.written)).toBe(true);
    const dir = path.join(artifactRoot, 'math__writes');
    expect(fs.existsSync(path.join(dir, 'a.txt'))).toBe(true);
    expect(fs.existsSync(path.join(dir, 'b.json'))).toBe(true);
    expect(fs.existsSync(path.join(dir, 'c.bin'))).toBe(true);
  });

  it('fails on unknown, duplicate, missing reason, required not written; optional and skip behavior', async () => {
    const artifactRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'native-artifacts-'));
    const root = Suite({ name: 's' },
      Fact({ name: 'unknown' }, Artifact({ name: 'x', path: 'x.txt' }), async ({ artifact }) => artifact.writeText('y', 'v', 'r')),
      Fact({ name: 'dup' }, Artifact({ name: 'x', path: 'x.txt' }), async ({ artifact }) => { await artifact.writeText('x', 'a', 'r'); await artifact.writeText('x', 'b', 'r'); }),
      Fact({ name: 'reason' }, Artifact({ name: 'x', path: 'x.txt' }), async ({ artifact }) => artifact.writeText('x', 'a', '')),
      Fact({ name: 'required' }, Artifact({ name: 'x', path: 'x.txt' }), () => {}),
      Fact({ name: 'optional' }, Artifact({ name: 'x', path: 'x.txt', optional: true }), () => {}),
      Fact({ name: 'skip' }, Artifact({ name: 'x', path: 'x.txt' }), () => { skip('later'); }),
    );
    const results = await runSuite(root, { artifactRoot });
    expect((results[0].error as { code?: string }).code).toBe('TSPACK_ARTIFACT_UNKNOWN');
    expect((results[1].error as { code?: string }).code).toBe('TSPACK_ARTIFACT_ALREADY_WRITTEN');
    expect((results[2].error as { code?: string }).code).toBe('TSPACK_ARTIFACT_REASON_REQUIRED');
    expect((results[3].error as { code?: string }).code).toBe('TSPACK_ARTIFACT_REQUIRED_NOT_WRITTEN');
    expect(results[4].status).toBe('passed');
    expect(results[5].status).toBe('skipped');
  });

  it('writes theory case artifacts to separate directories', async () => {
    const artifactRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'native-artifacts-'));
    const root = Suite({ name: 't' }, Theory({ name: 'cases' }, Artifact({ name: 'r', path: 'report.txt' }), Case({ n: 1 }), Case({ n: 2 }), async ({ n }, { artifact }) => artifact.writeText('r', String(n), 'record')));
    const results = await runSuite(root, { artifactRoot });
    expect(results.map((r) => r.status)).toEqual(['passed', 'passed']);
    expect(fs.existsSync(path.join(artifactRoot, 't__cases__0', 'report.txt'))).toBe(true);
    expect(fs.existsSync(path.join(artifactRoot, 't__cases__1', 'report.txt'))).toBe(true);
  });
});
