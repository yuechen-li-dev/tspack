import { spawn } from 'node:child_process';
import fs from 'node:fs/promises';
import path from 'node:path';
import { performance } from 'node:perf_hooks';
import type { Diagnostic } from './types.js';

export type CommandRunOptions = {
  cwd?: string;
  env?: Record<string, string>;
  timeoutSeconds?: number;
};

export type CommandResult = {
  args: string[];
  cwd: string;
  exitCode: number | null;
  signal?: string | null;
  timedOut: boolean;
  stdout: string;
  stderr: string;
  durationMs: number;
  diagnostics: Diagnostic[];
  evidence?: { stdoutPath?: string; stderrPath?: string; commandPath?: string };
};

export type CommandContext = {
  run: (args: string[], reason: string, options?: CommandRunOptions) => Promise<CommandResult>;
  tspack: (args: string[], reason: string, options?: CommandRunOptions) => Promise<CommandResult>;
};

export function createCommandContext(params: { projectRoot: string; evidenceDir: string; tspackPath?: string }): CommandContext {
  let index = 0;

  const run = async (args: string[], reason: string, options?: CommandRunOptions): Promise<CommandResult> => {
    validateArgs(args);
    validateReason(reason);
    const cwd = resolveCwd(params.projectRoot, options?.cwd);
    const timeoutSeconds = validateTimeout(options?.timeoutSeconds);
    const started = performance.now();
    const diagnostics: Diagnostic[] = [];

    const result = await spawnAndCapture(args, cwd, options?.env, timeoutSeconds, diagnostics);
    const commandResult: CommandResult = {
      args: [...args],
      cwd,
      exitCode: result.exitCode,
      signal: result.signal,
      timedOut: result.timedOut,
      stdout: result.stdout,
      stderr: result.stderr,
      durationMs: performance.now() - started,
      diagnostics,
    };

    const evidence = await writeEvidence(params.evidenceDir, index, reason, commandResult);
    index += 1;
    commandResult.evidence = evidence.paths;
    if (evidence.diagnostic) {
      diagnostics.push(evidence.diagnostic);
    }

    return commandResult;
  };

  return {
    run,
    tspack: async (args: string[], reason: string, options?: CommandRunOptions): Promise<CommandResult> => {
      validateArgs(args);
      const executable = params.tspackPath?.trim() ? params.tspackPath : 'tspack';
      return run([executable, ...args], reason, options);
    },
  };
}

function validateArgs(args: string[]): void {
  if (!Array.isArray(args) || args.length === 0 || args.some((entry) => typeof entry !== 'string' || entry.length === 0)) {
    throw withCode('TSPACK_COMMAND_EMPTY', 'command args must be a non-empty string array');
  }
}

function validateReason(reason: string): void {
  if (typeof reason !== 'string' || reason.trim().length === 0) {
    throw withCode('TSPACK_COMMAND_REASON_REQUIRED', 'command reason is required');
  }
}

function validateTimeout(raw: number | undefined): number {
  if (raw === undefined) {
    return 30;
  }
  if (!Number.isFinite(raw) || raw <= 0) {
    throw withCode('TSPACK_COMMAND_INVALID_TIMEOUT', 'timeoutSeconds must be a positive finite number');
  }
  return raw;
}

function resolveCwd(projectRoot: string, cwd: string | undefined): string {
  if (cwd === undefined) {
    return projectRoot;
  }
  if (!cwd.trim() || cwd.startsWith('/') || cwd.includes('..') || cwd.includes('\\')) {
    throw withCode('TSPACK_COMMAND_INVALID_CWD', `invalid cwd: ${cwd}`);
  }
  const resolved = path.resolve(projectRoot, cwd);
  if (!resolved.startsWith(`${projectRoot}${path.sep}`) && resolved !== projectRoot) {
    throw withCode('TSPACK_COMMAND_INVALID_CWD', `cwd escapes project root: ${cwd}`);
  }
  return resolved;
}

async function spawnAndCapture(args: string[], cwd: string, env: Record<string, string> | undefined, timeoutSeconds: number, diagnostics: Diagnostic[]) {
  return await new Promise<{ exitCode: number | null; signal: string | null; timedOut: boolean; stdout: string; stderr: string }>((resolve) => {
    let stdout = '';
    let stderr = '';
    let timedOut = false;
    let settled = false;
    const spawnArgs = resolveSpawnArgs(args);

    const child = spawn(spawnArgs[0], spawnArgs.slice(1), { cwd, env: { ...process.env, ...(env ?? {}) }, stdio: ['ignore', 'pipe', 'pipe'] });
    child.stdout?.on('data', (chunk) => { stdout += String(chunk); });
    child.stderr?.on('data', (chunk) => { stderr += String(chunk); });

    const timer = setTimeout(() => {
      timedOut = true;
      diagnostics.push({ code: 'TSPACK_COMMAND_TIMEOUT', message: `command timed out after ${timeoutSeconds} seconds`, file: cwd, severity: 'error' });
      child.kill('SIGKILL');
    }, Math.max(1, timeoutSeconds * 1000));

    child.on('error', (error) => {
      if (settled) {
        return;
      }
      settled = true;
      clearTimeout(timer);
      diagnostics.push({ code: 'TSPACK_COMMAND_SPAWN_FAILED', message: error.message, file: cwd, severity: 'error' });
      resolve({ exitCode: null, signal: null, timedOut, stdout, stderr });
    });

    child.on('close', (exitCode, signal) => {
      if (settled) {
        return;
      }
      settled = true;
      clearTimeout(timer);
      resolve({ exitCode, signal, timedOut, stdout, stderr });
    });
  });
}

function resolveSpawnArgs(args: string[]): string[] {
  if (process.platform !== 'win32') {
    return args;
  }

  const commandPath = args[0];
  const extension = path.extname(commandPath).toLowerCase();
  if (extension === '.js' || extension === '.cjs' || extension === '.mjs') {
    return [process.execPath, commandPath, ...args.slice(1)];
  }

  return args;
}

async function writeEvidence(evidenceDir: string, index: number, reason: string, result: CommandResult): Promise<{ paths: CommandResult['evidence']; diagnostic?: Diagnostic }> {
  const prefix = path.join(evidenceDir, `${index}`);
  const stdoutPath = `${prefix}.stdout.txt`;
  const stderrPath = `${prefix}.stderr.txt`;
  const commandPath = `${prefix}.command.json`;
  try {
    await fs.mkdir(evidenceDir, { recursive: true });
    await fs.writeFile(stdoutPath, result.stdout, 'utf8');
    await fs.writeFile(stderrPath, result.stderr, 'utf8');
    const commandPayload = {
      args: result.args,
      cwd: result.cwd,
      exitCode: result.exitCode,
      signal: result.signal,
      timedOut: result.timedOut,
      durationMs: result.durationMs,
      reason,
      diagnostics: result.diagnostics.map((entry) => entry.code),
    };
    await fs.writeFile(commandPath, `${JSON.stringify(commandPayload, null, 2)}\n`, 'utf8');
    return { paths: { stdoutPath, stderrPath, commandPath } };
  } catch (error) {
    return {
      paths: {},
      diagnostic: { code: 'TSPACK_COMMAND_EVIDENCE_WRITE_FAILED', message: (error as Error).message, file: evidenceDir, severity: 'error' },
    };
  }
}

function withCode(code: string, message: string): Error & { code: string } {
  const error = new Error(message) as Error & { code: string };
  error.code = code;
  return error;
}
