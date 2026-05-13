import fs from 'node:fs';
import { formatInspectText, formatInspectJson } from './format.js';
import { runInspect } from './backend.js';
import type { InspectBackendName } from './backend.js';
import { listCdpTargets, normalizeCdpEndpoint } from './cdp.js';

export type InspectOptions = {
  url?: string;
  browser: InspectBackendName;
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
