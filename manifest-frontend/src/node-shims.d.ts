declare namespace NodeJS {
  interface Timeout {}
}

declare const process: {
  argv: string[];
  env: Record<string, string | undefined>;
  cwd(): string;
  exit(code?: number): never;
  stdout: { write(chunk: string | Uint8Array): boolean };
  stderr: { write(chunk: string | Uint8Array): boolean };
};

declare const Buffer: {
  from(input: string, encoding?: string): Uint8Array;
  byteLength(input: string, encoding?: string): number;
};

declare module 'node:fs';
declare module 'node:fs/promises';
declare module 'node:path';
declare module 'node:os';
declare module 'node:net';
declare module 'node:child_process';
declare module 'node:http';
declare module 'node:util';
declare module 'node:perf_hooks';
declare module 'node:crypto';
declare module 'node:url';
