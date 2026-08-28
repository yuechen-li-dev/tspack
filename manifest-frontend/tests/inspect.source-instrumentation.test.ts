import path from 'node:path';
import ts from 'typescript';
import { describe, expect, it } from 'vitest';
import {
  createInspectSourceInstrumentation,
  tspackInspectSourceVitePlugin,
} from '../src/inspect/source-instrumentation.js';

const workspaceRoot = path.resolve('test-workspace');
const sourcePath = path.join(workspaceRoot, 'src', 'Toast.tsx');

function instrument(source: string) {
  return createInspectSourceInstrumentation({
    workspaceRoot,
    enabled: true,
  }).instrument(source, sourcePath);
}

describe('inspect source instrumentation', () => {
  it('injects workspace-relative locations on intrinsic nodes', () => {
    const result = instrument(`function Toast() {
  return <div role="alert"><button>Dismiss</button></div>;
}`);

    expect(result.injectedNodeCount).toBe(2);
    expect(result.code).toContain('data-tspack-source="src/Toast.tsx:2:10"');
    expect(result.code).toContain('data-tspack-source="src/Toast.tsx:2:28"');
    expect(result.code).toContain('data-tspack-component="Toast"');
    expect(result.code).not.toContain(slashPath(workspaceRoot));
  });

  it('keeps manual metadata authoritative without duplicate attributes', () => {
    const result = instrument(`export const Toast = () => (
  <div data-tspack-source="manual.tsx:7:3" {...props} />
);`);

    expect(result.injectedNodeCount).toBe(0);
    expect(result.code.match(/data-tspack-source/g)).toHaveLength(1);
    expect(result.code).toContain('manual.tsx:7:3');
  });

  it('appends generated metadata after spreads without changing expression count', () => {
    const result = instrument(`function Toast() {
  return <div before={sideEffect()} {...props} after={otherEffect()} />;
}`);

    expect(result.code.match(/sideEffect\(\)/g)).toHaveLength(1);
    expect(result.code.match(/otherEffect\(\)/g)).toHaveLength(1);
    expect(result.code.indexOf('{...props}')).toBeLessThan(
      result.code.indexOf('data-tspack-source'),
    );
  });

  it('does not instrument custom components or dependencies', () => {
    const custom = instrument(`function App() { return <ThirdParty />; }`);
    expect(custom.injectedNodeCount).toBe(0);

    const dependencyPath = path.join(
      workspaceRoot,
      'node_modules',
      'dependency',
      'index.tsx',
    );
    const dependency = createInspectSourceInstrumentation({
      workspaceRoot,
      enabled: true,
    }).instrument('export const View = () => <div />;', dependencyPath);
    expect(dependency.injectedNodeCount).toBe(0);
  });

  it('records the logical workspace path supplied by a symlinked source host', () => {
    const logicalPath = path.join(workspaceRoot, 'linked-src', 'View.tsx');
    const result = createInspectSourceInstrumentation({
      workspaceRoot,
      enabled: true,
    }).instrument('export const View = () => <main />;', logicalPath);

    expect(result.code).toContain('linked-src/View.tsx:1:27');
    expect(result.code).not.toContain(workspaceRoot.replace(/\\/g, '/'));
  });

  it('preserves a clear local JSX symbol without guessing from labels', () => {
    const result = instrument(`function Toolbar() {
  const PrimaryAction = <button>Save</button>;
  return <div>{PrimaryAction}</div>;
}`);

    expect(result.code).toContain('data-tspack-symbol="PrimaryAction"');
    expect(result.code).not.toContain('data-tspack-symbol="Save"');
  });

  it('is absent when build intent disables instrumentation', () => {
    const source = 'export const View = () => <div />;';
    const result = createInspectSourceInstrumentation({
      workspaceRoot,
      enabled: false,
    }).instrument(source, sourcePath);
    expect(result).toEqual({ code: source, injectedNodeCount: 0 });
    expect(result.code).not.toContain('data-tspack-');
  });

  it('returns a useful source map to the original TSX file', () => {
    const result = instrument(`type ToastProps = { message: string };
export function Toast(props: ToastProps) {
  return <div>{props.message}</div>;
}`);
    const sourceMap = JSON.parse(result.map ?? '{}') as ts.RawSourceMap;

    expect(sourceMap.sources).toContain('Toast.tsx');
    expect(sourceMap.sourcesContent?.[0]).toContain('ToastProps');
  });

  it('treats authored source movement as a deterministic provenance change', () => {
    const before = instrument('export const View = () => <main />;');
    const after = instrument('\nexport const View = () => <main />;');

    expect(before.code).toContain('src/Toast.tsx:1:27');
    expect(after.code).toContain('src/Toast.tsx:2:27');
  });

  it('exposes a development-only Vite adapter over the neutral seam', () => {
    const plugin = tspackInspectSourceVitePlugin({ workspaceRoot });
    expect(plugin.apply).toBe('serve');
    expect(plugin.enforce).toBe('pre');
    expect(plugin.transform('<div />', sourcePath)?.injectedNodeCount).toBe(1);
  });
});

function slashPath(value: string): string {
  return value.replace(/\\/g, '/');
}
