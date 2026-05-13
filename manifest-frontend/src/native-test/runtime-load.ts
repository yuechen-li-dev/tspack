import fs from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import ts from 'typescript';
import { assert } from './assert.js';
import { expect } from './expect.js';
import { skip } from './skip.js';

export async function loadRuntimeSuiteForFile(filePath: string): Promise<any> {
  const source = fs.readFileSync(filePath, 'utf8');
  const compiled = ts.transpileModule(source, { fileName: filePath, compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext, jsx: ts.JsxEmit.React, jsxFactory: '__tspackJsx' } });
  const prelude = `const __tspackJsx = (type, props, ...children) => { if (typeof type === 'function') return type(props ?? {}, ...children); return { __tag: String(type), props: props ?? {}, children }; };
const makeTag = (tag) => (props, ...children) => ({ __tag: tag, props: props ?? {}, children });
const Suite = makeTag('Suite'); const Fact = makeTag('Fact'); const Theory = makeTag('Theory'); const Case = makeTag('Case'); const Artifact = makeTag('Artifact'); const Valid = makeTag('Valid'); const Invalid = makeTag('Invalid'); const Project = makeTag('Project'); const CycleTime = makeTag('CycleTime'); const Benchmark = makeTag('Benchmark'); const Iterations = makeTag('Iterations'); const Warmup = makeTag('Warmup');
const assert = globalThis.__tspackAssert; const expect = globalThis.__tspackExpect; const skip = globalThis.__tspackSkip;`;
  const tempFile = path.join(path.dirname(filePath), `${path.basename(filePath)}.tspack-temp.mjs`);
  (globalThis as Record<string, unknown>).__tspackAssert = assert;
  (globalThis as Record<string, unknown>).__tspackExpect = expect;
  (globalThis as Record<string, unknown>).__tspackSkip = skip;
  fs.writeFileSync(tempFile, `${prelude}\n${compiled.outputText}`);
  try {
    const mod = await import(pathToFileURL(tempFile).href);
    return mod.default;
  } finally {
    fs.rmSync(tempFile, { force: true });
  }
}
