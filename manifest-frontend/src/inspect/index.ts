import fs from 'node:fs';
import { formatInspectText, formatInspectJson } from './format.js';
import { runInspect } from './backend.js';
import type { InspectBackendName } from './backend.js';

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
  if (!options.url) throw new Error('TSPACK_INSPECT_TARGET_REQUIRED');
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
