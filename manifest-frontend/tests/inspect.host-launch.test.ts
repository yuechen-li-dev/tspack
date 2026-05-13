import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import { launchInspectableHost, validateHostPath } from '../src/inspect/host-launch.js';
import { runInspect } from '../src/inspect/backend.js';

describe('inspect host launch', () => {
  it('validates host path', () => {
    expect(() => validateHostPath('/definitely/missing/host')).toThrow('TSPACK_INSPECT_HOST_PATH_NOT_FOUND');
    expect(() => validateHostPath(os.tmpdir())).toThrow('TSPACK_INSPECT_HOST_PATH_INVALID');
  });

  it('rejects conflicting backend options', async () => {
    await expect(runInspect({
      url: 'http://localhost:5173',
      browser: 'auto',
      hostPath: '/tmp/one',
      browserPath: '/tmp/two',
      viewport: { width: 800, height: 600 },
      points: [],
      json: true
    })).rejects.toThrow('TSPACK_INSPECT_INVALID_BACKEND_OPTIONS');

    await expect(runInspect({
      url: 'http://localhost:5173',
      browser: 'host-path',
      hostPath: '/tmp/one',
      cdpEndpoint: 'http://127.0.0.1:9222',
      viewport: { width: 800, height: 600 },
      points: [],
      json: true
    })).rejects.toThrow('TSPACK_INSPECT_INVALID_BACKEND_OPTIONS');
  });

  it('surfaces host launch failure for early exit host', async () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'inspect-host-fail-'));
    const executablePath = path.join(dir, 'fake-host.sh');
    fs.writeFileSync(executablePath, '#!/usr/bin/env bash\necho boom 1>&2\nexit 1\n', { mode: 0o755 });

    await expect(launchInspectableHost({ executablePath })).rejects.toThrow('TSPACK_INSPECT_HOST_LAUNCH_FAILED');
  });

  it('uses no-sandbox args when env flag is set', async () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'inspect-host-nosandbox-'));
    const executablePath = path.join(dir, 'fake-host.sh');
    fs.writeFileSync(executablePath, '#!/usr/bin/env bash\nif [[ "$*" == *"--no-sandbox"* ]]; then sleep 8; else exit 1; fi\n', { mode: 0o755 });

    process.env.TSPACK_INSPECT_HOST_NO_SANDBOX = '1';
    try {
      const launched = await launchInspectableHost({ executablePath });
      expect(launched.noSandboxUsed).toBe(true);
      await launched.cleanup();
    } catch (error: unknown) {
      expect((error as Error).message).toContain('TSPACK_INSPECT_HOST_CDP_ENDPOINT_FAILED');
    } finally {
      delete process.env.TSPACK_INSPECT_HOST_NO_SANDBOX;
    }
  }, 15000);
});
