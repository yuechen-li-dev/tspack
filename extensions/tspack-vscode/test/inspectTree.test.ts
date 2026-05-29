import { describe, expect, it } from 'vitest';
import {
  buildInspectNodeDescription,
  buildInspectNodeLabel,
  buildInspectNodeTooltip,
  buildInspectTree,
  serializeInspectNode,
} from '../src/inspectTree';
import {
  buildInspectTargetCommand,
  buildListTargetsCommand,
  TspackCliError,
} from '../src/tspackCli';
import type { InspectResult } from '../src/inspectTypes';

const fixture: InspectResult = {
  target: { url: 'vscode-file://vscode-app/workbench.html' },
  browser: { name: 'cdp', backend: 'cdp' },
  viewport: { width: 1200, height: 800 },
  diagnostics: [],
  hitTests: [],
  root: {
    id: 'root',
    tag: 'body',
    role: 'document',
    bounds: { x: 0, y: 0, width: 1200, height: 800 },
    visible: true,
    children: [
      {
        id: 'notifications',
        tag: 'button',
        role: 'button',
        name: 'Notifications',
        bounds: { x: 1140, y: 742, width: 36, height: 28 },
        visible: true,
        focusable: true,
        source: {
          raw: 'src/components/Notifications.tsx:42:7',
          file: 'src/components/Notifications.tsx',
          line: 42,
          column: 7,
          component: 'NotificationsButton',
          symbol: 'Notifications.Button',
        },
        style: {
          display: 'flex',
          position: 'relative',
        },
        children: [],
      },
      {
        id: 'statusbar',
        tag: 'div',
        bounds: { x: 0, y: 770, width: 1200, height: 30 },
        visible: true,
        children: [],
      },
    ],
  },
};

describe('inspect tree conversion', () => {
  it('converts fixture inspect JSON into tree nodes', () => {
    const tree = buildInspectTree(fixture);
    expect(tree).toHaveLength(1);
    expect(tree[0].label).toBe('document');
    expect(tree[0].description).toBe('0,0,1200,800 · visible');
    expect(tree[0].children).toHaveLength(2);
  });

  it('prefers role plus accessible name for labels', () => {
    const tree = buildInspectTree(fixture);
    expect(tree[0].children[0].label).toBe('button "Notifications"');
  });

  it('falls back to tag labels', () => {
    const tree = buildInspectTree(fixture);
    expect(tree[0].children[1].label).toBe('div');
    expect(buildInspectNodeLabel({ text: 'Only text' })).toBe('"Only text"');
  });

  it('describes compact bounds and focusability', () => {
    const node = fixture.root?.children?.[0];
    if (!node) {
      throw new Error('missing fixture node');
    }
    expect(buildInspectNodeDescription(node)).toBe('1140,742,36,28 · visible · focusable');
  });

  it('serializes selected node JSON exactly through JSON parsing', () => {
    const node = fixture.root?.children?.[0];
    if (!node) {
      throw new Error('missing fixture node');
    }
    expect(JSON.parse(serializeInspectNode(node))).toEqual(node);
  });

  it('includes source hint metadata in tooltips', () => {
    const node = fixture.root?.children?.[0];
    if (!node) {
      throw new Error('missing fixture node');
    }

    const tooltip = buildInspectNodeTooltip(node);

    expect(tooltip).toContain('sourceFile: src/components/Notifications.tsx');
    expect(tooltip).toContain('sourceLine: 42');
    expect(tooltip).toContain('component: NotificationsButton');
    expect(tooltip).toContain('symbol: Notifications.Button');
  });
});

describe('tspack cli command construction', () => {
  it('builds list-targets JSON command args', () => {
    expect(buildListTargetsCommand('tspack', 'http://127.0.0.1:9229')).toEqual({
      command: 'tspack',
      args: ['inspect', '--cdp', 'http://127.0.0.1:9229', '--list-targets', '--json'],
    });
  });

  it('builds inspect target JSON command args', () => {
    expect(buildInspectTargetCommand('/bin/tspack', 'http://127.0.0.1:9229', 2)).toEqual({
      command: '/bin/tspack',
      args: ['inspect', '--cdp', 'http://127.0.0.1:9229', '--target', '2', '--json'],
    });
  });

  it('represents missing binary as a user-facing error code', () => {
    const error = new TspackCliError(
      'TSPACK_CLI_NOT_FOUND',
      'TSPack CLI not found. Set tspack.inspect.tspackPath.',
    );
    expect(error.code).toBe('TSPACK_CLI_NOT_FOUND');
    expect(error.message).toContain('tspack.inspect.tspackPath');
  });
});
