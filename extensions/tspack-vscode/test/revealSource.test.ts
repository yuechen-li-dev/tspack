import * as fs from 'node:fs/promises';
import * as os from 'node:os';
import * as path from 'node:path';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import {
  buildRevealTarget,
  isSourceHintMalformed,
  resolveSourceHintPath,
} from '../src/revealSource';

let tempRoot: string;

async function writeWorkspaceFile(
  relativeFile: string,
  contents = 'content',
): Promise<string> {
  const filePath = path.join(tempRoot, ...relativeFile.split('/'));
  await fs.mkdir(path.dirname(filePath), { recursive: true });
  await fs.writeFile(filePath, contents);
  return filePath;
}

beforeEach(async () => {
  tempRoot = await fs.mkdtemp(
    path.join(os.tmpdir(), 'tspack-reveal-source-'),
  );
});

afterEach(async () => {
  await fs.rm(tempRoot, { recursive: true, force: true });
});

describe('source hint path validation', () => {
  it('resolves relative safe paths inside the workspace', async () => {
    const filePath = await writeWorkspaceFile('src/Button.tsx');

    const resolved = await resolveSourceHintPath(tempRoot, 'src/Button.tsx');

    expect(resolved.ok).toBe(true);
    if (!resolved.ok) {
      throw new Error('expected source hint path to resolve');
    }
    expect(resolved.resolvedPath).toBe(filePath);
    expect(resolved.realPath).toBe(await fs.realpath(filePath));
  });

  it('normalizes backslash paths before resolving', async () => {
    const filePath = await writeWorkspaceFile('src/components/Button.tsx');

    const resolved = await resolveSourceHintPath(
      tempRoot,
      'src\\components\\Button.tsx',
    );

    expect(resolved.ok).toBe(true);
    if (!resolved.ok) {
      throw new Error('expected backslash source hint path to resolve');
    }
    expect(resolved.displayPath).toBe('src/components/Button.tsx');
    expect(resolved.resolvedPath).toBe(filePath);
  });

  it('rejects absolute paths', async () => {
    const resolved = await resolveSourceHintPath(tempRoot, '/etc/passwd');

    expect(resolved).toMatchObject({
      ok: false,
      reason: 'unsafePath',
    });
  });

  it('rejects parent traversal paths', async () => {
    const resolved = await resolveSourceHintPath(tempRoot, '../outside.tsx');

    expect(resolved).toMatchObject({
      ok: false,
      reason: 'unsafePath',
    });
  });

  it('rejects nested parent traversal before normalization', async () => {
    await writeWorkspaceFile('Button.tsx');

    const resolved = await resolveSourceHintPath(tempRoot, 'src/../Button.tsx');

    expect(resolved).toMatchObject({
      ok: false,
      reason: 'unsafePath',
    });
  });

  it('rejects URL-like schemes', async () => {
    const fileUrl = await resolveSourceHintPath(
      tempRoot,
      'file:///tmp/Button.tsx',
    );
    const httpUrl = await resolveSourceHintPath(
      tempRoot,
      'http://example.test/Button.tsx',
    );

    expect(fileUrl).toMatchObject({
      ok: false,
      reason: 'unsafePath',
    });
    expect(httpUrl).toMatchObject({
      ok: false,
      reason: 'unsafePath',
    });
  });

  it('reports missing files without creating them', async () => {
    const resolved = await resolveSourceHintPath(tempRoot, 'src/Missing.tsx');

    expect(resolved).toMatchObject({
      ok: false,
      reason: 'notFound',
      displayPath: 'src/Missing.tsx',
    });
  });

  it('rejects symlinks that escape the workspace', async () => {
    const outsideRoot = await fs.mkdtemp(
      path.join(os.tmpdir(), 'tspack-outside-'),
    );
    try {
      const outsideFile = path.join(outsideRoot, 'Secret.tsx');
      await fs.writeFile(outsideFile, 'secret');
      await fs.symlink(outsideFile, path.join(tempRoot, 'Secret.tsx'));

      const resolved = await resolveSourceHintPath(tempRoot, 'Secret.tsx');

      expect(resolved).toMatchObject({
        ok: false,
        reason: 'outsideWorkspace',
      });
    } finally {
      await fs.rm(outsideRoot, { recursive: true, force: true });
    }
  });
});

describe('reveal target construction', () => {
  it('converts one-based source line and column to zero-based editor positions', () => {
    const target = buildRevealTarget('/workspace/src/Button.tsx', {
      file: 'src/Button.tsx',
      line: 42,
      column: 7,
    });

    expect(target).toEqual({
      file: '/workspace/src/Button.tsx',
      line: 41,
      column: 6,
    });
  });

  it('opens source hints without line or column at the top of the file', () => {
    const target = buildRevealTarget('/workspace/src/Button.tsx', {
      file: 'src/Button.tsx',
    });

    expect(target).toEqual({
      file: '/workspace/src/Button.tsx',
      line: 0,
      column: 0,
    });
  });

  it('detects malformed raw source hints without parsed files', () => {
    expect(isSourceHintMalformed({
      raw: 'src/Button.tsx:not-a-line',
      parseError: 'line must be a positive integer',
    })).toBe(true);
  });

  it('does not mark parsed source hints as malformed', () => {
    expect(isSourceHintMalformed({
      raw: 'src/Button.tsx:1:1',
      file: 'src/Button.tsx',
      line: 1,
      column: 1,
    })).toBe(false);
  });
});
