import fs from 'node:fs';
import path from 'node:path';
import { buildInspectAnalyzerExpression, findSystemChromiumExecutable } from './backend.js';
import type { InspectOptions } from './index.js';
import type { UIInspectResult } from './types.js';

export enum InspectWatchState {
  Idle = 'Idle',
  Inspecting = 'Inspecting',
  Dirty = 'Dirty',
  WaitingForTarget = 'WaitingForTarget',
  FailedTransiently = 'FailedTransiently',
  Stopped = 'Stopped',
}

export type InspectWatchCycle = {
  generation: number;
  changedPaths: string[];
  result: UIInspectResult;
  browserReused: boolean;
};

export type InspectWatchOptions = {
  root: string;
  debounceMilliseconds: number;
  inspectOptions: InspectOptions;
  onCycle(cycle: InspectWatchCycle): Promise<void>;
  onChange?(changedPaths: string[]): void;
  sessionFactory?(options: InspectOptions): Promise<InspectWatchSession>;
};

export type InspectWatchSession = {
  inspect(): Promise<UIInspectResult>;
  close(): Promise<void>;
};

type WatchHandle = { close(): void };

const IGNORED_DIRECTORIES = new Set([
  '.git',
  '.tspack',
  'node_modules',
  'dist',
  'build',
  'coverage',
  'tspack-artifacts',
]);

const WATCHED_FILE_NAMES = new Set([
  'manifest.tsx',
  'package.manifest.tsx',
  'vite.config.ts',
  'vite.config.js',
  'vite.config.mts',
  'vite.config.mjs',
  'index.html',
]);

function isRelevantPath(filePath: string): boolean {
  if (WATCHED_FILE_NAMES.has(path.basename(filePath))) {
    return true;
  }
  return /\.(?:[cm]?[jt]sx?|css|scss|sass|less|html)$/i.test(filePath);
}

function collectWatchDirectories(root: string): string[] {
  const directories: string[] = [];
  const visit = (directory: string) => {
    directories.push(directory);
    for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
      if (!entry.isDirectory() || IGNORED_DIRECTORIES.has(entry.name)) {
        continue;
      }
      visit(path.join(directory, entry.name));
    }
  };
  visit(root);
  return directories;
}

function messageOf(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function isTransientInspectError(error: unknown): boolean {
  const message = messageOf(error);
  return (
    message.includes('TSPACK_INSPECT_SELECTOR_NOT_FOUND') ||
    message.includes('TSPACK_INSPECT_PAGE_LOAD_FAILED') ||
    message.includes('Execution context was destroyed') ||
    message.includes('Target page, context or browser has been closed')
  );
}

function delay(milliseconds: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

async function launchReusableChromium(
  options: InspectOptions,
): Promise<InspectWatchSession> {
  const playwright = await import('playwright');
  let executablePath = options.browserPath;
  let executableSource: 'explicit' | 'playwright-managed' | 'system' =
    executablePath ? 'explicit' : 'playwright-managed';
  let browser;
  try {
    browser = await playwright.chromium.launch({ executablePath });
  } catch (error: unknown) {
    if (executablePath) {
      throw error;
    }
    executablePath = findSystemChromiumExecutable() ?? undefined;
    if (!executablePath) {
      throw new Error(
        `TSPACK_INSPECT_BROWSER_NOT_FOUND: no managed or system Chromium is available | ${messageOf(error)}`,
      );
    }
    executableSource = 'system';
    browser = await playwright.chromium.launch({ executablePath });
  }

  const page = await browser.newPage({ viewport: options.viewport });
  let firstNavigation = true;

  return {
    async inspect(): Promise<UIInspectResult> {
      if (firstNavigation) {
        await page.goto(options.url as string, { waitUntil: 'load' });
        firstNavigation = false;
      } else {
        await page.reload({ waitUntil: 'domcontentloaded' });
      }
      const expression = buildInspectAnalyzerExpression(
        options.selector,
        options.points,
      );
      const payload = (await page.evaluate(expression)) as Pick<
        UIInspectResult,
        'root' | 'hitTests'
      >;
      if (options.selector && !payload.root) {
        throw new Error('TSPACK_INSPECT_SELECTOR_NOT_FOUND');
      }
      return {
        target: { url: page.url() },
        browser: {
          name: 'chromium',
          backend: options.browserPath ? 'browser-path' : 'playwright',
          launchBackend: options.browserPath
            ? 'browser-path'
            : 'playwright-chromium',
          version: browser.version(),
          executable: { source: executableSource, path: executablePath },
        },
        viewport: options.viewport,
        root: payload.root,
        hitTests: payload.hitTests,
        diagnostics: [],
      };
    },
    async close(): Promise<void> {
      await browser.close();
    },
  };
}

export async function runInspectWatch(options: InspectWatchOptions): Promise<void> {
  if (options.inspectOptions.cdpEndpoint || options.inspectOptions.hostPath) {
    throw new Error('TSPACK_INSPECT_WATCH_BACKEND_UNSUPPORTED');
  }
  if (!options.inspectOptions.url) {
    throw new Error('TSPACK_INSPECT_TARGET_REQUIRED');
  }

  const root = path.resolve(options.root);
  const session = await (
    options.sessionFactory ?? launchReusableChromium
  )(options.inspectOptions);
  const watchHandles: WatchHandle[] = [];
  const changedPaths = new Set<string>();
  let state = InspectWatchState.Idle;
  let generation = 0;
  let debounceTimer: ReturnType<typeof setTimeout> | undefined;
  let cyclePromise = Promise.resolve();
  let stopped = false;
  const maxCycles = Number(process.env.TSPACK_INSPECT_WATCH_MAX_CYCLES ?? '0');
  let resolveStop: () => void = () => {};
  let rejectStop: (error: unknown) => void = () => {};
  const stopPromise = new Promise<void>((resolve, reject) => {
    resolveStop = resolve;
    rejectStop = reject;
  });
  const stopFromSignal = () => resolveStop();

  const inspectWithTransientRetry = async (): Promise<UIInspectResult> => {
    let lastError: unknown;
    for (let attempt = 0; attempt < 8; attempt += 1) {
      try {
        state = attempt === 0
          ? InspectWatchState.Inspecting
          : InspectWatchState.WaitingForTarget;
        return await session.inspect();
      } catch (error: unknown) {
        lastError = error;
        if (!isTransientInspectError(error) || attempt === 7) {
          throw error;
        }
        state = InspectWatchState.FailedTransiently;
        await delay(125);
      }
    }
    throw lastError;
  };

  const runCycle = async () => {
    if (stopped) {
      return;
    }
    const cycleChanges = [...changedPaths].sort();
    changedPaths.clear();
    generation += 1;
    const cycleGeneration = generation;
    const result = await inspectWithTransientRetry();
    if (changedPaths.size > 0) {
      state = InspectWatchState.Dirty;
    } else {
      await options.onCycle({
        generation: cycleGeneration,
        changedPaths: cycleChanges,
        result,
        browserReused: cycleGeneration > 1,
      });
      state = InspectWatchState.Idle;
      if (Number.isInteger(maxCycles) && maxCycles > 0 && generation >= maxCycles) {
        stopped = true;
        resolveStop();
      }
    }
    if (changedPaths.size > 0) {
      await runCycle();
    }
  };

  const schedule = (changedPath: string) => {
    const relativePath = path.relative(root, changedPath).replace(/\\/g, '/');
    if (!isRelevantPath(relativePath)) {
      return;
    }
    changedPaths.add(relativePath);
    state = state === InspectWatchState.Inspecting
      ? InspectWatchState.Dirty
      : state;
    if (debounceTimer) {
      clearTimeout(debounceTimer);
    }
    debounceTimer = setTimeout(() => {
      debounceTimer = undefined;
      const pending = [...changedPaths].sort();
      options.onChange?.(pending);
      cyclePromise = cyclePromise.then(runCycle).catch((error: unknown) => {
        stopped = true;
        rejectStop(error);
      });
    }, options.debounceMilliseconds);
  };

  try {
    process.once('SIGINT', stopFromSignal);
    process.once('SIGTERM', stopFromSignal);
    for (const directory of collectWatchDirectories(root)) {
      const handle = fs.watch(directory, (eventType: string, fileName: string | Uint8Array | null) => {
        if (!fileName) {
          return;
        }
        const changedPath = path.join(directory, String(fileName));
        schedule(changedPath);
        if (eventType === 'rename' && fs.existsSync(changedPath)) {
          try {
            if (fs.statSync(changedPath).isDirectory()) {
              const nestedHandle = fs.watch(changedPath, (_nestedEvent: string, nestedName: string | Uint8Array | null) => {
                if (nestedName) {
                  schedule(path.join(changedPath, String(nestedName)));
                }
              });
              watchHandles.push(nestedHandle);
            }
          } catch {
            // A rename can disappear before stat; the parent event is enough.
          }
        }
      });
      watchHandles.push(handle);
    }

    await runCycle();
    if (!stopped) {
      await stopPromise;
      stopped = true;
      await cyclePromise;
    }
  } finally {
    state = InspectWatchState.Stopped;
    process.off('SIGINT', stopFromSignal);
    process.off('SIGTERM', stopFromSignal);
    if (debounceTimer) {
      clearTimeout(debounceTimer);
    }
    for (const handle of watchHandles) {
      handle.close();
    }
    await session.close();
  }
}
