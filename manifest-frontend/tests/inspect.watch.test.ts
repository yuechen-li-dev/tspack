import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { InspectOptions } from '../src/inspect/index.js';
import {
  InspectWatchState,
  runInspectWatch,
  type InspectWatchSession,
} from '../src/inspect/watch.js';

const roots: string[] = [];

afterEach(async () => {
  delete process.env.TSPACK_INSPECT_WATCH_MAX_CYCLES;
  await Promise.all(roots.splice(0).map((root) => fs.rm(root, {
    recursive: true,
    force: true,
  })));
});

function inspectOptions(root: string): InspectOptions {
  return {
    url: 'http://127.0.0.1:5173',
    browser: 'chromium',
    viewport: { width: 800, height: 600 },
    points: [],
    json: false,
    root,
  };
}

function result() {
  return {
    target: { url: 'http://127.0.0.1:5173' },
    browser: { name: 'chromium' as const, backend: 'playwright' as const },
    viewport: { width: 800, height: 600 },
    root: {
      id: 'node-1',
      tag: 'main',
      bounds: { x: 0, y: 0, width: 800, height: 600 },
      visible: true,
      children: [],
    },
    hitTests: [],
    diagnostics: [],
  };
}

describe('inspect watch', () => {
  it('models the documented lifecycle states explicitly', () => {
    expect(Object.values(InspectWatchState)).toEqual([
      'Idle',
      'Inspecting',
      'Dirty',
      'WaitingForTarget',
      'FailedTransiently',
      'Stopped',
    ]);
  });

  it('coalesces file events, reuses one session, and cleans it up', async () => {
    const root = await fs.mkdtemp(path.join(os.tmpdir(), 'tspack-watch-'));
    roots.push(root);
    await fs.mkdir(path.join(root, 'src'));
    const sourcePath = path.join(root, 'src', 'Toast.tsx');
    await fs.writeFile(sourcePath, 'export const value = 1;\n');

    const inspect = vi.fn(async () => result());
    const close = vi.fn(async () => {});
    const session: InspectWatchSession = { inspect, close };
    const cycles: Array<{ generation: number; changedPaths: string[] }> = [];
    process.env.TSPACK_INSPECT_WATCH_MAX_CYCLES = '2';

    const watchPromise = runInspectWatch({
      root,
      debounceMilliseconds: 50,
      inspectOptions: inspectOptions(root),
      sessionFactory: async () => session,
      async onCycle(cycle) {
        cycles.push({
          generation: cycle.generation,
          changedPaths: cycle.changedPaths,
        });
      },
    });

    await waitFor(() => cycles.length === 1);
    await fs.writeFile(sourcePath, 'export const value = 2;\n');
    await fs.writeFile(sourcePath, 'export const value = 3;\n');
    await watchPromise;

    expect(cycles).toEqual([
      { generation: 1, changedPaths: [] },
      { generation: 2, changedPaths: ['src/Toast.tsx'] },
    ]);
    expect(inspect).toHaveBeenCalledTimes(2);
    expect(close).toHaveBeenCalledTimes(1);
  });

  it('reuses and cleans one session across fifty bounded update cycles', async () => {
    const root = await fs.mkdtemp(path.join(os.tmpdir(), 'tspack-watch-stress-'));
    roots.push(root);
    await fs.mkdir(path.join(root, 'src'));
    const sourcePath = path.join(root, 'src', 'Toast.tsx');
    await fs.writeFile(sourcePath, 'export const generation = 0;\n');

    const inspect = vi.fn(async () => result());
    const close = vi.fn(async () => {});
    process.env.TSPACK_INSPECT_WATCH_MAX_CYCLES = '51';
    let completedCycles = 0;
    await runInspectWatch({
      root,
      debounceMilliseconds: 25,
      inspectOptions: inspectOptions(root),
      sessionFactory: async () => ({ inspect, close }),
      async onCycle(cycle) {
        completedCycles = cycle.generation;
        if (cycle.generation < 51) {
          await fs.writeFile(
            sourcePath,
            `export const generation = ${cycle.generation};\n`,
          );
        }
      },
    });

    expect(completedCycles).toBe(51);
    expect(inspect).toHaveBeenCalledTimes(51);
    expect(close).toHaveBeenCalledTimes(1);
  }, 10_000);
});

async function waitFor(predicate: () => boolean): Promise<void> {
  const deadline = Date.now() + 3000;
  while (Date.now() < deadline) {
    if (predicate()) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  throw new Error('timed out waiting for inspect watch state');
}
