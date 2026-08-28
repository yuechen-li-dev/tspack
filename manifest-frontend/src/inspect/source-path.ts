import * as fs from 'node:fs/promises';
import * as path from 'node:path';
import type { UISourceHint } from './types.js';

export type SourcePathResolution =
  | {
      ok: true;
      displayPath: string;
      resolvedPath: string;
      realPath: string;
    }
  | {
      ok: false;
      reason: 'missingPath' | 'unsafePath' | 'notFound' | 'outsideWorkspace';
      displayPath?: string;
    };

export type RevealTarget = {
  file: string;
  line: number;
  column: number;
};

type FileSystemAccess = {
  realpath(targetPath: string): Promise<string>;
};

const defaultFileSystemAccess: FileSystemAccess = {
  realpath: fs.realpath,
};

function normalizeHintSlashes(sourceFile: string): string {
  return sourceFile.replace(/\\/g, '/').trim();
}

function hasUrlLikeScheme(normalizedFile: string): boolean {
  return /^[A-Za-z][A-Za-z0-9+.-]*:/.test(normalizedFile);
}

function hasParentTraversal(normalizedFile: string): boolean {
  const segments = normalizedFile.split('/');
  return segments.some((segment) => segment === '..');
}

function isPathInside(parentPath: string, childPath: string): boolean {
  const relativePath = path.relative(parentPath, childPath);
  if (relativePath === '') {
    return true;
  }
  if (relativePath.startsWith('..')) {
    return false;
  }
  return !path.isAbsolute(relativePath);
}

export function isSourceHintMalformed(
  source: UISourceHint | undefined,
): boolean {
  if (!source) {
    return false;
  }
  return Boolean(source.raw && source.parseError);
}

export async function resolveSourceHintPath(
  workspaceRoot: string,
  sourceFile: string | undefined,
  fileSystemAccess: FileSystemAccess = defaultFileSystemAccess,
): Promise<SourcePathResolution> {
  if (!sourceFile || !sourceFile.trim()) {
    return { ok: false, reason: 'missingPath' };
  }

  const displayPath = normalizeHintSlashes(sourceFile);
  if (!displayPath || hasUrlLikeScheme(displayPath)) {
    return { ok: false, reason: 'unsafePath', displayPath };
  }
  if (path.posix.isAbsolute(displayPath) || path.win32.isAbsolute(displayPath)) {
    return { ok: false, reason: 'unsafePath', displayPath };
  }
  if (hasParentTraversal(displayPath)) {
    return { ok: false, reason: 'unsafePath', displayPath };
  }

  const workspaceRealPath = await fileSystemAccess.realpath(workspaceRoot);
  const resolvedPath = path.resolve(
    workspaceRealPath,
    ...displayPath.split('/'),
  );
  if (!isPathInside(workspaceRealPath, resolvedPath)) {
    return { ok: false, reason: 'outsideWorkspace', displayPath };
  }

  let targetRealPath: string;
  try {
    targetRealPath = await fileSystemAccess.realpath(resolvedPath);
  } catch {
    return { ok: false, reason: 'notFound', displayPath };
  }

  if (!isPathInside(workspaceRealPath, targetRealPath)) {
    return { ok: false, reason: 'outsideWorkspace', displayPath };
  }

  return {
    ok: true,
    displayPath,
    resolvedPath,
    realPath: targetRealPath,
  };
}

function toZeroBasedPosition(value: number | undefined): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return 0;
  }
  const zeroBased = Math.floor(value) - 1;
  if (zeroBased < 0) {
    return 0;
  }
  return zeroBased;
}

export function buildRevealTarget(
  resolvedPath: string,
  source: UISourceHint,
): RevealTarget {
  return {
    file: resolvedPath,
    line: toZeroBasedPosition(source.line),
    column: toZeroBasedPosition(source.column),
  };
}
