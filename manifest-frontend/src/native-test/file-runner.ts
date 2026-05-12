import fs from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import ts from 'typescript';
import { discoverNativeTestFiles } from './discover.js';
import { expect } from './expect.js';
import { assert } from './assert.js';
import { runSuite } from './runner.js';
import type { Diagnostic, RunFilesOptions, RunFilesResult, TestResult } from './types.js';

type RuntimeNode = {
  __tag: 'Suite' | 'Fact' | 'Theory' | 'Case';
  props: Record<string, unknown>;
  children: unknown[];
};

export async function runNativeTestFiles(options: RunFilesOptions): Promise<RunFilesResult> {
  const discovered = discoverNativeTestFiles({ rootDir: options.rootDir });
  const diagnostics: Diagnostic[] = [...discovered.diagnostics];
  const results: TestResult[] = [];
  const selected = discovered.files.filter((file) => {
    if (!options.files || options.files.length === 0) {
      return true;
    }
    return options.files.some((candidate) => path.resolve(options.rootDir, candidate) === file.filePath || candidate === file.filePath);
  });

  if (options.listOnly) {
    for (const file of selected) {
      for (const test of file.tests) {
        if (options.filter && !test.id.includes(options.filter) && !test.name.includes(options.filter)) {
          continue;
        }
        results.push({ id: test.id, name: test.name, status: 'passed' });
      }
    }
    return { results, diagnostics };
  }

  for (const file of selected) {
    try {
      const root = await loadRuntimeSuite(file.filePath);
      const runResults = await runSuite(root);
      for (const result of runResults) {
        const fullId = `${path.relative(options.rootDir, file.filePath).split(path.sep).join('/')}::${result.id}`;
        if (options.filter && !fullId.includes(options.filter) && !result.name.includes(options.filter)) {
          continue;
        }
        results.push({ ...result, id: fullId });
      }
    } catch (error) {
      diagnostics.push({
        code: 'TSPACK_TEST_MODULE_LOAD_FAILED',
        message: `failed to load module ${file.filePath}: ${(error as Error).message}`,
        file: file.filePath,
      });
    }
  }

  return { results, diagnostics };
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

  const prelude = `const __tspackJsx = (type, props, ...children) => {\n  if (typeof type === 'function') return type(props ?? {}, ...children);\n  return { __tag: String(type), props: props ?? {}, children };\n};\nconst makeTag = (tag) => (props, ...children) => ({ __tag: tag, props: props ?? {}, children });\nconst Suite = makeTag('Suite');\nconst Fact = makeTag('Fact');\nconst Theory = makeTag('Theory');\nconst Case = makeTag('Case');\nconst assert = globalThis.__tspackAssert;\nconst expect = globalThis.__tspackExpect;\n`;
  const tempFile = path.join(path.dirname(filePath), `${path.basename(filePath)}.tspack-temp.mjs`);
  (globalThis as Record<string, unknown>).__tspackAssert = assert;
  (globalThis as Record<string, unknown>).__tspackExpect = expect;
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
