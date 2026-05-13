import fs from 'node:fs';
import path from 'node:path';

export type PlaywrightCoreProvider = {
  modulePath: string;
  source: 'env' | 'local' | 'vscode-bundled';
};

const VSCODE_CANDIDATES = [
  '/usr/share/code/resources/app/node_modules/playwright-core',
  '/usr/share/code-insiders/resources/app/node_modules/playwright-core',
  '/usr/share/codium/resources/app/node_modules/playwright-core',
  '/usr/share/code-oss/resources/app/node_modules/playwright-core'
];

function hasPackageJson(modulePath: string): boolean {
  return fs.existsSync(path.join(modulePath, 'package.json'));
}

export function resolvePlaywrightCoreProvider(projectRoot = process.cwd()): PlaywrightCoreProvider {
  const envPath = process.env.TSPACK_PLAYWRIGHT_CORE_PATH;
  if (envPath) {
    if (hasPackageJson(envPath)) {
      return { modulePath: envPath, source: 'env' };
    }
    throw new Error('TSPACK_INSPECT_PLAYWRIGHT_CORE_NOT_FOUND');
  }

  const localCandidates = [
    path.join(projectRoot, 'node_modules', 'playwright-core'),
    path.join(projectRoot, 'node_modules', 'playwright')
  ];
  for (const candidate of localCandidates) {
    if (hasPackageJson(candidate)) {
      return { modulePath: candidate, source: 'local' };
    }
  }

  for (const candidate of VSCODE_CANDIDATES) {
    if (hasPackageJson(candidate)) {
      return { modulePath: candidate, source: 'vscode-bundled' };
    }
  }

  throw new Error('TSPACK_INSPECT_PLAYWRIGHT_CORE_NOT_FOUND');
}

export async function loadPlaywrightCore(provider: PlaywrightCoreProvider): Promise<any> {
  try {
    return await import(path.join(provider.modulePath, 'index.js'));
  } catch {
    throw new Error('TSPACK_INSPECT_PLAYWRIGHT_CORE_LOAD_FAILED');
  }
}
