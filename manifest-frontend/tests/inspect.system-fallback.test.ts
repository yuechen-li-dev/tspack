import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const launchCalls: Array<{ executablePath?: string }> = [];

vi.mock('playwright', () => ({
  chromium: {
    launch: async (options: { executablePath?: string } = {}) => {
      launchCalls.push(options);
      if (!options.executablePath) {
        throw new Error(
          "browserType.launch: Executable doesn't exist at /missing/playwright/chromium",
        );
      }
      return {
        version: () => '140.0.0-system',
        newPage: async () => ({
          goto: async () => undefined,
          evaluate: async () => ({
            root: {
              id: 'node-1',
              tag: 'main',
              role: 'main',
              bounds: { x: 0, y: 0, width: 800, height: 600 },
              visible: true,
              children: [],
            },
            hitTests: [],
          }),
        }),
        close: async () => undefined,
      };
    },
  },
  webkit: {
    launch: async () => {
      throw new Error('unused');
    },
  },
}));

import { runInspect } from '../src/inspect/backend.js';

describe('Playwright Chromium system fallback', () => {
  let previousPath: string | undefined;
  let executablePath: string;

  beforeEach(() => {
    launchCalls.length = 0;
    previousPath = process.env.PATH;
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'tspack-browser-'));
    const executableName = process.platform === 'win32' ? 'chromium.exe' : 'chromium';
    executablePath = path.join(directory, executableName);
    fs.writeFileSync(executablePath, 'bounded fake executable');
    process.env.PATH = `${directory}${path.delimiter}${previousPath ?? ''}`;
  });

  afterEach(() => {
    if (previousPath === undefined) {
      delete process.env.PATH;
    } else {
      process.env.PATH = previousPath;
    }
  });

  it('retries a missing managed browser with system Chromium and reports provenance', async () => {
    const result = await runInspect({
      url: 'http://127.0.0.1:5173',
      browser: 'playwright-chromium',
      viewport: { width: 800, height: 600 },
      points: [],
      json: true,
    });

    expect(launchCalls).toHaveLength(2);
    expect(launchCalls[0]).toEqual({ executablePath: undefined });
    expect(launchCalls[1].executablePath).toBeTruthy();
    const selectedExecutablePath = launchCalls[1].executablePath;
    expect(result.browser).toMatchObject({
      name: 'chromium',
      backend: 'playwright',
      launchBackend: 'playwright-chromium',
      version: '140.0.0-system',
      executable: {
        source: 'system',
        path: selectedExecutablePath,
      },
    });
  });
});
