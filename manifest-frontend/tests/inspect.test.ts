import { describe, expect, it } from 'vitest';
import { formatInspectJson, formatInspectText } from '../src/inspect/format.js';
import { parsePoint, parseViewport } from '../src/inspect/index.js';
import { findVSCodeExecutable, runInspect } from '../src/inspect/backend.js';

describe('inspect parsing', () => {
  it('parses viewport', () => {
    expect(parseViewport('1440x900')).toEqual({ width: 1440, height: 900 });
    expect(() => parseViewport('a')).toThrow('TSPACK_INSPECT_INVALID_VIEWPORT');
  });

  it('parses point', () => {
    expect(parsePoint('320,148')).toEqual({ x: 320, y: 148 });
    expect(() => parsePoint('-1,2')).toThrow('TSPACK_INSPECT_INVALID_POINT');
  });

  it('formats text and json', () => {
    const result = {
      target: { url: 'http://localhost:5173' },
      browser: { name: 'chromium' as const },
      viewport: { width: 1440, height: 900 },
      root: {
        id: 'node-1',
        tag: 'h1',
        role: 'heading',
        name: 'Title',
        bounds: { x: 1, y: 2, width: 3, height: 4 },
        visible: true,
        children: []
      },
      hitTests: [],
      diagnostics: []
    };
    const text = formatInspectText(result);
    expect(text).toContain('UI Inspect: http://localhost:5173');
    expect(text).toContain('Browser: chromium');
    const json = formatInspectJson(result);
    expect(() => JSON.parse(json)).not.toThrow();
    expect(json.endsWith('\n')).toBe(true);
  });

  it('supports discovery and browser selection errors', async () => {
    expect(findVSCodeExecutable() === null || typeof findVSCodeExecutable() === 'string').toBe(true);

    await expect(
      runInspect({
        url: 'http://127.0.0.1:9999',
        browser: 'browser-path',
        browserPath: '/definitely/missing/browser',
        viewport: { width: 800, height: 600 },
        points: [],
        json: true
      })
    ).rejects.toThrow('TSPACK_INSPECT_BROWSER_PATH_NOT_FOUND');

    await expect(
      runInspect({
        url: 'http://127.0.0.1:9999',
        browser: 'vscode',
        viewport: { width: 800, height: 600 },
        points: [],
        json: true
      })
    ).rejects.toThrow(/TSPACK_INSPECT_VSCODE_/);
  });
});
