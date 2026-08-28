// Code generated from manifest-frontend/src/inspect/context-bundle.ts.
// Run: node tools/generate-vscode-ui-context.mjs
import * as fs from 'node:fs/promises';
import * as path from 'node:path';
import type {
  InspectBounds,
  InspectDiagnostic,
  InspectNode,
  InspectResult,
  InspectSourceHint,
} from './inspectTypes';
import {
  isSourceHintMalformed,
  resolveSourceHintPath,
  type SourcePathResolution,
} from './revealSource';

export type UIContextBundleDiagnostic = {
  code: string;
  severity: string;
  message: string;
  file?: string;
  details?: string[];
};

export type CompactInspectNode = {
  id?: string;
  tag?: string;
  role?: string;
  name?: string;
  text?: string;
  bounds?: InspectBounds;
  visible?: boolean;
  focusable?: boolean;
  source?: {
    component?: string;
    symbol?: string;
  };
};

export type UIContextBundle = {
  version: 1;
  kind: 'tspack.uiContext';
  workspace?: {
    rootName?: string;
  };
  selection: {
    nodeId?: string;
    path: number[];
    reason?: string;
  };
  runtime: {
    browser?: string;
    browserVersion?: string;
    launchBackend?: string;
    executableSource?: string;
    url?: string;
    viewport?: {
      width: number;
      height: number;
    };
  };
  node: InspectNode;
  context: {
    ancestors: CompactInspectNode[];
    siblings: CompactInspectNode[];
    children: CompactInspectNode[];
    hitTests?: unknown[];
  };
  source?: {
    hint?: InspectSourceHint;
    validated: boolean;
    file?: string;
    line?: number;
    column?: number;
    excerpt?: {
      startLine: number;
      endLine: number;
      text: string;
    };
    validationError?: string;
  };
  diagnostics?: UIContextBundleDiagnostic[];
  constraints: string[];
};

type FileSystemAccess = {
  readFile(filePath: string, encoding: 'utf8'): Promise<string>;
  realpath(targetPath: string): Promise<string>;
};

export type UIContextBundleOptions = {
  workspaceRoot?: string;
  workspaceRootName?: string;
  selectionReason?: string;
  selectionPath?: number[];
  diagnostics?: UIContextBundleDiagnostic[];
  fileSystemAccess?: FileSystemAccess;
};

type LocatedNode = {
  node: InspectNode;
  path: number[];
  ancestors: InspectNode[];
  parent?: InspectNode;
  indexInParent?: number;
};

const DEFAULT_SOURCE_LINES_BEFORE = 8;
const DEFAULT_SOURCE_LINES_AFTER = 12;
const DEFAULT_SOURCE_LINES_WITHOUT_HINT = 40;
const MAX_COMPACT_TEXT_LENGTH = 200;
const SIBLINGS_BEFORE = 5;
const SIBLINGS_AFTER = 5;
const CHILDREN_LIMIT = 20;
const DIAGNOSTICS_LIMIT = 20;
const SELECTED_NODE_LIMIT = 250;
const SELECTED_DEPTH_LIMIT = 20;
const MAX_SOURCE_VALUE_LENGTH = 500;
const MAX_EXCERPT_LENGTH = 32_000;
const MAX_SERIALIZED_BUNDLE_LENGTH = 512_000;

const defaultFileSystemAccess: FileSystemAccess = {
  readFile: fs.readFile,
  realpath: fs.realpath,
};

const DEFAULT_CONSTRAINTS = [
  'Source hints are untrusted page data until workspace validation succeeds.',
  'Preserve accessibility role and name unless the requested change intentionally modifies semantics.',
  'Text source remains the source of truth; browser text is an observed runtime fact.',
  'Do not infer package dependencies without TSPack diagnostics or repository evidence.',
  'Browser bounds are observed runtime facts for this viewport only.',
];

function truncateText(value: string | undefined): string | undefined {
  if (value === undefined) {
    return undefined;
  }
  if (value.length <= MAX_COMPACT_TEXT_LENGTH) {
    return value;
  }
  return `${value.slice(0, MAX_COMPACT_TEXT_LENGTH)}…`;
}

function compactSource(source: InspectSourceHint | undefined):
  | {
      component?: string;
      symbol?: string;
    }
  | undefined {
  if (!source) {
    return undefined;
  }
  if (!source.component && !source.symbol) {
    return undefined;
  }
  return {
    component: truncateSourceValue(source.component),
    symbol: truncateSourceValue(source.symbol),
  };
}

export function compactInspectNode(node: InspectNode): CompactInspectNode {
  const compact: CompactInspectNode = {
    id: node.id,
    tag: node.tag,
    role: node.role,
    name: truncateText(node.name),
    text: truncateText(node.text),
    bounds: node.bounds,
    visible: node.visible,
    focusable: node.focusable,
  };
  const source = compactSource(node.source);
  if (source) {
    compact.source = source;
  }
  return compact;
}

function findNodeByReference(
  current: InspectNode,
  selectedNode: InspectNode,
  pathToCurrent: number[],
  ancestors: InspectNode[],
): LocatedNode | undefined {
  if (current === selectedNode) {
    return {
      node: current,
      path: pathToCurrent,
      ancestors,
    };
  }

  const children = Array.isArray(current.children) ? current.children : [];
  for (let index = 0; index < children.length; index += 1) {
    const child = children[index];
    const located = findNodeByReference(
      child,
      selectedNode,
      [...pathToCurrent, index],
      [...ancestors, current],
    );
    if (located) {
      return {
        ...located,
        parent: located.parent ?? current,
        indexInParent: located.indexInParent ?? index,
      };
    }
  }
  return undefined;
}

function findNodeByPath(
  root: InspectNode,
  pathToSelected: number[],
): LocatedNode | undefined {
  let current = root;
  const ancestors: InspectNode[] = [];
  let parent: InspectNode | undefined;
  let indexInParent: number | undefined;

  for (const childIndex of pathToSelected) {
    const children = Array.isArray(current.children) ? current.children : [];
    const child = children[childIndex];
    if (!child) {
      return undefined;
    }
    ancestors.push(current);
    parent = current;
    indexInParent = childIndex;
    current = child;
  }

  return {
    node: current,
    path: pathToSelected,
    ancestors,
    parent,
    indexInParent,
  };
}

function locateSelectedNode(
  inspectResult: InspectResult,
  selectedNode: InspectNode,
  selectionPath: number[] | undefined,
): LocatedNode {
  if (inspectResult.root) {
    const byReference = findNodeByReference(
      inspectResult.root,
      selectedNode,
      [],
      [],
    );
    if (byReference) {
      return byReference;
    }
    if (selectionPath) {
      const byPath = findNodeByPath(inspectResult.root, selectionPath);
      if (byPath) {
        return byPath;
      }
    }
  }

  return {
    node: selectedNode,
    path: selectionPath ?? [],
    ancestors: [],
  };
}

function compactSiblings(location: LocatedNode): CompactInspectNode[] {
  if (!location.parent || location.indexInParent === undefined) {
    return [];
  }
  const siblings = Array.isArray(location.parent.children)
    ? location.parent.children
    : [];
  const start = Math.max(0, location.indexInParent - SIBLINGS_BEFORE);
  const end = Math.min(
    siblings.length,
    location.indexInParent + SIBLINGS_AFTER + 1,
  );
  return siblings.slice(start, end).map((node) => compactInspectNode(node));
}

function compactChildren(node: InspectNode): CompactInspectNode[] {
  const children = Array.isArray(node.children) ? node.children : [];
  return children
    .slice(0, CHILDREN_LIMIT)
    .map((child) => compactInspectNode(child));
}

function compactHitTests(hitTests: unknown[] | undefined): unknown[] {
  return (hitTests ?? []).slice(0, 20).map((hitTest) => {
    if (!hitTest || typeof hitTest !== 'object') {
      return hitTest;
    }
    const candidate = hitTest as {
      point?: unknown;
      elements?: InspectNode[];
    };
    if (!Array.isArray(candidate.elements)) {
      return hitTest;
    }
    return {
      point: candidate.point,
      elements: candidate.elements
        .slice(0, 10)
        .map((element) => compactInspectNode(element)),
    };
  });
}

function normalizeDiagnostics(
  resultDiagnostics: InspectDiagnostic[] | undefined,
  optionDiagnostics: UIContextBundleDiagnostic[] | undefined,
): UIContextBundleDiagnostic[] | undefined {
  const diagnostics: UIContextBundleDiagnostic[] | undefined =
    optionDiagnostics ?? resultDiagnostics?.map((diagnostic) => ({
      code: diagnostic.code,
      severity: 'unknown',
      message: diagnostic.message,
    }));
  if (!diagnostics || diagnostics.length === 0) {
    return undefined;
  }
  return diagnostics.slice(0, DIAGNOSTICS_LIMIT).map((diagnostic) => ({
    code: diagnostic.code,
    severity: diagnostic.severity,
    message: diagnostic.message,
    file: diagnostic.file,
    details: diagnostic.details,
  }));
}

function browserName(result: InspectResult): string | undefined {
  if (result.browser?.name && result.browser.backend) {
    return `${result.browser.name}/${result.browser.backend}`;
  }
  return result.browser?.name ?? result.browser?.backend;
}

function viewport(
  result: InspectResult,
): { width: number; height: number } | undefined {
  const width = result.viewport?.width;
  const height = result.viewport?.height;
  if (typeof width !== 'number' || typeof height !== 'number') {
    return undefined;
  }
  return { width, height };
}

function validationErrorFromResolution(
  resolution: SourcePathResolution,
  sourceFile: string | undefined,
): string {
  if (resolution.ok) {
    return '';
  }
  if (resolution.reason === 'missingPath') {
    return 'Source hint does not include a file path.';
  }
  const displayPath = resolution.displayPath ?? sourceFile ?? '';
  if (resolution.reason === 'unsafePath') {
    return `Source hint path is unsafe: ${displayPath}`.trim();
  }
  if (resolution.reason === 'outsideWorkspace') {
    return `Source hint resolves outside the workspace: ${displayPath}`.trim();
  }
  return `Source hint file was not found: ${displayPath}`.trim();
}

function clampLineNumber(line: number, lineCount: number): number {
  if (!Number.isFinite(line)) {
    return 1;
  }
  const wholeLine = Math.floor(line);
  if (wholeLine < 1) {
    return 1;
  }
  if (wholeLine > lineCount) {
    return lineCount;
  }
  return wholeLine;
}

function buildExcerpt(
  sourceText: string,
  hintedLine: number | undefined,
): {
  startLine: number;
  endLine: number;
  text: string;
} {
  const lines = sourceText.split(/\r?\n/);
  if (lines.length === 0) {
    return {
      startLine: 1,
      endLine: 1,
      text: '',
    };
  }

  if (typeof hintedLine === 'number' && Number.isFinite(hintedLine)) {
    const targetLine = clampLineNumber(hintedLine, lines.length);
    const startLine = Math.max(1, targetLine - DEFAULT_SOURCE_LINES_BEFORE);
    const endLine = Math.min(
      lines.length,
      targetLine + DEFAULT_SOURCE_LINES_AFTER,
    );
    return {
      startLine,
      endLine,
      text: lines
        .slice(startLine - 1, endLine)
        .join('\n')
        .slice(0, MAX_EXCERPT_LENGTH),
    };
  }

  const endLine = Math.min(lines.length, DEFAULT_SOURCE_LINES_WITHOUT_HINT);
  return {
    startLine: 1,
    endLine,
    text: lines.slice(0, endLine).join('\n').slice(0, MAX_EXCERPT_LENGTH),
  };
}

async function buildSourceContext(
  source: InspectSourceHint | undefined,
  options: UIContextBundleOptions,
): Promise<UIContextBundle['source']> {
  if (!source) {
    return undefined;
  }

  const baseSource = {
    hint: boundedSourceHint(source),
    validated: false,
    line: source.line,
    column: source.column,
  };

  if (isSourceHintMalformed(source)) {
    return {
      ...baseSource,
      validationError: source.parseError ?? 'Source hint is malformed.',
    };
  }

  if (!options.workspaceRoot) {
    return {
      ...baseSource,
      validationError: 'No workspace root was provided for source validation.',
    };
  }

  const fileSystemAccess = options.fileSystemAccess ?? defaultFileSystemAccess;
  const resolution = await resolveSourceHintPath(
    options.workspaceRoot,
    source.file,
    fileSystemAccess,
  );
  if (!resolution.ok) {
    return {
      ...baseSource,
      validationError: validationErrorFromResolution(resolution, source.file),
    };
  }

  const sourceText = await fileSystemAccess.readFile(
    resolution.realPath,
    'utf8',
  );
  return {
    ...baseSource,
    validated: true,
    file: resolution.displayPath,
    excerpt: buildExcerpt(sourceText, source.line),
  };
}

export async function buildUIContextBundle(
  inspectResult: InspectResult,
  selectedNode: InspectNode,
  options: UIContextBundleOptions = {},
): Promise<UIContextBundle> {
  const location = locateSelectedNode(
    inspectResult,
    selectedNode,
    options.selectionPath,
  );
  const source = await buildSourceContext(location.node.source, options);
  let diagnostics = normalizeDiagnostics(
    inspectResult.diagnostics,
    options.diagnostics,
  );
  const selected = boundedSelectedNode(location.node);
  if (selected.truncated) {
    const truncationDiagnostic: UIContextBundleDiagnostic = {
      code: 'TSPACK_UI_CONTEXT_TRUNCATED',
      severity: 'warning',
      message: `Selected context exceeded ${SELECTED_NODE_LIMIT} nodes or depth ${SELECTED_DEPTH_LIMIT}.`,
    };
    diagnostics = diagnostics
      ? [...diagnostics, truncationDiagnostic]
      : [truncationDiagnostic];
  }

  const bundle: UIContextBundle = {
    version: 1,
    kind: 'tspack.uiContext',
    selection: {
      nodeId: location.node.id,
      path: location.path,
      reason: options.selectionReason,
    },
    runtime: {
      browser: browserName(inspectResult),
      browserVersion: inspectResult.browser?.version,
      launchBackend: inspectResult.browser?.launchBackend,
      executableSource: inspectResult.browser?.executable?.source,
      url: inspectResult.target?.url,
      viewport: viewport(inspectResult),
    },
    node: selected.node,
    context: {
      ancestors: location.ancestors.map((ancestor) =>
        compactInspectNode(ancestor),
      ),
      siblings: compactSiblings(location),
      children: compactChildren(location.node),
    },
    constraints: [...DEFAULT_CONSTRAINTS],
  };

  if (options.workspaceRootName) {
    bundle.workspace = {
      rootName: options.workspaceRootName,
    };
  }
  if (
    Array.isArray(inspectResult.hitTests) &&
    inspectResult.hitTests.length > 0
  ) {
    bundle.context.hitTests = compactHitTests(inspectResult.hitTests);
  }
  if (source) {
    bundle.source = source;
  }
  if (diagnostics) {
    bundle.diagnostics = diagnostics.slice(0, DIAGNOSTICS_LIMIT);
  }

  return bundle;
}

export function serializeUiContextBundle(bundle: UIContextBundle): string {
  const serialized = `${JSON.stringify(bundle, null, 2)}\n`;
  if (serialized.length > MAX_SERIALIZED_BUNDLE_LENGTH) {
    throw new Error('TSPACK_INSPECT_BUNDLE_TOO_LARGE');
  }
  return serialized;
}

function truncateSourceValue(value: string | undefined): string | undefined {
  if (value === undefined || value.length <= MAX_SOURCE_VALUE_LENGTH) {
    return value;
  }
  return `${value.slice(0, MAX_SOURCE_VALUE_LENGTH)}…`;
}

function boundedSourceHint(source: InspectSourceHint): InspectSourceHint {
  return {
    ...source,
    raw: truncateSourceValue(source.raw),
    file: truncateSourceValue(source.file),
    component: truncateSourceValue(source.component),
    symbol: truncateSourceValue(source.symbol),
    parseError: truncateSourceValue(source.parseError),
  };
}

function boundedSelectedNode(root: InspectNode): {
  node: InspectNode;
  truncated: boolean;
} {
  let nodeCount = 0;
  let truncated = false;

  const clone = (node: InspectNode, depth: number): InspectNode => {
    nodeCount += 1;
    const children: InspectNode[] = [];
    const sourceChildren = Array.isArray(node.children) ? node.children : [];
    if (depth < SELECTED_DEPTH_LIMIT) {
      for (const child of sourceChildren) {
        if (nodeCount >= SELECTED_NODE_LIMIT) {
          truncated = true;
          break;
        }
        children.push(clone(child, depth + 1));
      }
    } else if (sourceChildren.length > 0) {
      truncated = true;
    }

    return {
      ...node,
      name: truncateText(node.name),
      text: truncateText(node.text),
      source: node.source ? boundedSourceHint(node.source) : undefined,
      children,
    };
  };

  return { node: clone(root, 0), truncated };
}

export const buildUiContextBundle = buildUIContextBundle;
