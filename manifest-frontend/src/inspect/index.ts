import fs from 'node:fs';
import path from 'node:path';
import { formatInspectText, formatInspectJson } from './format.js';
import { runInspect, resolveInspectBackend } from './backend.js';
import type { InspectBackendName } from './backend.js';
import { listCdpTargets, normalizeCdpEndpoint } from './cdp.js';
import type { InspectBrowserName, UIInspectResult } from './types.js';
import {
  buildUIContextBundle,
  serializeUiContextBundle,
} from './context-bundle.js';
import { runInspectWatch } from './watch.js';

export {
  createInspectSourceInstrumentation,
  tspackInspectSourceVitePlugin,
} from './source-instrumentation.js';
export type {
  InspectSourceInstrumentation,
  InspectSourceInstrumentationOptions,
  InspectSourceInstrumentationResult,
  TspackVitePlugin,
} from './source-instrumentation.js';

export type InspectOptions = {
  url?: string;
  browser: InspectBackendName;
  hostPath?: string;
  browserPath?: string;
  viewport: { width: number; height: number };
  selector?: string;
  points: Array<{ x: number; y: number }>;
  json: boolean;
  out?: string;
  text?: string;
  root?: string;
  bundle?: boolean;
  bundleOutput?: string;
  cdpEndpoint?: string;
  listTargets?: boolean;
  target?: string;
  targetUrl?: string;
  watch?: boolean;
  watchDebounceMilliseconds?: number;
  verbose?: boolean;
};

function inspectFailureBackend(options: Partial<InspectOptions>): UIInspectResult['browser']['backend'] {
  try {
    const backend = resolveInspectBackend({
      browser: options.browser ?? 'auto',
      viewport: options.viewport ?? { width: 0, height: 0 },
      points: options.points ?? [],
      json: options.json ?? false,
      url: options.url,
      hostPath: options.hostPath,
      browserPath: options.browserPath,
      cdpEndpoint: options.cdpEndpoint,
      selector: options.selector,
      out: options.out,
      text: options.text,
      listTargets: options.listTargets,
      target: options.target,
      targetUrl: options.targetUrl
    });

    switch (backend) {
      case 'cdp':
        return 'cdp';
      case 'vscode':
        return 'vscode';
      case 'browser-path':
        return 'browser-path';
      case 'host-path':
        return 'host-path';
      case 'platform-webview':
        return 'platform-webview';
      default:
        return 'playwright';
    }
  } catch {
    return undefined;
  }
}

function writeFileAtomically(filePath: string, contents: string): void {
  const resolvedPath = path.resolve(filePath);
  const directory = path.dirname(resolvedPath);
  fs.mkdirSync(directory, { recursive: true });
  const temporaryPath = path.join(
    directory,
    `.${path.basename(resolvedPath)}.${Date.now()}.${Math.random().toString(16).slice(2)}.tmp`,
  );

  try {
    fs.writeFileSync(temporaryPath, contents, { encoding: 'utf8', flag: 'wx' });
    fs.renameSync(temporaryPath, resolvedPath);
  } catch (error: unknown) {
    try {
      fs.rmSync(temporaryPath, { force: true });
    } catch {
      // Preserve the original write error.
    }
    throw error;
  }
}

function inspectFailureBrowserName(
  backend: InspectBackendName,
): InspectBrowserName {
  switch (backend) {
    case 'cdp':
      return 'cdp';
    case 'vscode':
      return 'vscode';
    case 'playwright-webkit':
    case 'webkit':
      return 'webkit';
    case 'playwright-chromium':
    case 'chromium':
    case 'browser-path':
    case 'host-path':
      return 'chromium';
    default:
      return 'unknown';
  }
}

function inspectDiagnosticCode(message: string): string {
  const match = /^(TSPACK_[A-Z0-9_]+)/.exec(message.trim());
  if (match) {
    return match[1];
  }
  return 'TSPACK_INSPECT_FAILED';
}

function inspectDiagnosticParts(message: string): {
  message: string;
  details: string[];
  fixes: string[];
} {
  const parts = message
    .split('|')
    .map((value) => value.trim())
    .filter(Boolean);
  if (parts.length === 0) {
    return { message, details: [], fixes: [] };
  }

  const details: string[] = [];
  const fixes: string[] = [];
  for (const part of parts.slice(1)) {
    if (
      part.startsWith('Install ') ||
      part.startsWith('Pass ') ||
      part.startsWith('Run ') ||
      part.startsWith('Use ')
    ) {
      fixes.push(part);
      continue;
    }
    details.push(part);
  }

  return {
    message: parts[0],
    details,
    fixes
  };
}

export function buildInspectFailureResult(
  options: Partial<InspectOptions>,
  error: unknown,
): UIInspectResult {
  const rawMessage = error instanceof Error ? error.message : String(error);
  const backend = resolveInspectBackend({
    browser: options.browser ?? 'auto',
    viewport: options.viewport ?? { width: 0, height: 0 },
    points: options.points ?? [],
    json: options.json ?? false,
    url: options.url,
    hostPath: options.hostPath,
    browserPath: options.browserPath,
    cdpEndpoint: options.cdpEndpoint,
    selector: options.selector,
    out: options.out,
    text: options.text,
    listTargets: options.listTargets,
    target: options.target,
    targetUrl: options.targetUrl
  });
  const diagnostic = inspectDiagnosticParts(rawMessage);

  return {
    target: { url: options.url ?? '' },
    browser: {
      name: inspectFailureBrowserName(backend),
      backend: inspectFailureBackend(options)
    },
    viewport: options.viewport ?? { width: 0, height: 0 },
    root: null,
    hitTests: [],
    diagnostics: [{
      code: inspectDiagnosticCode(rawMessage),
      message: diagnostic.message,
      details: diagnostic.details,
      fixes: diagnostic.fixes
    }]
  };
}

export function parseViewport(input: string): { width: number; height: number } {
  const match = /^(\d+)x(\d+)$/i.exec(input);
  if (!match) throw new Error('TSPACK_INSPECT_INVALID_VIEWPORT');
  const width = Number(match[1]);
  const height = Number(match[2]);
  if (!Number.isInteger(width) || !Number.isInteger(height) || width <= 0 || height <= 0) {
    throw new Error('TSPACK_INSPECT_INVALID_VIEWPORT');
  }
  return { width, height };
}

export function parsePoint(input: string): { x: number; y: number } {
  const match = /^([+-]?\d+(?:\.\d+)?),([+-]?\d+(?:\.\d+)?)$/.exec(input);
  if (!match) throw new Error('TSPACK_INSPECT_INVALID_POINT');
  const x = Number(match[1]);
  const y = Number(match[2]);
  if (!Number.isFinite(x) || !Number.isFinite(y) || x < 0 || y < 0) {
    throw new Error('TSPACK_INSPECT_INVALID_POINT');
  }
  return { x, y };
}

export async function inspectAndWrite(options: InspectOptions): Promise<void> {
  if (options.hostPath && options.browserPath) {
    throw new Error('TSPACK_INSPECT_INVALID_BACKEND_OPTIONS');
  }

  if (options.cdpEndpoint) {
    options.cdpEndpoint = normalizeCdpEndpoint(options.cdpEndpoint);
  }

  if (options.listTargets) {
    if (options.bundle || options.bundleOutput) {
      throw new Error('TSPACK_INSPECT_INVALID_BUNDLE_OPTIONS');
    }
    if (!options.cdpEndpoint) {
      throw new Error('TSPACK_INSPECT_CDP_ENDPOINT_REQUIRED');
    }
    const result = await listCdpTargets(options.cdpEndpoint);
    const jsonOut = `${JSON.stringify(result, null, 2)}\n`;
    const textLines: string[] = [`CDP targets: ${result.endpoint}`, ''];
    for (const item of result.targets) {
      textLines.push(`[${item.index}] ${item.type}`);
      textLines.push(`    title: ${item.title}`);
      textLines.push(`    url: ${item.url}`);
      textLines.push(`    id: ${item.id}`);
      textLines.push('');
    }
    const textOut = `${textLines.join('\n')}\n`;
    process.stdout.write(options.json ? jsonOut : textOut);
    return;
  }

  if (!options.url && !options.cdpEndpoint) throw new Error('TSPACK_INSPECT_TARGET_REQUIRED');
  if (options.watch) {
    if (
      options.json ||
      options.out ||
      options.text ||
      options.listTargets ||
      (options.bundle && !options.bundleOutput)
    ) {
      throw new Error('TSPACK_INSPECT_WATCH_INVALID_OPTIONS');
    }
    const workspaceRoot = path.resolve(options.root ?? process.cwd());
    await runInspectWatch({
      root: workspaceRoot,
      debounceMilliseconds: options.watchDebounceMilliseconds ?? 200,
      inspectOptions: options,
      onChange(changedPaths) {
        const description = changedPaths.length === 1
          ? changedPaths[0]
          : `${changedPaths.length} files`;
        process.stderr.write(`[${watchTimestamp()}] changed ${description}\n`);
      },
      async onCycle(cycle) {
        if (!cycle.result.root) {
          throw new Error('TSPACK_INSPECT_BUNDLE_SELECTION_REQUIRED');
        }
        if (options.bundle || options.bundleOutput) {
          const bundle = await buildUIContextBundle(
            cycle.result,
            cycle.result.root,
            {
              workspaceRoot,
              workspaceRootName: path.basename(workspaceRoot),
              selectionReason: options.selector
                ? `CSS selector: ${options.selector}`
                : 'inspect root',
            },
          );
          const bundleJson = serializeUiContextBundle(bundle);
          if (options.bundleOutput) {
            writeFileAtomically(options.bundleOutput, bundleJson);
          }
        }
        const root = cycle.result.root;
        const summary = [root.role ?? root.tag ?? 'node'];
        if (root.children.length > 0) {
          summary.push(`${root.children.length} children`);
        }
        process.stderr.write(
          `[${watchTimestamp()}] inspection updated (generation ${cycle.generation}, ${summary.join(', ')})\n`,
        );
        if (options.verbose) {
          process.stderr.write(
            `  browser reused: ${cycle.browserReused ? 'yes' : 'no'}\n`,
          );
        }
      },
    });
    return;
  }
  const result = await runInspect(options);
  if (options.bundle || options.bundleOutput) {
    if (!result.root) {
      throw new Error('TSPACK_INSPECT_BUNDLE_SELECTION_REQUIRED');
    }

    const workspaceRoot = path.resolve(options.root ?? process.cwd());
    const bundle = await buildUIContextBundle(result, result.root, {
      workspaceRoot,
      workspaceRootName: path.basename(workspaceRoot),
      selectionReason: options.selector
        ? `CSS selector: ${options.selector}`
        : 'inspect root',
    });
    const bundleJson = serializeUiContextBundle(bundle);
    if (options.bundleOutput) {
      writeFileAtomically(options.bundleOutput, bundleJson);
      process.stderr.write(`Wrote UI context bundle: ${options.bundleOutput}\n`);
      return;
    }
    process.stdout.write(bundleJson);
    return;
  }
  const jsonOut = formatInspectJson(result);
  const textOut = formatInspectText(result);

  if (options.out) {
    fs.writeFileSync(options.out, jsonOut, 'utf8');
  }
  if (options.text) {
    fs.writeFileSync(options.text, textOut, 'utf8');
  }
  process.stdout.write(options.json ? jsonOut : textOut);
}

function watchTimestamp(): string {
  return new Date().toISOString().slice(11, 19);
}
