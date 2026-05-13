#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';

const vscodeCandidates = [
  '/usr/share/code/resources/app/node_modules/playwright-core',
  '/usr/share/code-insiders/resources/app/node_modules/playwright-core',
  '/usr/share/codium/resources/app/node_modules/playwright-core',
  '/usr/share/code-oss/resources/app/node_modules/playwright-core'
];

function findProvider() {
  const envPath = process.env.TSPACK_PLAYWRIGHT_CORE_PATH;
  if (envPath && fs.existsSync(path.join(envPath, 'package.json'))) {
    return { modulePath: envPath, source: 'env' };
  }

  const localCandidates = [
    path.join(process.cwd(), 'node_modules', 'playwright-core'),
    path.join(process.cwd(), 'node_modules', 'playwright')
  ];

  for (const candidate of localCandidates) {
    if (fs.existsSync(path.join(candidate, 'package.json'))) {
      return { modulePath: candidate, source: 'local' };
    }
  }

  for (const candidate of vscodeCandidates) {
    if (fs.existsSync(path.join(candidate, 'package.json'))) {
      return { modulePath: candidate, source: 'vscode-bundled' };
    }
  }

  return null;
}

function errorText(error) {
  return error instanceof Error ? error.message : String(error);
}

async function main() {
  const provider = findProvider();
  console.log('Detected playwright-core provider candidates:');
  console.log(JSON.stringify({ envPath: process.env.TSPACK_PLAYWRIGHT_CORE_PATH ?? null, vscodeCandidates }, null, 2));

  if (!provider) {
    console.log('TSPACK_INSPECT_PLAYWRIGHT_CORE_NOT_FOUND');
    process.exit(0);
  }

  console.log(`Using provider: ${provider.source} -> ${provider.modulePath}`);

  let playwright;
  try {
    playwright = await import(path.join(provider.modulePath, 'index.js'));
  } catch (error) {
    console.log(`TSPACK_INSPECT_PLAYWRIGHT_CORE_LOAD_FAILED: ${errorText(error)}`);
    process.exit(0);
  }

  const packageJsonPath = path.join(provider.modulePath, 'package.json');
  const version = fs.existsSync(packageJsonPath)
    ? JSON.parse(fs.readFileSync(packageJsonPath, 'utf8')).version
    : 'unknown';
  console.log(`playwright-core version: ${version}`);

  const api = playwright.default ?? playwright;
  const exportsMap = {
    chromium: Boolean(api.chromium),
    firefox: Boolean(api.firefox),
    webkit: Boolean(api.webkit),
    _electron: Boolean(api._electron)
  };
  console.log('Exports:');
  console.log(JSON.stringify(exportsMap, null, 2));

  const cdpEndpoint = process.env.TSPACK_INSPECT_CDP_ENDPOINT;
  if (cdpEndpoint && api.chromium) {
    try {
      const browser = await api.chromium.connectOverCDP(cdpEndpoint);
      await browser.close();
      console.log(`connectOverCDP success: ${cdpEndpoint}`);
    } catch (error) {
      console.log(`connectOverCDP failed: ${errorText(error)}`);
    }
  } else {
    console.log('connectOverCDP skipped: set TSPACK_INSPECT_CDP_ENDPOINT to probe explicit endpoint.');
  }

  const executablePath = '/usr/share/code/code';
  const launchArgs = ['--no-sandbox', '--disable-gpu', '--disable-dev-shm-usage'];
  if (api.chromium && fs.existsSync(executablePath)) {
    try {
      const browser = await api.chromium.launch({ executablePath, headless: true, args: launchArgs });
      const context = await browser.newContext();
      const page = await context.newPage();
      await page.goto('about:blank');
      await browser.close();
      console.log('chromium.launch success with /usr/share/code/code');
    } catch (error) {
      console.log(`chromium.launch failed: ${errorText(error)}`);
    }
  } else {
    console.log('chromium.launch skipped: chromium export missing or /usr/share/code/code missing.');
  }

  if (api._electron && fs.existsSync(executablePath)) {
    try {
      const app = await api._electron.launch({ executablePath, args: launchArgs });
      const windows = await app.windows();
      console.log(`_electron.launch success; windows=${windows.length}`);
      await app.close();
    } catch (error) {
      console.log(`_electron.launch failed: ${errorText(error)}`);
    }
  } else {
    console.log('_electron.launch skipped: _electron export missing or /usr/share/code/code missing.');
  }
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
