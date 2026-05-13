import type { CDPTargetListResult, CDPTargetSummary } from './types.js';

type RawTarget = {
  id?: string;
  type?: string;
  title?: string;
  url?: string;
  webSocketDebuggerUrl?: string;
};

export function normalizeCdpEndpoint(endpoint: string | undefined): string {
  if (!endpoint) {
    throw new Error('TSPACK_INSPECT_CDP_ENDPOINT_REQUIRED');
  }
  let parsed: URL;
  try {
    parsed = new URL(endpoint);
  } catch {
    throw new Error('TSPACK_INSPECT_CDP_ENDPOINT_INVALID');
  }
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    throw new Error('TSPACK_INSPECT_CDP_ENDPOINT_INVALID');
  }
  return parsed.origin;
}

async function fetchJson<T>(endpoint: string, suffix: string): Promise<T> {
  const url = `${endpoint}${suffix}`;
  let response: Response;
  try {
    response = await fetch(url);
  } catch {
    throw new Error('TSPACK_INSPECT_CDP_CONNECT_FAILED');
  }
  if (!response.ok) {
    throw new Error('TSPACK_INSPECT_CDP_CONNECT_FAILED');
  }
  return response.json() as Promise<T>;
}

function isInspectableTarget(target: RawTarget): boolean {
  return target.type === 'page' || target.type === 'webview';
}

export async function listCdpTargets(endpoint: string): Promise<CDPTargetListResult> {
  const rawTargets = await fetchJson<RawTarget[]>(endpoint, '/json/list');
  const targets: CDPTargetSummary[] = [];
  for (const target of rawTargets) {
    if (!isInspectableTarget(target)) {
      continue;
    }
    targets.push({
      index: targets.length,
      id: target.id ?? '',
      type: target.type ?? 'unknown',
      title: target.title ?? '',
      url: target.url ?? '',
      webSocketDebuggerUrl: target.webSocketDebuggerUrl
    });
  }
  return { endpoint, targets, diagnostics: [] };
}

