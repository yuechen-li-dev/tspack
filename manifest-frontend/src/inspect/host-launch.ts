import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import net from 'node:net';
import { spawn, type ChildProcess } from 'node:child_process';

export type LaunchHostOptions = {
  executablePath: string;
  url?: string;
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

function formatLaunchFailure(prefix: string, details: {
  args: string[];
  exitCode: number | null;
  signal: NodeJS.Signals | null;
  stderr: string;
  stdout: string;
}): Error {
  const stderrTail = tailOutput(details.stderr);
  const stdoutTail = tailOutput(details.stdout);
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

  if (options.url) {
    baseArgs.push(options.url);
  }

  const launchOnce = async (args: string[]): Promise<{ child: ChildProcess; readStderr: () => string; readStdout: () => string }> => {
    const child = spawn(options.executablePath, args, { stdio: ['ignore', 'pipe', 'pipe'] });
    let stderr = '';
    let stdout = '';
    child.stderr?.on('data', (chunk) => {
      stderr += chunk.toString();
    });
    child.stdout?.on('data', (chunk) => {
      stdout += chunk.toString();
    });
    return { child, readStderr: () => stderr, readStdout: () => stdout };
  };

  const forceNoSandbox = process.env.TSPACK_INSPECT_HOST_NO_SANDBOX === '1';
  const noSandboxArgs = [...baseArgs, '--no-sandbox', '--disable-gpu', '--disable-dev-shm-usage'];
  let args = forceNoSandbox ? noSandboxArgs : baseArgs;
  let launched = await launchOnce(args);
  let child = launched.child;
  let readStderr = launched.readStderr;
  let readStdout = launched.readStdout;
  let noSandboxUsed = forceNoSandbox;

  const cleanup = async (): Promise<void> => {
    if (!child.killed && child.exitCode === null) {
      child.kill('SIGTERM');
      await new Promise<void>((resolve) => {
        child.once('exit', () => resolve());
        setTimeout(() => resolve(), 1500);
      });
    }
    try {
      fs.rmSync(userDataDir, { recursive: true, force: true });
    } catch {
      throw new Error('TSPACK_INSPECT_HOST_CLEANUP_FAILED');
    }
  };

  const waitUntilReady = async (): Promise<void> => {
    const startedAt = Date.now();
    while (Date.now() - startedAt < 5000) {
      if (child.exitCode !== null) {
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
