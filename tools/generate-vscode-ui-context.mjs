import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const toolsDirectory = path.dirname(fileURLToPath(import.meta.url));
const repositoryRoot = path.resolve(toolsDirectory, '..');
const canonicalPath = path.join(
  repositoryRoot,
  'manifest-frontend',
  'src',
  'inspect',
  'context-bundle.ts',
);
const extensionPath = path.join(
  repositoryRoot,
  'extensions',
  'tspack-vscode',
  'src',
  'uiContextBundle.ts',
);

const canonical = fs.readFileSync(canonicalPath, 'utf8');
const semanticStart = canonical.indexOf('export type UIContextBundleDiagnostic');
if (semanticStart < 0) {
  throw new Error('canonical UI context bundle semantic start was not found');
}

const prefix = `// Code generated from manifest-frontend/src/inspect/context-bundle.ts.
// Run: node tools/generate-vscode-ui-context.mjs
import * as fs from 'node:fs/promises';
import * as path from 'node:path';
import type {
  InspectBounds,
  InspectDiagnostic,
  InspectNode,
  InspectResult,
  InspectSourceHint,
} from './inspectTypes';
import {
  isSourceHintMalformed,
  resolveSourceHintPath,
  type SourcePathResolution,
} from './revealSource';

`;

const generated = `${prefix}${canonical.slice(semanticStart)}`;
const checkOnly = process.argv.includes('--check');
if (checkOnly) {
  const existing = fs.readFileSync(extensionPath, 'utf8');
  if (existing !== generated) {
    process.stderr.write(
      'VS Code UI context bundle is stale; run node tools/generate-vscode-ui-context.mjs\n',
    );
    process.exit(1);
  }
  process.stdout.write('VS Code UI context bundle matches canonical source.\n');
} else {
  fs.writeFileSync(extensionPath, generated);
  process.stdout.write('Generated VS Code UI context bundle.\n');
}
