import fs from 'node:fs';
import type { InspectOptions } from './index.js';
import { INSPECT_ANALYZER_SCRIPT } from './analyzer.js';
import type { InspectBrowserName, UIInspectResult } from './types.js';

export type InspectBackendName = 'auto' | 'vscode' | 'playwright-chromium' | 'browser-path' | 'chromium';

export type InspectBackendProbe = {
  executablePath?: string;
  reason?: string;
};

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

export async function probeVSCodeElectronBackend(): Promise<InspectBackendProbe> {
  const executablePath = findVSCodeExecutable();
  if (!executablePath) {
    return { reason: 'TSPACK_INSPECT_VSCODE_NOT_FOUND' };
  }

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

export async function runInspect(options: InspectOptions): Promise<UIInspectResult> {
  if (!/^https?:\/\//i.test(options.url ?? '')) {
    throw new Error('TSPACK_INSPECT_INVALID_TARGET');
  }

  const browserPath = options.browserPath;
  const backend = options.browser;

  if (backend === 'playwright-chromium' || backend === 'chromium') {
    return inspectWithChromium(options, { browserName: 'chromium' });
  }

  if (backend === 'browser-path') {
    if (!browserPath || !fs.existsSync(browserPath)) {
      throw new Error('TSPACK_INSPECT_BROWSER_PATH_NOT_FOUND');
    }
    return inspectWithChromium(options, { executablePath: browserPath, browserName: 'chromium' });
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
