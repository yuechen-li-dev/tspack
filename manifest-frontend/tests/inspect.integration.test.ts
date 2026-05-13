import fs from 'node:fs';
import http from 'node:http';
import os from 'node:os';
import path from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { inspectAndWrite, parsePoint } from '../src/inspect/index.js';
import { runInspect } from '../src/inspect/browser.js';
import { formatInspectJson, formatInspectText } from '../src/inspect/format.js';

type TestServer = {
  baseUrl: string;
  close: () => Promise<void>;
};

const pages: Record<string, string> = {
  '/basic': `<!doctype html><html><body><header><div>Machina</div></header><main><h1>Account settings</h1><p>Manage your profile settings.</p><button aria-label="Save settings">Save</button><a href="/docs">Read docs</a></main></body></html>`,
  '/selector': `<!doctype html><html><body><section id="root"><h1>Inside Root</h1><button>Root Button</button></section><aside><h2>Outside Root</h2></aside></body></html>`,
  '/hit': `<!doctype html><html><body style="margin:0;"><main style="position:relative; width:500px; height:300px;"><button style="position:absolute; left:100px; top:80px; width:220px; height:60px;">Hit Target</button></main></body></html>`
};

async function chromiumAvailability(): Promise<{ available: boolean; reason?: string }> {
  try {
    const playwright = await import('playwright');
    const browser = await playwright.chromium.launch();
    await browser.close();
    return { available: true };
  } catch (error: unknown) {
    const message = error instanceof Error ? error.message : String(error);
    return { available: false, reason: `PLAYWRIGHT_UNAVAILABLE: ${message}` };
  }
}

const chromium = await chromiumAvailability();
const describeIntegration = chromium.available ? describe : describe.skip;

let server: TestServer;

async function createServer(): Promise<TestServer> {
  const httpServer = http.createServer((req, res) => {
    const pathname = new URL(req.url ?? '/', 'http://localhost').pathname;
    const html = pages[pathname];
    if (!html) {
      res.statusCode = 404;
      res.end('missing');
      return;
    }
    res.setHeader('content-type', 'text/html; charset=utf-8');
    res.end(html);
  });

  await new Promise<void>((resolve) => {
    httpServer.listen(0, '127.0.0.1', () => resolve());
  });

  const address = httpServer.address();
  if (!address || typeof address === 'string') {
    throw new Error('failed to bind test server');
  }

  return {
    baseUrl: `http://127.0.0.1:${address.port}`,
    close: async () => new Promise<void>((resolve, reject) => httpServer.close((err) => (err ? reject(err) : resolve())))
  };
}

describeIntegration('inspect browser integration', () => {
  beforeAll(async () => {
    server = await createServer();
  });

  afterAll(async () => {
    await server.close();
  });

  it('inspects basic local page', async () => {
    const result = await runInspect({
      url: `${server.baseUrl}/basic`,
      browser: 'chromium',
      viewport: { width: 1440, height: 900 },
      points: [],
      json: true
    });

    expect(result.target.url).toContain('/basic');
    expect(result.browser.name).toBe('chromium');
    expect(result.viewport.width).toBe(1440);
    expect(result.viewport.height).toBe(900);
    expect(result.root).not.toBeNull();
    expect(result.root?.bounds.width).toBeGreaterThan(0);

    const text = formatInspectText(result);
    expect(text).toContain('UI Inspect:');
    expect(text).toContain('Browser: chromium');
    expect(text).toContain('h1 heading "Account settings"');
    expect(text).toContain('button button "Save settings"');

    const json = formatInspectJson(result);
    expect(json.endsWith('\n')).toBe(true);
    const parsed = JSON.parse(json);
    expect(parsed.root).toBeTruthy();
    expect(Array.isArray(parsed.hitTests)).toBe(true);
    expect(Array.isArray(parsed.diagnostics)).toBe(true);
  });

  it('supports selector roots and missing selector diagnostic', async () => {
    const result = await runInspect({
      url: `${server.baseUrl}/selector`,
      browser: 'chromium',
      viewport: { width: 800, height: 600 },
      selector: '#root',
      points: [],
      json: true
    });

    expect(result.root?.tag).toBe('section');
    const text = formatInspectText(result);
    expect(text).toContain('Inside Root');
    expect(text).not.toContain('Outside Root');

    await expect(
      runInspect({
        url: `${server.baseUrl}/selector`,
        browser: 'chromium',
        viewport: { width: 800, height: 600 },
        selector: '#missing',
        points: [],
        json: true
      })
    ).rejects.toThrow('TSPACK_INSPECT_SELECTOR_NOT_FOUND');
  });

  it('supports hit testing', async () => {
    const result = await runInspect({
      url: `${server.baseUrl}/hit`,
      browser: 'chromium',
      viewport: { width: 600, height: 400 },
      points: [parsePoint('130,100')],
      json: true
    });

    expect(result.hitTests).toHaveLength(1);
    expect(result.hitTests[0].point).toEqual({ x: 130, y: 100 });
    const top = result.hitTests[0].elements[0];
    expect(top.tag).toBe('button');
    expect(top.name).toContain('Hit Target');
    expect(top.visible).toBe(true);

    const text = formatInspectText(result);
    expect(text).toContain('Hit tests:');
    expect(text).toContain('point 130,100');
  });

  it('validates unsupported browser and invalid/unreachable target', async () => {
    await expect(
      runInspect({ url: `${server.baseUrl}/basic`, browser: 'firefox', viewport: { width: 800, height: 600 }, points: [], json: true })
    ).rejects.toThrow('TSPACK_INSPECT_BROWSER_UNSUPPORTED');

    await expect(
      runInspect({ url: 'notaurl', browser: 'chromium', viewport: { width: 800, height: 600 }, points: [], json: true })
    ).rejects.toThrow('TSPACK_INSPECT_INVALID_TARGET');

    await expect(
      runInspect({ url: 'http://127.0.0.1:1/unreachable', browser: 'chromium', viewport: { width: 800, height: 600 }, points: [], json: true })
    ).rejects.toThrow('TSPACK_INSPECT_PAGE_LOAD_FAILED');
  });

  it('inspectAndWrite writes --out and --text artifacts', async () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'inspect-test-'));
    const jsonPath = path.join(dir, 'inspect.json');
    const textPath = path.join(dir, 'inspect.txt');

    const originalWrite = process.stdout.write.bind(process.stdout);
    const chunks: string[] = [];
    (process.stdout.write as unknown as (chunk: string) => boolean) = (chunk: string) => {
      chunks.push(chunk);
      return true;
    };

    try {
      await inspectAndWrite({
        url: `${server.baseUrl}/basic`,
        browser: 'chromium',
        viewport: { width: 1024, height: 768 },
        points: [],
        json: true,
        out: jsonPath,
        text: textPath
      });
    } finally {
      process.stdout.write = originalWrite;
    }

    const json = fs.readFileSync(jsonPath, 'utf8');
    const text = fs.readFileSync(textPath, 'utf8');
    expect(() => JSON.parse(json)).not.toThrow();
    expect(text).toContain('UI Inspect:');
    expect(chunks.join('')).toContain('"target"');
  });
});

if (!chromium.available) {
  // eslint-disable-next-line no-console
  console.warn(chromium.reason);
}
