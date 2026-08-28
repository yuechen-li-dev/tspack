import * as fs from 'node:fs/promises';
import * as os from 'node:os';
import * as path from 'node:path';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import type { InspectNode, InspectResult } from '../src/inspectTypes';
import {
  buildUiContextBundle,
  compactInspectNode,
  serializeUiContextBundle,
} from '../src/uiContextBundle';

let tempRoot: string;

function longText(): string {
  return Array.from({ length: 260 }, () => 'x').join('');
}

function sourceFileText(lineCount: number): string {
  return Array.from({ length: lineCount }, (_unused, index) => {
    return `line ${index + 1}`;
  }).join('\n');
}

async function writeWorkspaceFile(
  relativeFile: string,
  contents: string,
): Promise<string> {
  const filePath = path.join(tempRoot, ...relativeFile.split('/'));
  await fs.mkdir(path.dirname(filePath), { recursive: true });
  await fs.writeFile(filePath, contents);
  return filePath;
}

function makeFixture(): InspectResult {
  const siblings: InspectNode[] = [];
  for (let index = 0; index < 14; index += 1) {
    siblings.push({
      id: `sibling-${index}`,
      tag: 'div',
      text: `Sibling ${index}`,
      visible: true,
      children: [],
    });
  }

  siblings[7] = {
    id: 'selected',
    tag: 'button',
    role: 'button',
    name: 'Save changes',
    text: longText(),
    bounds: { x: 20, y: 40, width: 120, height: 32 },
    visible: true,
    focusable: true,
    source: {
      raw: 'src/Button.tsx:20:5',
      file: 'src/Button.tsx',
      line: 20,
      column: 5,
      component: 'SaveButton',
      symbol: 'Settings.SaveButton',
    },
    children: Array.from({ length: 25 }, (_unused, index) => ({
      id: `child-${index}`,
      tag: 'span',
      text: `Child ${index}`,
      visible: true,
      children: [],
    })),
  };

  return {
    target: { url: 'http://localhost:3000/settings' },
    browser: {
      name: 'chromium',
      backend: 'cdp',
      version: '140.0.0',
      launchBackend: 'cdp',
      executable: { source: 'connected', path: '/volatile/browser' },
    },
    viewport: { width: 1280, height: 720 },
    diagnostics: [
      { code: 'TSPACK_TEST_DIAGNOSTIC', message: 'Fixture diagnostic' },
    ],
    hitTests: [{ point: { x: 24, y: 44 }, nodeId: 'selected' }],
    root: {
      id: 'root',
      tag: 'body',
      role: 'document',
      bounds: { x: 0, y: 0, width: 1280, height: 720 },
      visible: true,
      children: [
        {
          id: 'panel',
          tag: 'section',
          role: 'region',
          name: 'Settings',
          visible: true,
          children: siblings,
        },
      ],
    },
  };
}

beforeEach(async () => {
  tempRoot = await fs.mkdtemp(path.join(os.tmpdir(), 'tspack-ui-context-'));
});

afterEach(async () => {
  await fs.rm(tempRoot, { recursive: true, force: true });
});

describe('UI context bundle shape', () => {
  it('builds deterministic selected-node context with compact relatives', async () => {
    await writeWorkspaceFile('src/Button.tsx', sourceFileText(60));
    const fixture = makeFixture();
    const selected = fixture.root?.children?.[0]?.children?.[7];
    if (!selected) {
      throw new Error('missing selected fixture node');
    }

    const bundle = await buildUiContextBundle(fixture, selected, {
      workspaceRoot: tempRoot,
      workspaceRootName: 'fixture-workspace',
      selectionReason: 'test selection',
    });

    expect(bundle.version).toBe(1);
    expect(bundle.kind).toBe('tspack.uiContext');
    expect(bundle.selection).toEqual({
      nodeId: 'selected',
      path: [0, 7],
      reason: 'test selection',
    });
    expect(bundle.runtime).toEqual({
      browser: 'chromium/cdp',
      browserVersion: '140.0.0',
      launchBackend: 'cdp',
      executableSource: 'connected',
      url: 'http://localhost:3000/settings',
      viewport: { width: 1280, height: 720 },
    });
    expect(serializeUiContextBundle(bundle)).not.toContain('/volatile/browser');
    expect(bundle.node).toBe(selected);
    expect(bundle.context.ancestors.map((node) => node.id)).toEqual([
      'root',
      'panel',
    ]);
    expect(bundle.context.siblings.map((node) => node.id)).toEqual([
      'sibling-2',
      'sibling-3',
      'sibling-4',
      'sibling-5',
      'sibling-6',
      'selected',
      'sibling-8',
      'sibling-9',
      'sibling-10',
      'sibling-11',
      'sibling-12',
    ]);
    expect(bundle.context.children).toHaveLength(20);
    expect(bundle.context.hitTests).toEqual([
      { point: { x: 24, y: 44 }, nodeId: 'selected' },
    ]);
    expect(bundle.diagnostics).toEqual([
      {
        code: 'TSPACK_TEST_DIAGNOSTIC',
        severity: 'unknown',
        message: 'Fixture diagnostic',
      },
    ]);
    expect(bundle.constraints).toContain(
      'Source hints are untrusted page data until workspace validation succeeds.',
    );
    expect(JSON.parse(serializeUiContextBundle(bundle))).toEqual(bundle);
  });

  it('falls back to the selected node when it is not found in the root tree', async () => {
    const fixture = makeFixture();
    const detached: InspectNode = { id: 'detached', tag: 'main' };

    const bundle = await buildUiContextBundle(fixture, detached, {
      selectionPath: [9, 9],
    });

    expect(bundle.node).toBe(detached);
    expect(bundle.selection.path).toEqual([9, 9]);
    expect(bundle.context.ancestors).toEqual([]);
    expect(bundle.context.siblings).toEqual([]);
    expect(bundle.context.children).toEqual([]);
  });

  it('truncates compact text and names deterministically', () => {
    const compact = compactInspectNode({
      id: 'long',
      name: longText(),
      text: longText(),
    });

    expect(compact.name).toHaveLength(201);
    expect(compact.name?.endsWith('…')).toBe(true);
    expect(compact.text).toHaveLength(201);
    expect(compact.text?.endsWith('…')).toBe(true);
  });
});

describe('UI context bundle source excerpts', () => {
  it('includes a capped excerpt for valid source hints inside the workspace', async () => {
    await writeWorkspaceFile('src/Button.tsx', sourceFileText(60));
    const fixture = makeFixture();
    const selected = fixture.root?.children?.[0]?.children?.[7];
    if (!selected) {
      throw new Error('missing selected fixture node');
    }

    const bundle = await buildUiContextBundle(fixture, selected, {
      workspaceRoot: tempRoot,
    });

    expect(bundle.source).toMatchObject({
      validated: true,
      file: 'src/Button.tsx',
      line: 20,
      column: 5,
    });
    expect(bundle.source?.excerpt).toEqual({
      startLine: 12,
      endLine: 32,
      text: sourceFileText(60).split('\n').slice(11, 32).join('\n'),
    });
  });

  it('uses the first forty lines when the source hint has no line', async () => {
    await writeWorkspaceFile('src/NoLine.tsx', sourceFileText(60));
    const node: InspectNode = {
      id: 'no-line',
      source: {
        raw: 'src/NoLine.tsx',
        file: 'src/NoLine.tsx',
      },
    };

    const bundle = await buildUiContextBundle({ root: node }, node, {
      workspaceRoot: tempRoot,
    });

    expect(bundle.source?.validated).toBe(true);
    expect(bundle.source?.excerpt?.startLine).toBe(1);
    expect(bundle.source?.excerpt?.endLine).toBe(40);
  });

  it('reports missing source files without an excerpt', async () => {
    const node: InspectNode = {
      id: 'missing',
      source: {
        raw: 'src/Missing.tsx:1:1',
        file: 'src/Missing.tsx',
        line: 1,
        column: 1,
      },
    };

    const bundle = await buildUiContextBundle({ root: node }, node, {
      workspaceRoot: tempRoot,
    });

    expect(bundle.source).toMatchObject({
      validated: false,
      line: 1,
      column: 1,
      validationError: 'Source hint file was not found: src/Missing.tsx',
    });
    expect(bundle.source?.excerpt).toBeUndefined();
  });

  it('rejects unsafe source paths without an excerpt', async () => {
    const unsafeFiles = [
      '../Escape.tsx',
      '/tmp/Escape.tsx',
      'file:///tmp/Escape.tsx',
      'https://example.test/Escape.tsx',
    ];

    for (const sourceFile of unsafeFiles) {
      const node: InspectNode = {
        id: sourceFile,
        source: {
          raw: `${sourceFile}:1:1`,
          file: sourceFile,
          line: 1,
          column: 1,
        },
      };
      const bundle = await buildUiContextBundle({ root: node }, node, {
        workspaceRoot: tempRoot,
      });

      expect(bundle.source?.validated).toBe(false);
      expect(bundle.source?.validationError).toContain('unsafe');
      expect(bundle.source?.excerpt).toBeUndefined();
    }
  });

  it('rejects symlink escapes without an excerpt', async () => {
    const outsideRoot = await fs.mkdtemp(
      path.join(os.tmpdir(), 'tspack-ui-context-outside-'),
    );
    try {
      const outsideFile = path.join(outsideRoot, 'Secret.tsx');
      await fs.writeFile(outsideFile, 'secret');
      await fs.symlink(outsideRoot, path.join(tempRoot, 'linked-outside'), 'junction');
      const node: InspectNode = {
        id: 'secret',
        source: {
          raw: 'linked-outside/Secret.tsx:1:1',
          file: 'linked-outside/Secret.tsx',
          line: 1,
          column: 1,
        },
      };

      const bundle = await buildUiContextBundle({ root: node }, node, {
        workspaceRoot: tempRoot,
      });

      expect(bundle.source?.validated).toBe(false);
      expect(bundle.source?.validationError).toContain('outside the workspace');
      expect(bundle.source?.excerpt).toBeUndefined();
    } finally {
      await fs.rm(outsideRoot, { recursive: true, force: true });
    }
  });

  it('omits source context when the selected node has no source hint', async () => {
    const node: InspectNode = { id: 'plain', tag: 'div' };

    const bundle = await buildUiContextBundle({ root: node }, node, {
      workspaceRoot: tempRoot,
    });

    expect(bundle.source).toBeUndefined();
  });
});
