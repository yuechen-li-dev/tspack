import fs from 'node:fs';
import { formatInspectText, formatInspectJson } from './format.js';
import { runInspect, resolveInspectBackend } from './backend.js';
import type { InspectBackendName } from './backend.js';
import { listCdpTargets, normalizeCdpEndpoint } from './cdp.js';
import type { InspectBrowserName, UIInspectResult } from './types.js';

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
  cdpEndpoint?: string;
  listTargets?: boolean;
  target?: string;
  targetUrl?: string;
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
  const result = await runInspect(options);
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
