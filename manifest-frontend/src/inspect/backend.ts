import fs from 'node:fs';
import path from 'node:path';
import type { InspectOptions } from './index.js';
import { INSPECT_ANALYZER_SCRIPT } from './analyzer.js';
import type { InspectBrowserName, UIInspectResult, CDPTargetSummary } from './types.js';
import { listCdpTargets } from './cdp.js';
import { launchInspectableHost, validateHostPath } from './host-launch.js';

export type InspectBackendName = 'auto' | 'vscode' | 'playwright-chromium' | 'browser-path' | 'host-path' | 'chromium' | 'cdp' | 'platform-webview';

export type InspectBackendProbe = {
  executablePath?: string;
  reason?: string;
};


type PlatformWebViewProbeResult = {
  os: NodeJS.Platform;
  candidate: 'webview2' | 'wkwebview' | 'webkitgtk';
  checks: string[];
  blocker?: string;
  outcome: 'usable' | 'not-usable' | 'unavailable';
};

function probePlatformWebViewEnvironment(): PlatformWebViewProbeResult {
  const os = process.platform;

  if (os === 'win32') {
    const checks = ['platform=windows', 'expected-engine=WebView2'];
    return {
      os,
      candidate: 'webview2',
      checks,
      blocker: 'TSPACK_INSPECT_PLATFORM_WEBVIEW_UNAVAILABLE: WebView2 probe is not implemented yet in Node-only backend.',
      outcome: 'not-usable'
    };
  }

  if (os === 'darwin') {
    const checks = ['platform=macos', 'expected-engine=WKWebView'];
    return {
      os,
      candidate: 'wkwebview',
      checks,
      blocker: 'TSPACK_INSPECT_PLATFORM_WEBVIEW_UNAVAILABLE: WKWebView probe is not implemented yet in Node-only backend.',
      outcome: 'not-usable'
    };
  }

  const checks: string[] = ['platform=linux', 'expected-engine=WebKitGTK'];
  const hasDisplay = Boolean(process.env.DISPLAY || process.env.WAYLAND_DISPLAY);
  checks.push(`display=${hasDisplay ? 'present' : 'missing'}`);

  if (!hasDisplay) {
    return {
      os,
      candidate: 'webkitgtk',
      checks,
      blocker: 'TSPACK_INSPECT_PLATFORM_WEBVIEW_UNAVAILABLE: missing DISPLAY/WAYLAND_DISPLAY for WebKitGTK runtime session.',
      outcome: 'unavailable'
    };
  }

  return {
    os,
    candidate: 'webkitgtk',
    checks,
    blocker: 'TSPACK_INSPECT_PLATFORM_WEBVIEW_INIT_FAILED: WebKitGTK backend scaffold exists but runtime host is not wired yet.',
    outcome: 'not-usable'
  };
}

export function findVSCodeExecutable(): string | null {
  const candidates = ['code', 'code-insiders', 'codium'];
  const pathParts = (process.env.PATH ?? '').split(':').filter(Boolean);
  for (const name of candidates) {
    for (const part of pathParts) {
      const fullPath = `${part}/${name}`;
      if (fs.existsSync(fullPath)) {
        return fullPath;
      }
    }
  }
  return null;
}

export function resolveVSCodeElectronExecutable(wrapperPath: string): string {
  const wrapperName = path.basename(wrapperPath);
  const variants: Record<string, string[]> = {
    code: ['/usr/share/code/code'],
    'code-insiders': ['/usr/share/code-insiders/code-insiders'],
    codium: ['/usr/share/codium/codium'],
    'code-oss': ['/usr/share/code-oss/code-oss'],
    vscodium: ['/usr/share/vscodium/vscodium']
  };

  const wrapperDirCandidate = path.resolve(path.dirname(wrapperPath), '..', 'share', wrapperName, wrapperName);
  const knownCandidates = variants[wrapperName] ?? [];
  const candidates = [wrapperDirCandidate, ...knownCandidates, wrapperPath];

  for (const candidate of candidates) {
    if (!fs.existsSync(candidate)) {
      continue;
    }
    const stat = fs.statSync(candidate);
    if (stat.isFile()) {
      return candidate;
    }
  }

  return wrapperPath;
}

export async function probeVSCodeElectronBackend(): Promise<InspectBackendProbe> {
  const wrapperPath = findVSCodeExecutable();
  if (!wrapperPath) {
    return { reason: 'TSPACK_INSPECT_VSCODE_NOT_FOUND' };
  }
  const executablePath = resolveVSCodeElectronExecutable(wrapperPath);

  try {
    const playwright = await import('playwright');
    const browser = await playwright.chromium.launch({ executablePath, headless: true });
    await browser.close();
    return { executablePath };
  } catch (error: unknown) {
    const message = error instanceof Error ? error.message : String(error);
    return { executablePath, reason: `TSPACK_INSPECT_VSCODE_ELECTRON_NOT_USABLE: ${message}` };
  }
}

async function inspectWithChromium(options: InspectOptions, overrides?: { executablePath?: string; browserName?: InspectBrowserName }): Promise<UIInspectResult> {
  let playwright: typeof import('playwright');
  try {
    playwright = await import('playwright');
  } catch {
    throw new Error('TSPACK_INSPECT_BROWSER_LAUNCH_FAILED');
  }

  const browser = await playwright.chromium.launch({ executablePath: overrides?.executablePath });
  try {
    const page = await browser.newPage({ viewport: { width: options.viewport.width, height: options.viewport.height } });
    await page.goto(options.url as string, { waitUntil: 'load' });

    const payload = await page.evaluate(INSPECT_ANALYZER_SCRIPT, { selector: options.selector, points: options.points });
    if (options.selector && !payload.root) {
      throw new Error('TSPACK_INSPECT_SELECTOR_NOT_FOUND');
    }

    return {
      target: { url: options.url as string },
      browser: { name: overrides?.browserName ?? 'chromium' },
      viewport: { width: options.viewport.width, height: options.viewport.height },
      root: payload.root,
      hitTests: payload.hitTests,
      diagnostics: []
    };
  } catch (error: unknown) {
    if (error instanceof Error && error.message.startsWith('TSPACK_INSPECT_')) {
      throw error;
    }
    throw new Error('TSPACK_INSPECT_PAGE_LOAD_FAILED');
  } finally {
    await browser.close();
  }
}


function selectCdpTarget(targets: CDPTargetSummary[], targetInput?: string, targetUrlSubstring?: string): CDPTargetSummary {
  if (targetInput) {
    const asIndex = Number(targetInput);
    if (Number.isInteger(asIndex)) {
      const target = targets.find((item) => item.index === asIndex);
      if (!target) throw new Error('TSPACK_INSPECT_CDP_TARGET_NOT_FOUND');
      return target;
    }
    const byId = targets.find((item) => item.id === targetInput);
    if (!byId) throw new Error('TSPACK_INSPECT_CDP_TARGET_NOT_FOUND');
    return byId;
  }

  if (targetUrlSubstring) {
    const matches = targets.filter((item) => item.url.includes(targetUrlSubstring));
    if (matches.length === 0) throw new Error('TSPACK_INSPECT_CDP_TARGET_NOT_FOUND');
    if (matches.length > 1) throw new Error('TSPACK_INSPECT_CDP_TARGET_AMBIGUOUS');
    return matches[0];
  }

  if (targets.length === 0) throw new Error('TSPACK_INSPECT_CDP_TARGET_NOT_FOUND');
  return targets[0];
}

async function inspectWithCdp(options: InspectOptions): Promise<UIInspectResult> {
  let playwright: typeof import('playwright');
  try {
    playwright = await import('playwright');
  } catch {
    throw new Error('TSPACK_INSPECT_CDP_CONNECT_FAILED');
  }

  let browser: Awaited<ReturnType<typeof playwright.chromium.connectOverCDP>> | undefined;
  try {
    const list = await listCdpTargets(options.cdpEndpoint as string);
    const target = selectCdpTarget(list.targets, options.target, options.targetUrl);
    if (!target.webSocketDebuggerUrl) {
      throw new Error('TSPACK_INSPECT_CDP_TARGET_UNSUPPORTED');
    }

    browser = await playwright.chromium.connectOverCDP(options.cdpEndpoint as string);
    const contexts = browser.contexts();
    const pages = contexts.flatMap((context) => context.pages());
    let page = pages.find((candidate) => candidate.url() === target.url);
    if (!page) {
      page = pages[0];
    }
    if (!page) {
      throw new Error('TSPACK_INSPECT_CDP_TARGET_NOT_FOUND');
    }

    if (options.url) {
      await page.goto(options.url, { waitUntil: 'load' });
    }

    if (options.viewport.width > 0 && options.viewport.height > 0) {
      await page.setViewportSize({ width: options.viewport.width, height: options.viewport.height });
    }

    const payload = await page.evaluate(INSPECT_ANALYZER_SCRIPT, { selector: options.selector, points: options.points });
    if (options.selector && !payload.root) {
      throw new Error('TSPACK_INSPECT_SELECTOR_NOT_FOUND');
    }

    return {
      target: { url: options.url ?? page.url() },
      browser: { name: 'cdp', backend: 'cdp' },
      viewport: { width: options.viewport.width, height: options.viewport.height },
      root: payload.root,
      hitTests: payload.hitTests,
      diagnostics: []
    };
  } catch (error: unknown) {
    if (error instanceof Error && error.message.startsWith('TSPACK_INSPECT_')) {
      throw error;
    }
    throw new Error('TSPACK_INSPECT_CDP_EVALUATION_FAILED');
  } finally {
    await browser?.close();
  }
}
export async function runInspect(options: InspectOptions): Promise<UIInspectResult> {
  if (options.url && !/^https?:\/\//i.test(options.url)) {
    throw new Error('TSPACK_INSPECT_INVALID_TARGET');
  }

  const hostPath = options.hostPath;
  const browserPath = options.browserPath;
  const backend = options.browser;

  if (hostPath && browserPath) {
    throw new Error('TSPACK_INSPECT_INVALID_BACKEND_OPTIONS');
  }

  if (options.cdpEndpoint && (hostPath || browserPath)) {
    throw new Error('TSPACK_INSPECT_INVALID_BACKEND_OPTIONS');
  }

  if (options.cdpEndpoint && backend !== 'cdp') {
    throw new Error('TSPACK_INSPECT_INVALID_BACKEND_OPTIONS');
  }

  if ((hostPath || browserPath) && backend !== 'auto' && backend !== 'host-path' && backend !== 'browser-path') {
    throw new Error('TSPACK_INSPECT_INVALID_BACKEND_OPTIONS');
  }

  if (backend === 'platform-webview') {
    const probe = probePlatformWebViewEnvironment();
    const checks = probe.checks.join(',');
    const blocker = probe.blocker ?? 'TSPACK_INSPECT_PLATFORM_WEBVIEW_UNAVAILABLE';
    throw new Error(`${blocker} [${checks}]`);
  }

  if (backend === 'cdp') {
    return inspectWithCdp(options);
  }

  if (backend === 'playwright-chromium' || backend === 'chromium') {
    return inspectWithChromium(options, { browserName: 'chromium' });
  }

  if (backend === 'browser-path') {
    if (!browserPath || !fs.existsSync(browserPath)) {
      throw new Error('TSPACK_INSPECT_BROWSER_PATH_NOT_FOUND');
    }
    return inspectWithChromium(options, { executablePath: browserPath, browserName: 'chromium' });
  }

  if (backend === 'host-path' || hostPath || browserPath) {
    const explicitHostPath = hostPath ?? browserPath;
    if (!explicitHostPath) {
      throw new Error('TSPACK_INSPECT_HOST_PATH_NOT_FOUND');
    }
    validateHostPath(explicitHostPath);
    const launchedHost = await launchInspectableHost({ executablePath: explicitHostPath });
    try {
      return await inspectWithCdp({
        ...options,
        cdpEndpoint: launchedHost.endpoint,
        browser: 'cdp'
      });
    } catch (error: unknown) {
      if (error instanceof Error && error.message.startsWith('TSPACK_INSPECT_')) {
        throw error;
      }
      throw new Error('TSPACK_INSPECT_HOST_CDP_ENDPOINT_FAILED');
    } finally {
      await launchedHost.cleanup();
    }
  }

  if (backend === 'vscode') {
    const probe = await probeVSCodeElectronBackend();
    if (!probe.executablePath) {
      throw new Error(probe.reason ?? 'TSPACK_INSPECT_VSCODE_NOT_FOUND');
    }
    if (probe.reason) {
      throw new Error(probe.reason);
    }
    return inspectWithChromium(options, { executablePath: probe.executablePath, browserName: 'chromium' });
  }

  if (backend === 'auto') {
    const probe = await probeVSCodeElectronBackend();
    if (probe.executablePath && !probe.reason) {
      return inspectWithChromium(options, { executablePath: probe.executablePath, browserName: 'chromium' });
    }
    try {
      return await inspectWithChromium(options, { browserName: 'chromium' });
    } catch {
      throw new Error(probe.reason ?? 'TSPACK_INSPECT_BROWSER_LAUNCH_FAILED');
    }
  }

  throw new Error('TSPACK_INSPECT_BROWSER_UNSUPPORTED');
}
