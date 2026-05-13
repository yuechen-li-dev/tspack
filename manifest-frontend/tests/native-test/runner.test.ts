import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import { Artifact, assert, Case, Fact, Project, runSuite, skip, Suite, Theory } from '../../src/native-test/index';

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
          assert.true(true, 'artifact write fact asserts meaningful action');
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
      Fact({ name: 'optional' }, Artifact({ name: 'x', path: 'x.txt', optional: true }), () => { assert.true(true, 'optional still asserts'); }),
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
    const root = Suite({ name: 't' }, Theory({ name: 'cases' }, Artifact({ name: 'r', path: 'report.txt' }), Case({ n: 1 }), Case({ n: 2 }), async ({ n }, { artifact }) => { await artifact.writeText('r', String(n), 'record'); assert.true(true, 'case asserted'); }));
    const results = await runSuite(root, { artifactRoot });
    expect(results.map((r) => r.status)).toEqual(['passed', 'passed']);
    expect(fs.existsSync(path.join(artifactRoot, 't__cases__0', 'report.txt'))).toBe(true);
    expect(fs.existsSync(path.join(artifactRoot, 't__cases__1', 'report.txt'))).toBe(true);
  });
});

describe('native runner project fixtures', () => {
  it('creates sandbox, supports reads and writes, and cleans up on pass', async () => {
    const fixtureRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'native-project-fixture-'));
    fs.writeFileSync(path.join(fixtureRoot, 'manifest.json'), '{"x":1}\n');

    const root = Suite({ name: 'p' },
      Fact({ name: 'project' },
        Project({ from: fixtureRoot },
        ),
        async ({ project }) => {
          expect(project).toBeDefined();
          const parsed = await project!.readJson<{ x: number }>('manifest.json');
          assert.equal(parsed.x, 1, 'reads copied fixture json');
          await project!.writeText('generated.txt', 'hello\n', 'write generated file');
          const text = await project!.readText('generated.txt');
          assert.equal(text, 'hello\n', 'reads generated file');
        },
      ),
    );

    const results = await runSuite(root, {});
    expect(results[0].status).toBe('passed');
    expect(results[0].project?.kept).toBe(false);
    expect(fs.existsSync(path.join(fixtureRoot, 'generated.txt'))).toBe(false);
  });
});

it('enforces project path/write/read safety and keepOnFailure semantics', async () => {
  const fixtureRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'native-project-fixture-'));
  fs.mkdirSync(path.join(fixtureRoot, 'node_modules'));
  fs.mkdirSync(path.join(fixtureRoot, '.git'));
  fs.writeFileSync(path.join(fixtureRoot, 'ok.json'), '{"b":2,"a":1}');

  const root = Suite({ name: 'px' },
    Fact({ name: 'empty' }, Project({}), async ({ project }) => { assert.true(!!project?.rootPath, 'has root'); }),
    Fact({ name: 'path' }, Project({ from: fixtureRoot }), async ({ project }) => {
      const invalid = ['', '   ', '/abs', '..', 'a/../b', 'a\\b'];
      for (const item of invalid) {
        await expect(project!.readText(item)).rejects.toThrow();
      }
    }),
    Fact({ name: 'write reason' }, Project({ from: fixtureRoot }), async ({ project }) => { await project!.writeText('x.txt', 'x', ''); }),
    Fact({ name: 'read missing' }, Project({ from: fixtureRoot }), async ({ project }) => { await project!.readText('missing.txt'); }),
    Fact({ name: 'write/json/bytes/read' }, Project({ from: fixtureRoot }), async ({ project }) => {
      await project!.writeText('sub/t.txt', 'hello', 'write text');
      await project!.writeJson('sub/j.json', { z: 1, a: 2 }, 'write json');
      await project!.writeBytes('sub/b.bin', new Uint8Array([1, 2, 3]), 'write bytes');
      assert.equal(await project!.readText('sub/t.txt'), 'hello', 'read text');
      const json = await project!.readJson<{ z: number; a: number }>('sub/j.json');
      assert.equal(json.a, 2, 'read json');
      assert.true(fs.existsSync(project!.path('sub/b.bin')), 'bytes exists');
      assert.true(!fs.existsSync(path.join(project!.rootPath, 'node_modules')), 'skip node_modules copy');
      assert.true(!fs.existsSync(path.join(project!.rootPath, '.git')), 'skip .git copy');
    }),
    Theory({ name: 'separate' }, Project({}), Case({ n: 1 }), Case({ n: 2 }), async ({ n }, { project }) => {
      await project!.writeText(`case-${n}.txt`, String(n), 'case file');
      assert.true(fs.existsSync(project!.path(`case-${n}.txt`)), 'file in case sandbox');
    }),
    Fact({ name: 'artifact and project' }, Project({}), Artifact({ name: 'a', path: 'a.txt' }), async ({ artifact, project }) => {
      await project!.writeText('p.txt', 'p', 'project write');
      await artifact.writeText('a', 'a', 'artifact write');
      assert.true(true, 'artifact/project asserts');
    }),
    Fact({ name: 'keep false fail' }, Project({ keepOnFailure: false }), async () => { assert.equal(1, 2, 'fail'); }),
    Fact({ name: 'keep true fail' }, Project({ keepOnFailure: true }), async () => { assert.equal(1, 2, 'fail keep'); }),
    Fact({ name: 'keep true pass' }, Project({ keepOnFailure: true }), async ({ project }) => { await project!.writeText('ok.txt', 'ok', 'pass'); assert.true(true, 'pass has assertion'); }),
    Fact({ name: 'skip cleanup' }, Project({}), async () => { skip('later'); }),
  );

  const results = await runSuite(root, {});
  const byId = new Map(results.map((r) => [r.id, r]));
  expect(byId.get('px/empty')?.status).toBe('passed');
  expect((byId.get('px/write reason')?.error as { code?: string })?.code).toBe('TSPACK_PROJECT_WRITE_REASON_REQUIRED');
  expect((byId.get('px/read missing')?.error as { code?: string })?.code).toBe('TSPACK_PROJECT_READ_FAILED');
  expect(byId.get('px/write/json/bytes/read')?.status).toBe('passed');
  expect(byId.get('px/artifact and project')?.status).toBe('passed');
  expect(byId.get('px/skip cleanup')?.status).toBe('skipped');
  expect(byId.get('px/keep false fail')?.project?.kept).toBe(false);
  expect(byId.get('px/keep true fail')?.project?.kept).toBe(true);
  expect(byId.get('px/keep true pass')?.project?.kept).toBe(false);
});

describe('native runner command helpers', () => {
  it('provides command context only for project tests and captures evidence', async () => {
    const artifactRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'native-command-artifacts-'));
    const root = Suite({ name: 'cmd' },
      Fact({ name: 'no-project' }, async ({ command }) => {
        expect(command).toBeUndefined();
        assert.true(true, 'explicit assertion');
      }),
      Fact({ name: 'with-project' }, Project({}), async ({ command, project }) => {
        expect(command).toBeDefined();
        const result = await command!.run(['node', '-e', 'console.log(process.cwd())'], 'print cwd');
        assert.exitCode(result, 0, 'node command should pass');
        assert.true(result.stdout.includes(project!.rootPath), 'command should run in project cwd');
      }),
    );

    const results = await runSuite(root, { artifactRoot });
    expect(results[0].status).toBe('passed');
    expect(results[1].status).toBe('passed');

    const commandDir = path.join(artifactRoot, 'cmd__with-project', 'commands');
    expect(fs.existsSync(path.join(commandDir, '0.stdout.txt'))).toBe(true);
    expect(fs.existsSync(path.join(commandDir, '0.stderr.txt'))).toBe(true);
    expect(fs.existsSync(path.join(commandDir, '0.command.json'))).toBe(true);
    const payload = JSON.parse(fs.readFileSync(path.join(commandDir, '0.command.json'), 'utf8'));
    expect(payload.env).toBeUndefined();
  });

  it('supports invalid args/reason, timeout, nonzero, and invalid cwd', async () => {
    const root = Suite({ name: 'cmd2' },
      Fact({ name: 'nonzero' }, Project({}), async ({ command }) => {
        const result = await command!.run(['node', '-e', 'process.exit(5)'], 'nonzero');
        assert.exitCode(result, 5, 'expected nonzero should pass');
      }),
      Fact({ name: 'timeout' }, Project({}), async ({ command }) => {
        const result = await command!.run(['node', '-e', 'setTimeout(()=>{}, 2000)'], 'timeout', { timeoutSeconds: 0.05 });
        assert.true(result.timedOut, 'command should time out');
      }),
      Fact({ name: 'invalid-cwd' }, Project({}), async ({ command }) => {
        await command!.run(['node', '-e', 'console.log(1)'], 'bad cwd', { cwd: '../escape' });
      }),
      Fact({ name: 'missing-reason' }, Project({}), async ({ command }) => {
        await command!.run(['node', '-e', 'console.log(1)'], '');
      }),
    );

    const results = await runSuite(root, {});
    expect(results[0].status).toBe('passed');
    expect(results[1].status).toBe('passed');
    expect((results[2].error as { code?: string }).code).toBe('TSPACK_COMMAND_INVALID_CWD');
    expect((results[3].error as { code?: string }).code).toBe('TSPACK_COMMAND_REASON_REQUIRED');
  });

  it('supports command.tspack via injected executable path', async () => {
    const toolRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'fake-tspack-'));
    const fakeTool = path.join(toolRoot, 'fake-tspack.js');
    fs.writeFileSync(fakeTool, `#!/usr/bin/env node\nconsole.log(JSON.stringify({argv: process.argv.slice(2), cwd: process.cwd(), marker: process.env.MARKER ?? ''}))\n`);
    fs.chmodSync(fakeTool, 0o755);
    process.env.TSPACK_TEST_TSPACK_PATH = fakeTool;
    try {
      const root = Suite({ name: 'cmd3' },
        Fact({ name: 'tspack' }, Project({}), async ({ command, project }) => {
          fs.mkdirSync(path.join(project!.rootPath, 'sub'), { recursive: true });
          const result = await command!.tspack(['alpha', 'beta'], 'call tspack', { cwd: 'sub', env: { MARKER: 'yes' } });
          assert.exitCode(result, 0, 'fake tspack should pass');
          const payload = JSON.parse(result.stdout.trim());
          assert.equal(payload.argv, ['alpha', 'beta'], 'args passed');
          assert.true(String(payload.cwd).endsWith('/sub'), 'cwd option applied');
          assert.equal(payload.marker, 'yes', 'env merged');
          assert.true(!!result.evidence?.commandPath, 'evidence path provided');
        }),
      );
      const results = await runSuite(root, {});
      expect(results[0].status).toBe('passed');
    } finally {
      delete process.env.TSPACK_TEST_TSPACK_PATH;
    }
  });
});
