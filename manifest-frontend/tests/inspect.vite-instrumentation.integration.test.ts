import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { afterEach, describe, expect, it } from 'vitest';
import { build, createServer, preview } from 'vite';
import { runInspect } from '../src/inspect/backend.js';
import {
  buildUIContextBundle,
  serializeUiContextBundle,
} from '../src/inspect/context-bundle.js';
import { tspackInspectSourceVitePlugin } from '../src/inspect/source-instrumentation.js';
import { runInspectWatch } from '../src/inspect/watch.js';

const temporaryRoots: string[] = [];
const servers: Array<{ close(): Promise<void> }> = [];

afterEach(async () => {
  delete process.env.TSPACK_INSPECT_WATCH_MAX_CYCLES;
  await Promise.all(servers.splice(0).map((server) => server.close()));
  await Promise.all(
    temporaryRoots.splice(0).map((root) => fs.rm(root, {
      recursive: true,
      force: true,
    })),
  );
});

async function createFixture(): Promise<string> {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'tspack-vite-m73-'));
  temporaryRoots.push(root);
  await fs.mkdir(path.join(root, 'src'));
  await fs.writeFile(
    path.join(root, 'index.html'),
    '<div id="root"></div><script type="module" src="/src/App.tsx"></script>\n',
  );
  await fs.writeFile(
    path.join(root, 'src', 'App.tsx'),
    `const React = {
  createElement(tag: string, props: Record<string, unknown> | null, ...children: unknown[]) {
    const element = document.createElement(tag);
    for (const [name, value] of Object.entries(props ?? {})) {
      if (value !== undefined && value !== false) element.setAttribute(name, String(value));
    }
    for (const child of children.flat()) {
      element.append(child instanceof Node ? child : document.createTextNode(String(child)));
    }
    return element;
  },
};
export function Toast() {
  return <div role="alert"><button type="button">Dismiss</button></div>;
}
document.querySelector('#root')!.replaceChildren(Toast());
`,
  );
  return root;
}

describe('Vite inspect source instrumentation', () => {
  it('injects original TSX provenance in a real dev transform', async () => {
    const root = await createFixture();
    const server = await createServer({
      root,
      logLevel: 'silent',
      plugins: [tspackInspectSourceVitePlugin({ workspaceRoot: root })],
      server: { host: '127.0.0.1', port: 0 },
    });
    servers.push(server);
    await server.listen();
    const address = server.httpServer?.address();
    if (!address || typeof address === 'string') {
      throw new Error('Vite did not expose a TCP address');
    }

    const response = await fetch(
      `http://127.0.0.1:${address.port}/src/App.tsx`,
    );
    const transformed = await response.text();

    expect(response.ok).toBe(true);
    expect(transformed).toContain('data-tspack-source');
    expect(transformed).toMatch(/src\/App\.tsx:\d+:10/);
    expect(transformed).toContain('data-tspack-component');
    expect(transformed).toContain('Toast');
    expect(transformed).not.toContain(root.replace(/\\/g, '/'));
    const sourceMapMarker = 'base64,';
    const sourceMapOffset = transformed.lastIndexOf(sourceMapMarker);
    expect(sourceMapOffset).toBeGreaterThan(0);
    const sourceMapText = Buffer.from(
      transformed.slice(sourceMapOffset + sourceMapMarker.length).trim(),
      'base64',
    ).toString('utf8');
    const sourceMap = JSON.parse(sourceMapText) as { sources: string[] };
    expect(sourceMap.sources).toEqual(['App.tsx']);
  });

  it('leaves zero instrumentation footprint in actual production artifacts', async () => {
    const root = await createFixture();
    const outputDirectory = path.join(root, 'production');
    await build({
      root,
      logLevel: 'silent',
      plugins: [tspackInspectSourceVitePlugin({ workspaceRoot: root })],
      build: { outDir: outputDirectory },
    });

    const artifactPaths = await collectFiles(outputDirectory);
    const artifacts = (
      await Promise.all(artifactPaths.map((filePath) => fs.readFile(filePath, 'utf8')))
    ).join('\n');
    expect(artifacts).not.toContain('data-tspack-source');
    expect(artifacts).not.toContain('data-tspack-component');
    expect(artifacts).not.toContain('data-tspack-symbol');
    expect(artifacts).not.toContain('tspack-inspect-source-instrumentation');

    const previewServer = await preview({
      root,
      logLevel: 'silent',
      build: { outDir: outputDirectory },
      preview: { host: '127.0.0.1', port: 0 },
    });
    servers.push(previewServer);
    const address = previewServer.httpServer.address();
    if (!address || typeof address === 'string') {
      throw new Error('Vite preview did not expose a TCP address');
    }
    const inspected = await runInspect({
      url: `http://127.0.0.1:${address.port}`,
      root,
      browser: 'chromium',
      viewport: { width: 800, height: 600 },
      selector: '[role="alert"]',
      points: [],
      json: false,
    });
    expect(inspected.root?.role).toBe('alert');
    expect(inspected.root?.source).toBeUndefined();
  });

  it('grounds a real browser inspection and deterministic bundle in original TSX', async () => {
    const root = await createFixture();
    const server = await createServer({
      root,
      logLevel: 'silent',
      plugins: [tspackInspectSourceVitePlugin({ workspaceRoot: root })],
      server: { host: '127.0.0.1', port: 0 },
    });
    servers.push(server);
    await server.listen();
    const address = server.httpServer?.address();
    if (!address || typeof address === 'string') {
      throw new Error('Vite did not expose a TCP address');
    }

    const inspected = await runInspect({
      url: `http://127.0.0.1:${address.port}`,
      root,
      browser: 'chromium',
      viewport: { width: 800, height: 600 },
      selector: '[role="alert"]',
      points: [],
      json: false,
    });
    expect(inspected.root?.source).toMatchObject({
      file: 'src/App.tsx',
      component: 'Toast',
    });
    expect(inspected.root?.source?.line).toBeGreaterThan(1);
    if (process.env.TSPACK_EXPECT_SYSTEM_CHROMIUM === '1') {
      expect(inspected.browser.executable?.source).toBe('system');
    }
    const button = inspected.root?.children.find((node) => node.role === 'button');
    expect(button?.name).toBe('Dismiss');
    expect(button?.focusable).toBe(true);

    if (!inspected.root) {
      throw new Error('inspect result did not contain the selected root');
    }
    const first = await buildUIContextBundle(inspected, inspected.root, {
      workspaceRoot: root,
      workspaceRootName: path.basename(root),
      selectionReason: 'automatic instrumentation fixture',
    });
    const second = await buildUIContextBundle(inspected, inspected.root, {
      workspaceRoot: root,
      workspaceRootName: path.basename(root),
      selectionReason: 'automatic instrumentation fixture',
    });
    expect(first.source?.validated).toBe(true);
    expect(first.source?.file).toBe('src/App.tsx');
    expect(serializeUiContextBundle(first)).toBe(serializeUiContextBundle(second));
  });

  it('reuses the real browser and observes updated source truth after a save', async () => {
    const root = await createFixture();
    const server = await createServer({
      root,
      logLevel: 'silent',
      plugins: [tspackInspectSourceVitePlugin({ workspaceRoot: root })],
      server: { host: '127.0.0.1', port: 0 },
    });
    servers.push(server);
    await server.listen();
    const address = server.httpServer?.address();
    if (!address || typeof address === 'string') {
      throw new Error('Vite did not expose a TCP address');
    }

    const sourcePath = path.join(root, 'src', 'App.tsx');
    const originalSource = await fs.readFile(sourcePath, 'utf8');
    const observedLines: number[] = [];
    const reuse: boolean[] = [];
    process.env.TSPACK_INSPECT_WATCH_MAX_CYCLES = '2';
    await runInspectWatch({
      root,
      debounceMilliseconds: 50,
      inspectOptions: {
        url: `http://127.0.0.1:${address.port}`,
        root,
        browser: 'chromium',
        viewport: { width: 800, height: 600 },
        selector: '[role="alert"]',
        points: [],
        json: false,
      },
      async onCycle(cycle) {
        observedLines.push(cycle.result.root?.source?.line ?? 0);
        reuse.push(cycle.browserReused);
        if (cycle.generation === 1) {
          await fs.writeFile(sourcePath, `\n${originalSource}`);
        }
      },
    });

    expect(observedLines).toHaveLength(2);
    expect(observedLines[1]).toBe(observedLines[0] + 1);
    expect(reuse).toEqual([false, true]);
  });
});

async function collectFiles(root: string): Promise<string[]> {
  const files: string[] = [];
  for (const entry of await fs.readdir(root, { withFileTypes: true })) {
    const entryPath = path.join(root, entry.name);
    if (entry.isDirectory()) {
      files.push(...await collectFiles(entryPath));
    } else {
      files.push(entryPath);
    }
  }
  return files.sort();
}
