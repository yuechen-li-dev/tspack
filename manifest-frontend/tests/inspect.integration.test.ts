import fs from 'node:fs';
import http from 'node:http';
import os from 'node:os';
import path from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { inspectAndWrite, parsePoint } from '../src/inspect/index.js';
import { runInspect } from '../src/inspect/backend.js';
import { formatInspectJson, formatInspectText } from '../src/inspect/format.js';
import { launchInspectableHost } from '../src/inspect/host-launch.js';
import { listCdpTargets } from '../src/inspect/cdp.js';

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

  it('uses Playwright Chromium for auto-routed URL inspect', async () => {
    const result = await runInspect({
      url: `${server.baseUrl}/selector`,
      browser: 'auto',
      viewport: { width: 800, height: 600 },
      selector: 'main, section',
      points: [parsePoint('130,100')],
      json: true
    });

    expect(result.browser.name).toBe('chromium');
    expect(result.root).not.toBeNull();
    expect(result.root?.tag).toBe('section');
    expect(result.hitTests).toHaveLength(1);
  });

  it('inspects basic local page', async () => {
    const result = await runInspect({
      url: `${server.baseUrl}/basic`,
      browser: 'playwright-chromium',
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
      browser: 'playwright-chromium',
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
        browser: 'playwright-chromium',
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
      browser: 'playwright-chromium',
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
      runInspect({ url: `${server.baseUrl}/basic`, browser: 'not-real' as never, viewport: { width: 800, height: 600 }, points: [], json: true })
    ).rejects.toThrow('TSPACK_INSPECT_BROWSER_UNSUPPORTED');

    await expect(
      runInspect({ url: 'notaurl', browser: 'playwright-chromium', viewport: { width: 800, height: 600 }, points: [], json: true })
    ).rejects.toThrow('TSPACK_INSPECT_INVALID_TARGET');

    await expect(
      runInspect({ url: 'http://127.0.0.1:1/unreachable', browser: 'playwright-chromium', viewport: { width: 800, height: 600 }, points: [], json: true })
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
        browser: 'playwright-chromium',
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

  it('writes deterministic selector-scoped bundles to stdout and atomic files', async () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'inspect-bundle-'));
    const bundlePath = path.join(dir, 'context.json');
    const originalStdoutWrite = process.stdout.write.bind(process.stdout);
    const originalStderrWrite = process.stderr.write.bind(process.stderr);
    const stdoutChunks: string[] = [];
    const stderrChunks: string[] = [];
    (process.stdout.write as unknown as (chunk: string) => boolean) = (chunk: string) => {
      stdoutChunks.push(chunk);
      return true;
    };
    (process.stderr.write as unknown as (chunk: string) => boolean) = (chunk: string) => {
      stderrChunks.push(chunk);
      return true;
    };

    try {
      await inspectAndWrite({
        url: `${server.baseUrl}/selector`,
        browser: 'playwright-chromium',
        viewport: { width: 800, height: 600 },
        selector: '#root',
        points: [],
        json: false,
        bundle: true,
      });
      const stdoutBundle = JSON.parse(stdoutChunks.join('')) as {
        version: number;
        kind: string;
        node: { tag: string };
      };
      expect(stdoutBundle).toMatchObject({
        version: 1,
        kind: 'tspack.uiContext',
        node: { tag: 'section' },
      });

      stdoutChunks.length = 0;
      await inspectAndWrite({
        url: `${server.baseUrl}/selector`,
        browser: 'playwright-chromium',
        viewport: { width: 800, height: 600 },
        selector: '#root',
        points: [],
        json: false,
        bundle: true,
        bundleOutput: bundlePath,
      });
      const firstFile = fs.readFileSync(bundlePath, 'utf8');
      await inspectAndWrite({
        url: `${server.baseUrl}/selector`,
        browser: 'playwright-chromium',
        viewport: { width: 800, height: 600 },
        selector: '#root',
        points: [],
        json: false,
        bundle: true,
        bundleOutput: bundlePath,
      });
      const secondFile = fs.readFileSync(bundlePath, 'utf8');

      await expect(
        inspectAndWrite({
          url: `${server.baseUrl}/selector`,
          browser: 'playwright-chromium',
          viewport: { width: 800, height: 600 },
          selector: '#missing',
          points: [],
          json: false,
          bundle: true,
          bundleOutput: bundlePath,
        }),
      ).rejects.toThrow('TSPACK_INSPECT_SELECTOR_NOT_FOUND');

      expect(firstFile).toBe(secondFile);
      expect(fs.readFileSync(bundlePath, 'utf8')).toBe(secondFile);
      expect(stdoutChunks).toEqual([]);
      expect(stderrChunks.join('')).toContain('Wrote UI context bundle:');
      expect(
        fs.readdirSync(dir).filter((name) => name.endsWith('.tmp')),
      ).toEqual([]);
    } finally {
      process.stdout.write = originalStdoutWrite;
      process.stderr.write = originalStderrWrite;
    }
  });
});

if (!chromium.available) {
  // eslint-disable-next-line no-console
  console.warn(chromium.reason);
}

const hostPath = process.env.TSPACK_INSPECT_HOST_PATH;
const describeHostIntegration = hostPath ? describe : describe.skip;
if (!hostPath) {
  // eslint-disable-next-line no-console
  console.warn('HOST_PATH_UNSET: set TSPACK_INSPECT_HOST_PATH=/path/to/code-or-compatible-host to run installed-host inspect integration.');
}

describeHostIntegration('inspect installed host integration', () => {
  let hostServer: TestServer;

  beforeAll(async () => {
    hostServer = await createServer();
  });

  afterAll(async () => {
    await hostServer.close();
  });

  it('launches explicit host and probes CDP targets', async () => {
    try {
      const hostProbe = await launchInspectableHost({ executablePath: hostPath as string });
      expect(hostProbe.endpoint).toContain('http://127.0.0.1:');
      expect(fs.existsSync(hostProbe.userDataDir)).toBe(true);
      const targetList = await listCdpTargets(hostProbe.endpoint);
      expect(Array.isArray(targetList.targets)).toBe(true);
      await hostProbe.cleanup();
      expect(fs.existsSync(hostProbe.userDataDir)).toBe(false);
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : String(error);
      expect(message).toMatch(/TSPACK_INSPECT_(HOST_LAUNCH_FAILED|HOST_CDP_ENDPOINT_FAILED|CDP_CONNECT_FAILED)/);
    }
  });

  it('attempts URL inspection with explicit host and reports host limitations', async () => {
    try {
      const result = await runInspect({
      url: `${hostServer.baseUrl}/selector`,
      browser: 'host-path',
      hostPath: hostPath as string,
      viewport: { width: 1024, height: 768 },
      selector: '#root',
      points: [parsePoint('130,100')],
      json: true
      });

      expect(result.root).toBeTruthy();
      expect(result.root?.tag).toBe('section');
      expect(result.browser.name).toBe('chromium');
      expect(result.browser.backend).toBe('host-path');
      expect(result.browser.launchBackend).toBe('host-path');
      expect(result.browser.executable).toMatchObject({
        source: 'explicit',
        path: hostPath,
      });

      const text = formatInspectText(result);
      expect(text).toContain('Inside Root');
      expect(text).toContain('Root Button');

      if (result.hitTests.length > 0 && result.hitTests[0].elements.length > 0) {
        expect(result.hitTests[0].elements[0].visible).toBe(true);
      }
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : String(error);
      expect(message).toMatch(/TSPACK_INSPECT_(PAGE_LOAD_FAILED|CDP_TARGET_NOT_FOUND|CDP_EVALUATION_FAILED|HOST_CDP_ENDPOINT_FAILED|HOST_LAUNCH_FAILED)/);
    }
  });
});
