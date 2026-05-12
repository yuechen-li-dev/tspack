import fs from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import ts from 'typescript';
import { discoverNativeTestFiles } from './discover.js';
import { expect } from './expect.js';
import { assert } from './assert.js';
import { skip } from './skip.js';
import { runSuite } from './runner.js';
import type { Diagnostic, DiscoveredFile, RunFilesOptions, RunFilesResult, TestResult } from './types.js';

type RuntimeNode = {
  __tag: 'Suite' | 'Fact' | 'Theory' | 'Case' | 'Artifact';
  props: Record<string, unknown>;
  children: unknown[];
};

export async function runNativeTestFiles(options: RunFilesOptions): Promise<RunFilesResult> {
  const discovered = discoverNativeTestFiles({ rootDir: options.rootDir });
  const diagnostics: Diagnostic[] = [...discovered.diagnostics];
  const results: TestResult[] = [];
  const selectedFiles = filterByFileSelection(discovered.files, options);
  const runnableFiles = filterByTestSelection(selectedFiles, options.filter, diagnostics, options.rootDir);

  if (options.listOnly) {
    for (const file of runnableFiles) {
      for (const test of file.tests) {
        if (!matchesFilter(test.id, test.name, options.filter)) {
          continue;
        }
        results.push({ id: test.id, name: test.name, status: 'passed' });
      }
    }
    return { results, diagnostics };
  }

  for (const file of runnableFiles) {
    try {
      const root = await loadRuntimeSuite(file.filePath);
      const artifactRoot = options.artifactRoot ?? path.join(options.rootDir, '.tspack', 'test-artifacts');
      const runResults = await runSuite(root, { artifactRoot });
      for (const result of runResults) {
        const fullId = `${path.relative(options.rootDir, file.filePath).split(path.sep).join('/')}::${result.id}`;
        if (!matchesFilter(fullId, result.name, options.filter)) {
          continue;
        }
        results.push({ ...result, id: fullId });
      }
    } catch (error) {
      diagnostics.push({
        code: 'TSPACK_TEST_MODULE_LOAD_FAILED',
        message: `failed to load module ${file.filePath}: ${(error as Error).message}`,
        file: file.filePath,
        severity: 'error',
      });
    }
  }

  return { results, diagnostics };
}

function filterByFileSelection(files: DiscoveredFile[], options: RunFilesOptions): DiscoveredFile[] {
  return files.filter((file) => {
    if (!options.files || options.files.length === 0) {
      return true;
    }
    return options.files.some((candidate) => path.resolve(options.rootDir, candidate) === file.filePath || candidate === file.filePath);
  });
}

function filterByTestSelection(files: DiscoveredFile[], filter: string | undefined, diagnostics: Diagnostic[], rootDir: string): DiscoveredFile[] {
  if (!filter) {
    return files;
  }

  const matchedFiles = files.filter((file) => file.tests.some((test) => matchesFilter(test.id, test.name, filter)));
  const matchedCount = matchedFiles.reduce((total, file) => total + file.tests.filter((test) => matchesFilter(test.id, test.name, filter)).length, 0);

  if (matchedCount === 0) {
    diagnostics.push({
      code: 'TSPACK_TEST_FILTER_NO_MATCH',
      message: `native test filter matched no tests: ${filter}`,
      file: path.resolve(rootDir),
      severity: 'error',
    });
  }

  return matchedFiles;
}

function matchesFilter(id: string, name: string, filter: string | undefined): boolean {
  if (!filter) {
    return true;
  }
  return id.includes(filter) || name.includes(filter);
}

async function loadRuntimeSuite(filePath: string): Promise<RuntimeNode> {
  const source = fs.readFileSync(filePath, 'utf8');
  const compiled = ts.transpileModule(source, {
    fileName: filePath,
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ESNext,
      jsx: ts.JsxEmit.React,
      jsxFactory: '__tspackJsx',
    },
  });

  const prelude = `const __tspackJsx = (type, props, ...children) => {\n  if (typeof type === 'function') return type(props ?? {}, ...children);\n  return { __tag: String(type), props: props ?? {}, children };\n};\nconst makeTag = (tag) => (props, ...children) => ({ __tag: tag, props: props ?? {}, children });\nconst Suite = makeTag('Suite');\nconst Fact = makeTag('Fact');\nconst Theory = makeTag('Theory');\nconst Case = makeTag('Case');
const Artifact = makeTag('Artifact');\nconst assert = globalThis.__tspackAssert;\nconst expect = globalThis.__tspackExpect;\nconst skip = globalThis.__tspackSkip;\n`;
  const tempFile = path.join(path.dirname(filePath), `${path.basename(filePath)}.tspack-temp.mjs`);
  (globalThis as Record<string, unknown>).__tspackAssert = assert;
  (globalThis as Record<string, unknown>).__tspackExpect = expect;
  (globalThis as Record<string, unknown>).__tspackSkip = skip;
  fs.writeFileSync(tempFile, `${prelude}${compiled.outputText}`);

  try {
    const mod = await import(pathToFileURL(tempFile).href);
    const root = mod.default as RuntimeNode;
    if (!root || root.__tag !== 'Suite') {
      throw new Error('default export must evaluate to Suite runtime node');
    }
    return root;
  } finally {
    fs.rmSync(tempFile, { force: true });
  }
}
