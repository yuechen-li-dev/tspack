import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import {
  buildHostEnvironment,
  launchInspectableHost,
  validateHostPath,
} from '../src/inspect/host-launch.js';
import { runInspect } from '../src/inspect/backend.js';

describe('inspect host launch', () => {
  it('forwards the full environment while composing a display override', () => {
    const environment = buildHostEnvironment(
      {
        PATH: '/tools',
        HOME: '/home/tester',
        TSPACK_TEST_SECRET: 'not-for-diagnostics',
        DISPLAY: ':1',
      },
      ':99',
    );

    expect(environment).toMatchObject({
      PATH: '/tools',
      HOME: '/home/tester',
      TSPACK_TEST_SECRET: 'not-for-diagnostics',
      DISPLAY: ':99',
    });
  });

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
    const executablePath = path.join(
      dir,
      process.platform === 'win32' ? 'fake-host.cmd' : 'fake-host.sh',
    );
    const script = process.platform === 'win32'
      ? '@echo off\r\necho boom %TSPACK_TEST_SECRET% 1>&2\r\nexit /b 1\r\n'
      : '#!/usr/bin/env sh\necho "boom $TSPACK_TEST_SECRET" 1>&2\nexit 1\n';
    fs.writeFileSync(executablePath, script, { mode: 0o755 });

    let message = '';
    try {
      await launchInspectableHost({
        executablePath,
        env: {
          ...process.env,
          TSPACK_TEST_SECRET: 'sentinel-secret-value',
        },
      });
    } catch (error: unknown) {
      message = (error as Error).message;
    }
    expect(message).toContain('TSPACK_INSPECT_HOST_LAUNCH_FAILED');
    expect(message).not.toContain('sentinel-secret-value');
    expect(message).not.toContain('boom');
  });

  it('uses no-sandbox args when env flag is set', async () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'inspect-host-nosandbox-'));
    const executablePath = path.join(
      dir,
      process.platform === 'win32' ? 'fake-host.cmd' : 'fake-host.sh',
    );
    const script = process.platform === 'win32'
      ? '@echo off\r\nset ARGS=%*\r\necho %ARGS% | findstr /C:"--no-sandbox" >nul\r\nif errorlevel 1 exit /b 1\r\npowershell -Command "Start-Sleep -Seconds 8"\r\n'
      : '#!/usr/bin/env sh\ncase "$*" in\n  *--no-sandbox*) sleep 8 ;;\n  *) exit 1 ;;\nesac\n';
    fs.writeFileSync(executablePath, script, { mode: 0o755 });

    process.env.TSPACK_INSPECT_HOST_NO_SANDBOX = '1';
    try {
      const launched = await launchInspectableHost({ executablePath });
      expect(launched.noSandboxUsed).toBe(true);
      await launched.cleanup();
    } catch (error: unknown) {
      expect((error as Error).message).toMatch(/TSPACK_INSPECT_HOST_(CDP_ENDPOINT_FAILED|LAUNCH_FAILED)/);
    } finally {
      delete process.env.TSPACK_INSPECT_HOST_NO_SANDBOX;
    }
  }, 15000);
});
