import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import { loadPlaywrightCore, resolvePlaywrightCoreProvider } from '../src/inspect/playwright-provider.js';

describe('inspect playwright provider', () => {
  it('prefers TSPACK_PLAYWRIGHT_CORE_PATH', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'inspect-pw-env-'));
    fs.writeFileSync(path.join(dir, 'package.json'), '{"name":"playwright-core"}');
    process.env.TSPACK_PLAYWRIGHT_CORE_PATH = dir;
    try {
      const result = resolvePlaywrightCoreProvider('/tmp/nope');
      expect(result.source).toBe('env');
      expect(result.modulePath).toBe(dir);
    } finally {
      delete process.env.TSPACK_PLAYWRIGHT_CORE_PATH;
    }
  });

  it('finds local playwright-core', () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'inspect-pw-local-'));
    const modulePath = path.join(root, 'node_modules', 'playwright-core');
    fs.mkdirSync(modulePath, { recursive: true });
    fs.writeFileSync(path.join(modulePath, 'package.json'), '{"name":"playwright-core"}');
    const result = resolvePlaywrightCoreProvider(root);
    expect(result.source).toBe('local');
    expect(result.modulePath).toBe(modulePath);
  });

  it('throws not found for invalid env override', () => {
    process.env.TSPACK_PLAYWRIGHT_CORE_PATH = '/definitely/missing/playwright-core';
    try {
      expect(() => resolvePlaywrightCoreProvider('/tmp/ignore')).toThrow('TSPACK_INSPECT_PLAYWRIGHT_CORE_NOT_FOUND');
    } finally {
      delete process.env.TSPACK_PLAYWRIGHT_CORE_PATH;
    }
  });

  it('throws load failed for invalid module path', async () => {
    await expect(loadPlaywrightCore({ modulePath: '/definitely/missing/playwright-core', source: 'env' })).rejects.toThrow('TSPACK_INSPECT_PLAYWRIGHT_CORE_LOAD_FAILED');
  });
});
