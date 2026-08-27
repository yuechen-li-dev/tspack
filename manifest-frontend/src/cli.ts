import path from 'node:path';
import readline from 'node:readline';
import { analyzeDependencySource, parsePackageManifestFile, parseWorkspace } from './index.js';

type WorkerRequest = {
  id: number;
  operation?: 'parse' | 'analyze-dependencies';
  manifestPath: string;
  packageName?: string;
  directory: string;
  environment: string[];
};

function parseManifest(manifestPath: string): ReturnType<typeof parseWorkspace> {
  if (path.basename(manifestPath) === 'package.manifest.tsx') {
    return parsePackageManifestFile(manifestPath);
  }
  return parseWorkspace(manifestPath);
}

function replaceEnvironment(environment: string[]): void {
  for (const name of Object.keys(process.env)) {
    delete process.env[name];
  }
  for (const entry of environment) {
    const separator = entry.indexOf('=');
    if (separator < 0) {
      continue;
    }
    process.env[entry.slice(0, separator)] = entry.slice(separator + 1);
  }
}

async function runWorker(): Promise<void> {
  const lines = readline.createInterface({ input: process.stdin });
  for await (const line of lines) {
    let id = 0;
    try {
      const request = JSON.parse(line) as WorkerRequest;
      id = request.id;
      replaceEnvironment(request.environment);
      process.chdir(request.directory);
      const result = request.operation === 'analyze-dependencies'
        ? analyzeDependencySource(request.manifestPath, request.packageName)
        : parseManifest(request.manifestPath);
      process.stdout.write(`${JSON.stringify({ id, result })}\n`);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      process.stdout.write(`${JSON.stringify({ id, error: message })}\n`);
    }
  }
}

async function main(): Promise<void> {
  if (process.argv[2] === '--stdio-worker') {
    await runWorker();
    return;
  }
  if (process.argv[2] === '--analyze-dependencies') {
    const manifestPath = process.argv[3];
    if (!manifestPath) {
      process.stderr.write('usage: node dist/cli.js --analyze-dependencies <manifest.tsx> [package-name]\n');
      process.exit(2);
    }
    const result = analyzeDependencySource(manifestPath, process.argv[4]);
    process.stdout.write(`${JSON.stringify(result)}\n`);
    return;
  }
  const manifestPath = process.argv[2];
  if (!manifestPath) {
    process.stderr.write('usage: node dist/src/cli.js <manifest.tsx>\n');
    process.exit(2);
  }
  const result = parseManifest(manifestPath);
  process.stdout.write(`${JSON.stringify(result)}\n`);
  if (!result.ok) {
    process.exit(1);
  }
}

void main();
