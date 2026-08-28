// Temporary sandbox fallback while @types/node cannot be installed from npm (registry 403 in this environment).
// Keep this file intentionally minimal and limited to symbols required by production source build.
declare namespace NodeJS {
  type Platform = 'aix' | 'darwin' | 'freebsd' | 'linux' | 'openbsd' | 'sunos' | 'win32';
  type Signals =
    | 'SIGABRT' | 'SIGALRM' | 'SIGBUS' | 'SIGCHLD' | 'SIGCONT' | 'SIGFPE' | 'SIGHUP' | 'SIGILL' | 'SIGINT'
    | 'SIGIO' | 'SIGIOT' | 'SIGKILL' | 'SIGPIPE' | 'SIGPOLL' | 'SIGPROF' | 'SIGPWR' | 'SIGQUIT' | 'SIGSEGV'
    | 'SIGSTKFLT' | 'SIGSTOP' | 'SIGSYS' | 'SIGTERM' | 'SIGTRAP' | 'SIGTSTP' | 'SIGTTIN' | 'SIGTTOU' | 'SIGUNUSED'
    | 'SIGURG' | 'SIGUSR1' | 'SIGUSR2' | 'SIGVTALRM' | 'SIGWINCH' | 'SIGXCPU' | 'SIGXFSZ';
  interface Timeout {}
}

declare const process: {
  argv: string[];
  env: Record<string, string | undefined>;
  platform: NodeJS.Platform;
  cwd(): string;
  chdir(directory: string): void;
  stdin: unknown;
  exit(code?: number): never;
  stdout: { write(chunk: string | Uint8Array): boolean };
  stderr: { write(chunk: string | Uint8Array): boolean };
  once(event: 'SIGINT' | 'SIGTERM', listener: () => void): void;
  off(event: 'SIGINT' | 'SIGTERM', listener: () => void): void;
};

declare const Buffer: {
  from(input: string, encoding?: string): Uint8Array;
  byteLength(input: string, encoding?: string): number;
};

declare module 'node:child_process' {
  export type ChildProcess = {
    stderr?: { on(event: 'data', listener: (chunk: Uint8Array | string) => void): void };
    stdout?: { on(event: 'data', listener: (chunk: Uint8Array | string) => void): void };
    killed: boolean;
    exitCode: number | null;
    signalCode: NodeJS.Signals | null;
    kill(signal?: NodeJS.Signals): boolean;
    once(event: 'exit', listener: () => void): void;
  };
  export function spawn(command: string, args?: string[], options?: unknown): ChildProcess;
}

declare module 'node:fs';
declare module 'node:fs/promises';
declare module 'node:path';
declare module 'node:os';
declare module 'node:net';
declare module 'node:http';
declare module 'node:util';
declare module 'node:perf_hooks';
declare module 'node:crypto';
declare module 'node:url';
declare module 'node:readline';
