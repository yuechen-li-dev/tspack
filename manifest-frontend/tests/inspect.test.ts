import { describe, expect, it } from 'vitest';
import http from 'node:http';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { formatInspectJson, formatInspectText } from '../src/inspect/format.js';
import { parsePoint, parseViewport } from '../src/inspect/index.js';
import { findVSCodeExecutable, resolveVSCodeElectronExecutable, runInspect } from '../src/inspect/backend.js';
import { listCdpTargets, normalizeCdpEndpoint } from '../src/inspect/cdp.js';

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
    ).rejects.toThrow(/TSPACK_INSPECT_(VSCODE_|PAGE_LOAD_FAILED)/);


    await expect(
      runInspect({
        url: 'http://127.0.0.1:9999',
        browser: 'platform-webview',
        viewport: { width: 800, height: 600 },
        points: [],
        json: true
      })
    ).rejects.toThrow(/TSPACK_INSPECT_PLATFORM_WEBVIEW_(UNAVAILABLE|INIT_FAILED)/);
  }, 15000);


  it('resolves vscode wrapper path to electron binary when available', () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'inspect-vscode-resolve-'));
    const wrapperDir = path.join(root, 'bin');
    const shareDir = path.join(root, 'share', 'code');
    fs.mkdirSync(wrapperDir, { recursive: true });
    fs.mkdirSync(shareDir, { recursive: true });
    const wrapperPath = path.join(wrapperDir, 'code');
    const electronPath = path.join(shareDir, 'code');
    fs.writeFileSync(wrapperPath, '#!/usr/bin/env bash\n', { mode: 0o755 });
    fs.writeFileSync(electronPath, 'binary', { mode: 0o755 });

    expect(resolveVSCodeElectronExecutable(wrapperPath)).toBe(electronPath);
  });
});

describe('inspect cdp helpers', () => {
  it('validates cdp endpoint', () => {
    expect(normalizeCdpEndpoint('http://127.0.0.1:9222')).toBe('http://127.0.0.1:9222');
    expect(() => normalizeCdpEndpoint('')).toThrow('TSPACK_INSPECT_CDP_ENDPOINT_REQUIRED');
    expect(() => normalizeCdpEndpoint('ws://127.0.0.1:9222')).toThrow('TSPACK_INSPECT_CDP_ENDPOINT_INVALID');
  });

  it('lists inspectable cdp targets', async () => {
    const server = http.createServer((req, res) => {
      if ((req.url ?? '').startsWith('/json/list')) {
        res.setHeader('content-type', 'application/json');
        res.end(JSON.stringify([
          { id: 'page-1', type: 'page', title: 'App', url: 'http://localhost:5173/', webSocketDebuggerUrl: 'ws://x/page-1' },
          { id: 'devtools', type: 'other', title: 'DevTools', url: 'devtools://devtools' }
        ]));
        return;
      }
      res.statusCode = 404;
      res.end('missing');
    });
    await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', () => resolve()));
    const address = server.address();
    if (!address || typeof address === 'string') {
      throw new Error('failed to bind test server');
    }
    const endpoint = `http://127.0.0.1:${address.port}`;

    try {
      const result = await listCdpTargets(endpoint);
      expect(result.targets).toHaveLength(1);
      expect(result.targets[0].index).toBe(0);
      expect(result.targets[0].id).toBe('page-1');
    } finally {
      await new Promise<void>((resolve, reject) => server.close((err) => (err ? reject(err) : resolve())));
    }
  });
});
