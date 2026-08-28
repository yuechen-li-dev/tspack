import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import {
  buildUIContextBundle,
  serializeUiContextBundle,
} from '../src/inspect/context-bundle.js';
import { resolveSourceHintPath } from '../src/inspect/source-path.js';
import type {
  UIInspectNode,
  UIInspectResult,
} from '../src/inspect/types.js';

function node(
  id: string,
  overrides: Partial<UIInspectNode> = {},
): UIInspectNode {
  return {
    id,
    tag: 'div',
    bounds: { x: 0, y: 0, width: 100, height: 20 },
    visible: true,
    children: [],
    ...overrides,
  };
}

function inspection(root: UIInspectNode): UIInspectResult {
  return {
    target: { url: 'http://127.0.0.1:5173' },
    browser: {
      name: 'chromium',
      backend: 'playwright',
      launchBackend: 'playwright-chromium',
      version: '140.0.0',
      executable: {
        source: 'system',
        path: '/machine-specific/chromium',
      },
    },
    viewport: { width: 800, height: 600 },
    root,
    hitTests: [],
    diagnostics: [],
  };
}

describe('UI context bundle', () => {
  it('builds deterministic selector-sized context without executable paths', async () => {
    const selected = node('dismiss', {
      tag: 'button',
      role: 'button',
      name: 'Dismiss',
      focusable: false,
      source: {
        raw: 'src/toast.tsx:25:7',
        file: 'src/toast.tsx',
        line: 25,
        column: 7,
        component: 'Toast',
        symbol: 'Toast.DismissButton',
      },
    });
    const root = node('alert', {
      role: 'alert',
      name: 'Save failed',
      children: [selected],
    });

    const first = await buildUIContextBundle(inspection(root), selected, {
      selectionReason: 'nested dismiss button',
    });
    const second = await buildUIContextBundle(inspection(root), selected, {
      selectionReason: 'nested dismiss button',
    });

    expect(first).toEqual(second);
    expect(first).toMatchObject({
      version: 1,
      kind: 'tspack.uiContext',
      selection: { nodeId: 'dismiss', path: [0] },
      runtime: {
        browser: 'chromium/playwright',
        browserVersion: '140.0.0',
        launchBackend: 'playwright-chromium',
        executableSource: 'system',
      },
      node: {
        role: 'button',
        name: 'Dismiss',
        focusable: false,
      },
      source: {
        validated: false,
        validationError: 'No workspace root was provided for source validation.',
      },
    });
    expect(serializeUiContextBundle(first)).toBe(
      serializeUiContextBundle(second),
    );
    expect(serializeUiContextBundle(first)).not.toContain(
      '/machine-specific/chromium',
    );
  });

  it('validates and reads only bounded workspace-contained source context', async () => {
    const workspace = fs.mkdtempSync(path.join(os.tmpdir(), 'tspack-bundle-'));
    fs.mkdirSync(path.join(workspace, 'src'));
    fs.writeFileSync(
      path.join(workspace, 'src', 'toast.tsx'),
      Array.from({ length: 60 }, (_, index) => `line ${index + 1}`).join('\n'),
    );
    const selected = node('toast', {
      source: {
        raw: 'src/toast.tsx:25:2',
        file: 'src/toast.tsx',
        line: 25,
        column: 2,
      },
    });

    const bundle = await buildUIContextBundle(inspection(selected), selected, {
      workspaceRoot: workspace,
      workspaceRootName: 'fixture',
    });

    expect(bundle.source).toMatchObject({
      validated: true,
      file: 'src/toast.tsx',
      line: 25,
      column: 2,
      excerpt: {
        startLine: 17,
        endLine: 37,
      },
    });
    expect(bundle.workspace).toEqual({ rootName: 'fixture' });
  });

  it('retains malformed source evidence without attempting a source read', async () => {
    const selected = node('malformed', {
      source: {
        raw: 'src/Toast.tsx:line',
        parseError: 'invalid source line or column',
        component: 'Toast',
      },
    });

    const bundle = await buildUIContextBundle(inspection(selected), selected, {
      workspaceRoot: os.tmpdir(),
    });

    expect(bundle.source).toMatchObject({
      validated: false,
      hint: {
        raw: 'src/Toast.tsx:line',
        parseError: 'invalid source line or column',
        component: 'Toast',
      },
      validationError: 'invalid source line or column',
    });
  });

  it.each([
    '../../secret',
    '/etc/passwd',
    'C:\\outside\\file.tsx',
    'file:///etc/passwd',
    'https://evil.example/x.tsx',
  ])('rejects hostile source path %s', async (sourceFile) => {
    const workspace = fs.mkdtempSync(path.join(os.tmpdir(), 'tspack-source-'));
    const resolution = await resolveSourceHintPath(workspace, sourceFile);
    expect(resolution.ok).toBe(false);
    if (!resolution.ok) {
      expect(['unsafePath', 'outsideWorkspace']).toContain(resolution.reason);
    }
  });

  it('rejects a symlink that escapes the workspace where supported', async () => {
    const workspace = fs.mkdtempSync(path.join(os.tmpdir(), 'tspack-source-'));
    const outside = fs.mkdtempSync(path.join(os.tmpdir(), 'tspack-outside-'));
    fs.writeFileSync(path.join(outside, 'secret.tsx'), 'secret');
    const link = path.join(workspace, 'linked');
    try {
      fs.symlinkSync(outside, link, process.platform === 'win32' ? 'junction' : 'dir');
    } catch {
      return;
    }

    const resolution = await resolveSourceHintPath(
      workspace,
      'linked/secret.tsx',
    );
    expect(resolution).toMatchObject({
      ok: false,
      reason: 'outsideWorkspace',
    });
  });

  it('bounds large selected trees and source attributes with a truncation diagnostic', async () => {
    const hugeSourceValue = 'x'.repeat(10_000);
    const root = node('large-root', {
      source: {
        raw: hugeSourceValue,
        file: hugeSourceValue,
        component: hugeSourceValue,
      },
      children: Array.from({ length: 400 }, (_, index) =>
        node(`child-${index}`, {
          text: hugeSourceValue,
          source: { raw: hugeSourceValue, component: hugeSourceValue },
        }),
      ),
    });

    const bundle = await buildUIContextBundle(inspection(root), root);
    const serialized = serializeUiContextBundle(bundle);

    expect(bundle.node.children).toHaveLength(249);
    expect(bundle.node.source?.raw?.length).toBeLessThan(600);
    expect(bundle.source?.hint?.raw?.length).toBeLessThan(600);
    expect(bundle.diagnostics).toContainEqual(
      expect.objectContaining({ code: 'TSPACK_UI_CONTEXT_TRUNCATED' }),
    );
    expect(serialized.length).toBeLessThan(512_000);
  });
});
