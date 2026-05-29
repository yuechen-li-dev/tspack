import path from 'node:path';
import { parsePackageManifestFile, parseWorkspace } from './index.js';

function main(): void {
  const manifestPath = process.argv[2];
  if (!manifestPath) {
    process.stderr.write('usage: node dist/src/cli.js <manifest.tsx>\n');
    process.exit(2);
  }
  let result: ReturnType<typeof parseWorkspace>;
  if (path.basename(manifestPath) === 'package.manifest.tsx') {
    result = parsePackageManifestFile(manifestPath);
  } else {
    result = parseWorkspace(manifestPath);
  }
  process.stdout.write(`${JSON.stringify(result)}\n`);
  if (!result.ok) {
    process.exit(1);
  }
}

main();
