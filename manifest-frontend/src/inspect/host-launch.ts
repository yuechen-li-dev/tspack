import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import net from 'node:net';
import { spawn, type ChildProcess } from 'node:child_process';

export type LaunchHostOptions = {
  executablePath: string;
  url?: string;
  env?: Record<string, string | undefined>;
  display?: string;
  headless?: boolean;
};

export type LaunchedHost = {
  executablePath: string;
  endpoint: string;
  process: ChildProcess;
  userDataDir: string;
  noSandboxUsed: boolean;
  cleanup: () => Promise<void>;
};

const SANDBOX_FAILURE_SIGNATURES = [
  'running as root',
  'no-sandbox',
  'setuid sandbox',
  'namespace sandbox',
  'zygote'
];

function tailOutput(text: string): string {
  const lines = text.trim().split('\n').slice(-8);
  return lines.join(' | ');
}

function diagnosticOutput(text: string): string {
  if (!text) {
    return '';
  }
  if (process.env.TSPACK_INSPECT_DEBUG === '1') {
    return tailOutput(text).slice(0, 2000);
  }
  return `[captured ${text.length} characters; use TSPACK_INSPECT_DEBUG=1 for bounded output]`;
}

function formatLaunchFailure(prefix: string, details: {
  args: string[];
  exitCode: number | null;
  signal: NodeJS.Signals | null;
  stderr: string;
  stdout: string;
}): Error {
  const stderrTail = diagnosticOutput(details.stderr);
  const stdoutTail = diagnosticOutput(details.stdout);
  const argsText = details.args.join(' ');
  return new Error(
    `${prefix}: exit=${details.exitCode ?? 'null'} signal=${details.signal ?? 'null'} args="${argsText}" stderrTail="${stderrTail}" stdoutTail="${stdoutTail}"`
  );
}

function isLikelySandboxFailure(stderr: string, stdout: string): boolean {
  const normalized = `${stderr}\n${stdout}`.toLowerCase();
  return SANDBOX_FAILURE_SIGNATURES.some((token) => normalized.includes(token));
}

export function validateHostPath(executablePath: string): void {
  if (!fs.existsSync(executablePath)) {
    throw new Error('TSPACK_INSPECT_HOST_PATH_NOT_FOUND');
  }
  const stat = fs.statSync(executablePath);
  if (!stat.isFile()) {
    throw new Error('TSPACK_INSPECT_HOST_PATH_INVALID');
  }
}

export function buildHostEnvironment(
  baseEnvironment: Record<string, string | undefined> = process.env,
  display?: string,
): Record<string, string | undefined> {
  const environment = { ...baseEnvironment };
  if (display !== undefined) {
    environment.DISPLAY = display;
  }
  return environment;
}

async function terminateWindowsProfileProcesses(
  userDataDir: string,
  environment: Record<string, string | undefined>,
): Promise<void> {
  const script = [
    "$profilePath = $env:TSPACK_INSPECT_CLEANUP_PROFILE;",
    "$browserNames = @('msedge.exe', 'chrome.exe', 'chromium.exe', 'Code.exe');",
    "Get-CimInstance Win32_Process |",
    "  Where-Object { $browserNames -contains $_.Name -and $_.CommandLine -like ('*' + $profilePath + '*') } |",
    "  ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }",
  ].join(' ');
  const cleanupProcess = spawn(
    'powershell.exe',
    ['-NoProfile', '-NonInteractive', '-Command', script],
    {
      env: {
        ...environment,
        TSPACK_INSPECT_CLEANUP_PROFILE: userDataDir,
      },
      stdio: 'ignore',
      windowsHide: true,
    },
  );
  await new Promise<void>((resolve) => {
    cleanupProcess.once('exit', () => resolve());
    setTimeout(() => resolve(), 5000);
  });
}

function needsWindowsProfileCleanup(executablePath: string): boolean {
  const executableName = path.win32.basename(executablePath).toLowerCase();
  return [
    'msedge.exe',
    'chrome.exe',
    'chromium.exe',
    'code.exe',
  ].includes(executableName);
}

async function chooseFreePort(): Promise<number> {
  return new Promise<number>((resolve, reject) => {
    const server = net.createServer();
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      if (!address || typeof address === 'string') {
        server.close();
        reject(new Error('TSPACK_INSPECT_HOST_LAUNCH_FAILED'));
        return;
      }
      const { port } = address;
      server.close((error: Error | undefined) => {
        if (error) {
          reject(error);
          return;
        }
        resolve(port);
      });
    });
  });
}

export async function launchInspectableHost(options: LaunchHostOptions): Promise<LaunchedHost> {
  validateHostPath(options.executablePath);

  const port = await chooseFreePort();
  const userDataDir = fs.mkdtempSync(path.join(os.tmpdir(), 'tspack-inspect-host-'));
  const endpoint = `http://127.0.0.1:${port}`;

  const baseArgs = [
    `--remote-debugging-port=${port}`,
    `--user-data-dir=${userDataDir}`,
    '--no-first-run',
    '--no-default-browser-check',
    '--disable-background-networking'
  ];

  const inheritedDisplay = options.env?.DISPLAY ?? process.env.DISPLAY;
  const inheritedWayland =
    options.env?.WAYLAND_DISPLAY ?? process.env.WAYLAND_DISPLAY;
  const headless =
    options.headless ??
    !(options.display ?? inheritedDisplay ?? inheritedWayland);
  if (headless) {
    baseArgs.push('--headless=new');
  }

  if (options.url) {
    baseArgs.push(options.url);
  }

  const launchOnce = async (args: string[]): Promise<{
    child: ChildProcess;
    readStderr: () => string;
    readStdout: () => string;
    readSpawnError: () => Error | undefined;
  }> => {
    let stderr = '';
    let stdout = '';
    let spawnError: Error | undefined;
    let child: ChildProcess;

    try {
      child = spawn(options.executablePath, args, {
        env: buildHostEnvironment(options.env, options.display),
        stdio: ['ignore', 'pipe', 'pipe'],
      });
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : String(error);
      throw formatLaunchFailure('TSPACK_INSPECT_HOST_LAUNCH_FAILED', {
        args,
        exitCode: null,
        signal: null,
        stderr: message,
        stdout: ''
      });
    }

    child.stderr?.on('data', (chunk) => {
      stderr += chunk.toString();
    });
    child.stdout?.on('data', (chunk) => {
      stdout += chunk.toString();
    });
    (child as ChildProcess & { on?: (event: string, listener: (error: Error) => void) => void }).on?.('error', (error: Error) => {
      spawnError = error;
    });
    return {
      child,
      readStderr: () => stderr,
      readStdout: () => stdout,
      readSpawnError: () => spawnError
    };
  };

  const forceNoSandbox = process.env.TSPACK_INSPECT_HOST_NO_SANDBOX === '1';
  const noSandboxArgs = [...baseArgs, '--no-sandbox', '--disable-gpu', '--disable-dev-shm-usage'];
  let args = forceNoSandbox ? noSandboxArgs : baseArgs;
  let launched = await launchOnce(args);
  let child = launched.child;
  let readStderr = launched.readStderr;
  let readStdout = launched.readStdout;
  let readSpawnError = launched.readSpawnError;
  let noSandboxUsed = forceNoSandbox;

  const cleanup = async (): Promise<void> => {
    const launchEnvironment = buildHostEnvironment(options.env, options.display);
    if (
      process.platform === 'win32' &&
      needsWindowsProfileCleanup(options.executablePath)
    ) {
      await terminateWindowsProfileProcesses(userDataDir, launchEnvironment);
    }
    if (!child.killed && child.exitCode === null) {
      const childPID = (child as ChildProcess & { pid?: number }).pid;
      if (process.platform === 'win32' && childPID) {
        const treeKill = spawn(
          'taskkill.exe',
          ['/PID', String(childPID), '/T', '/F'],
          {
            env: launchEnvironment,
            stdio: 'ignore',
          },
        );
        await new Promise<void>((resolve) => {
          treeKill.once('exit', () => resolve());
          setTimeout(() => resolve(), 3000);
        });
      } else {
        child.kill('SIGTERM');
      }
      await new Promise<void>((resolve) => {
        child.once('exit', () => resolve());
        setTimeout(() => resolve(), 1500);
      });
    }
    for (let attempt = 0; attempt < 20; attempt += 1) {
      try {
        fs.rmSync(userDataDir, { recursive: true, force: true });
        return;
      } catch {
        if (attempt === 19) {
          throw new Error('TSPACK_INSPECT_HOST_CLEANUP_FAILED');
        }
        await new Promise((resolve) => setTimeout(resolve, 100));
      }
    }
  };

  const waitUntilReady = async (): Promise<void> => {
    const startedAt = Date.now();
    while (Date.now() - startedAt < 5000) {
      const spawnError = readSpawnError();
      if (spawnError) {
        throw formatLaunchFailure('TSPACK_INSPECT_HOST_LAUNCH_FAILED', {
          args,
          exitCode: child.exitCode,
          signal: child.signalCode,
          stderr: `${readStderr()} ${spawnError.message}`.trim(),
          stdout: readStdout()
        });
      }
      const detachedWindowsBrowser =
        process.platform === 'win32' &&
        needsWindowsProfileCleanup(options.executablePath) &&
        child.exitCode === 0;
      if (child.exitCode !== null && !detachedWindowsBrowser) {
        throw formatLaunchFailure('TSPACK_INSPECT_HOST_LAUNCH_FAILED', {
          args,
          exitCode: child.exitCode,
          signal: child.signalCode,
          stderr: readStderr(),
          stdout: readStdout()
        });
      }
      try {
        const response = await fetch(`${endpoint}/json/version`);
        if (response.ok) {
          return;
        }
      } catch {
        // Wait for endpoint to come up.
      }
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
    throw formatLaunchFailure('TSPACK_INSPECT_HOST_CDP_ENDPOINT_FAILED', {
      args,
      exitCode: child.exitCode,
      signal: child.signalCode,
      stderr: readStderr(),
      stdout: readStdout()
    });
  };

  try {
    await waitUntilReady();
  } catch (error: unknown) {
    const shouldRetryWithNoSandbox = !forceNoSandbox
      && child.exitCode !== null
      && isLikelySandboxFailure(readStderr(), readStdout());

    if (shouldRetryWithNoSandbox) {
      await cleanup();
      args = noSandboxArgs;
      launched = await launchOnce(args);
      child = launched.child;
      readStderr = launched.readStderr;
      readStdout = launched.readStdout;
      readSpawnError = launched.readSpawnError;
      noSandboxUsed = true;
      try {
        await waitUntilReady();
      } catch (retryError: unknown) {
        await cleanup();
        throw retryError;
      }
    } else {
      await cleanup();
      throw error;
    }
  }

  return {
    executablePath: options.executablePath,
    endpoint,
    process: child,
    userDataDir,
    noSandboxUsed,
    cleanup
  };
}
