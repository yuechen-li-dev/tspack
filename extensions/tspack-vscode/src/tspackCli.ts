import { execFile } from 'node:child_process';
import type { CdpTargetListResult, InspectResult } from './inspectTypes';

export type TspackCommand = {
  command: string;
  args: string[];
};

export class TspackCliError extends Error {
  readonly code: string;
  readonly stdout: string;
  readonly stderr: string;

  constructor(code: string, message: string, stdout = '', stderr = '') {
    super(message);
    this.code = code;
    this.stdout = stdout;
    this.stderr = stderr;
  }
}

export function buildListTargetsCommand(
  tspackPath: string,
  endpoint: string,
): TspackCommand {
  return {
    command: tspackPath,
    args: ['inspect', '--cdp', endpoint, '--list-targets', '--json'],
  };
}

export function buildInspectTargetCommand(
  tspackPath: string,
  endpoint: string,
  targetIndex: number,
): TspackCommand {
  return {
    command: tspackPath,
    args: ['inspect', '--cdp', endpoint, '--target', String(targetIndex), '--json'],
  };
}

function execTspack(
  command: TspackCommand,
): Promise<{ stdout: string; stderr: string }> {
  return new Promise((resolve, reject) => {
    execFile(command.command, command.args, { windowsHide: true }, (
      error,
      stdout,
      stderr,
    ) => {
      const stdoutText = stdout.toString();
      const stderrText = stderr.toString();
      if (!error) {
        resolve({ stdout: stdoutText, stderr: stderrText });
        return;
      }

      const maybeNodeError = error as NodeJS.ErrnoException;
      if (maybeNodeError.code === 'ENOENT') {
        reject(new TspackCliError(
          'TSPACK_CLI_NOT_FOUND',
          'TSPack CLI not found. Set tspack.inspect.tspackPath.',
          stdoutText,
          stderrText,
        ));
        return;
      }

      reject(new TspackCliError(
        'TSPACK_CLI_FAILED',
        stderrText.trim() || error.message,
        stdoutText,
        stderrText,
      ));
    });
  });
}

function parseJson<T>(stdout: string, stderr: string): T {
  try {
    return JSON.parse(stdout) as T;
  } catch {
    throw new TspackCliError(
      'TSPACK_CLI_INVALID_JSON',
      'tspack inspect returned invalid JSON. See the TSPack Inspect output channel for details.',
      stdout,
      stderr,
    );
  }
}

export async function listCdpTargets(
  tspackPath: string,
  endpoint: string,
): Promise<{ result: CdpTargetListResult; stdout: string; stderr: string }> {
  const command = buildListTargetsCommand(tspackPath, endpoint);
  const executed = await execTspack(command);
  return {
    result: parseJson<CdpTargetListResult>(executed.stdout, executed.stderr),
    stdout: executed.stdout,
    stderr: executed.stderr,
  };
}

export async function inspectCdpTarget(
  tspackPath: string,
  endpoint: string,
  targetIndex: number,
): Promise<{ result: InspectResult; stdout: string; stderr: string }> {
  const command = buildInspectTargetCommand(tspackPath, endpoint, targetIndex);
  const executed = await execTspack(command);
  return {
    result: parseJson<InspectResult>(executed.stdout, executed.stderr),
    stdout: executed.stdout,
    stderr: executed.stderr,
  };
}
