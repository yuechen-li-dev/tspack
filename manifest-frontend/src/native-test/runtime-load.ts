import fs from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import ts from 'typescript';
import { assert } from './assert.js';
import { expect } from './expect.js';
import { skip } from './skip.js';
import { inspect } from './inspect.js';
import { runScript as runLifecycleScript } from './lifecycle.js';

export async function loadRuntimeSuiteForFile(filePath: string, options?: { rootDir?: string }): Promise<any> {
  const rootDir = path.resolve(options?.rootDir ?? path.dirname(filePath));
  const prelude = `const __tspackJsx = (type, props, ...children) => { if (typeof type === 'function') return type(props ?? {}, ...children); return { __tag: String(type), props: props ?? {}, children }; };
const makeTag = (tag) => (props, ...children) => ({ __tag: tag, props: props ?? {}, children });
const Suite = makeTag('Suite'); const Fact = makeTag('Fact'); const Theory = makeTag('Theory'); const Case = makeTag('Case'); const Artifact = makeTag('Artifact'); const Valid = makeTag('Valid'); const Invalid = makeTag('Invalid'); const Project = makeTag('Project'); const CycleTime = makeTag('CycleTime'); const Benchmark = makeTag('Benchmark'); const Iterations = makeTag('Iterations'); const Warmup = makeTag('Warmup');
const assert = globalThis.__tspackAssert; const expect = globalThis.__tspackExpect; const skip = globalThis.__tspackSkip; const inspect = globalThis.__tspackInspect;
const runLifecycleScript = globalThis.__tspackRunLifecycleScript;`;
  const tempRoot = fs.mkdtempSync(path.join(path.dirname(filePath), '.tspack-runtime-'));
  const generated = materializeLocalModuleClosure(filePath, rootDir, tempRoot);
  const tempFile = generated.entryFile;
  (globalThis as Record<string, unknown>).__tspackAssert = assert;
  (globalThis as Record<string, unknown>).__tspackExpect = expect;
  (globalThis as Record<string, unknown>).__tspackSkip = skip;
  (globalThis as Record<string, unknown>).__tspackInspect = inspect;
  (globalThis as Record<string, unknown>).__tspackRunLifecycleScript = runLifecycleScript;
  fs.writeFileSync(tempFile, `${prelude}\n${fs.readFileSync(tempFile, 'utf8')}`);
  try {
    const mod = await import(pathToFileURL(tempFile).href);
    return mod.default;
  } finally {
    fs.rmSync(tempRoot, { recursive: true, force: true });
  }
}

function materializeLocalModuleClosure(entryFile: string, rootDir: string, tempRoot: string): { entryFile: string } {
  const root = path.resolve(rootDir);
  const seen = new Set<string>();
  const stack: string[] = [path.resolve(entryFile)];

  while (stack.length > 0) {
    const filePath = stack.pop() as string;
    if (seen.has(filePath)) {
      continue;
    }
    seen.add(filePath);

    assertInsideRoot(filePath, root);
    const source = fs.readFileSync(filePath, 'utf8');
    const transformed = ts.transform(ts.createSourceFile(filePath, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX), [
      (context) => (node) => ts.visitNode(node, function visit(current): ts.Node {
        if (ts.isImportDeclaration(current) || ts.isExportDeclaration(current)) {
          const moduleSpecifier = current.moduleSpecifier;
          if (moduleSpecifier && ts.isStringLiteral(moduleSpecifier)) {
            const rewritten = rewriteSpecifier(moduleSpecifier.text, filePath, root);
            if (rewritten.resolvedPath) {
              stack.push(rewritten.resolvedPath);
            }
            if (rewritten.nextSpecifier !== moduleSpecifier.text) {
              if (ts.isImportDeclaration(current)) {
                return ts.factory.updateImportDeclaration(current, current.modifiers, current.importClause, ts.factory.createStringLiteral(rewritten.nextSpecifier), current.assertClause);
              }
              return ts.factory.updateExportDeclaration(current, current.modifiers, current.isTypeOnly, current.exportClause, ts.factory.createStringLiteral(rewritten.nextSpecifier), current.assertClause);
            }
          }
        }
        return ts.visitEachChild(current, visit, context);
      }),
    ]);

    const printed = ts.createPrinter().printFile(transformed.transformed[0] as ts.SourceFile);
    const compiled = ts.transpileModule(printed, { fileName: filePath, compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext, jsx: ts.JsxEmit.React, jsxFactory: '__tspackJsx' } });
    const outFile = tempFilePath(filePath, root, tempRoot);
    fs.mkdirSync(path.dirname(outFile), { recursive: true });
    fs.writeFileSync(outFile, compiled.outputText);
  }

  return { entryFile: tempFilePath(path.resolve(entryFile), root, tempRoot) };
}

function rewriteSpecifier(specifier: string, fromFile: string, root: string): { nextSpecifier: string; resolvedPath?: string } {
  if (!specifier.startsWith('./') && !specifier.startsWith('../')) {
    return { nextSpecifier: specifier };
  }
  const resolved = resolveLocalImport(fromFile, specifier);
  assertInsideRoot(resolved, root);
  const targetRel = relativeImportPath(tempFilePath(fromFile, root, ''), tempFilePath(resolved, root, ''));
  return { nextSpecifier: targetRel, resolvedPath: resolved };
}

function resolveLocalImport(fromFile: string, specifier: string): string {
  const base = path.resolve(path.dirname(fromFile), specifier);
  const candidates = path.extname(base) ? [base] : [base + '.ts', base + '.tsx', base + '.js', base + '.jsx', path.join(base, 'index.ts'), path.join(base, 'index.tsx'), path.join(base, 'index.js'), path.join(base, 'index.jsx')];
  const hit = candidates.find((candidate) => fs.existsSync(candidate) && fs.statSync(candidate).isFile());
  if (!hit) {
    throw new Error(`TSPACK_TEST_IMPORT_NOT_FOUND: ${specifier}`);
  }
  if (!/\.(ts|tsx|js|jsx)$/i.test(hit)) {
    throw new Error(`TSPACK_TEST_UNSUPPORTED_IMPORT: ${specifier}`);
  }
  return hit;
}

function assertInsideRoot(filePath: string, root: string): void {
  const rel = path.relative(root, filePath);
  if (rel.startsWith('..') || path.isAbsolute(rel)) {
    throw new Error(`TSPACK_TEST_IMPORT_OUTSIDE_ROOT: ${filePath}`);
  }
}

function tempFilePath(sourcePath: string, root: string, tempRoot: string): string {
  const rel = path.relative(root, sourcePath);
  const outRel = rel.replace(/\.(ts|tsx|js|jsx)$/i, '.mjs');
  return path.resolve(tempRoot, outRel);
}

function relativeImportPath(fromFile: string, toFile: string): string {
  const rel = path.relative(path.dirname(fromFile), toFile).split(path.sep).join('/');
  if (rel.startsWith('.')) return rel;
  return `./${rel}`;
}
