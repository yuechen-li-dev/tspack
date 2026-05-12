import fs from 'node:fs';
import path from 'node:path';
import ts from 'typescript';
import type { Diagnostic, DiscoverFilesResult, DiscoverOptions, DiscoveredFile, DiscoveryResult, Literal } from './types.js';

const allowed = new Set(['Suite', 'Fact', 'Theory', 'Case']);

export function discoverNativeTestFile(filePath: string): DiscoveryResult {
  const abs = path.resolve(filePath);
  if (!abs.endsWith('.xtest.tsx')) {
    return {
      tests: [],
      facts: [],
      theories: [],
      diagnostics: [{ code: 'TSPACK_TEST_NON_NATIVE_FILE', message: 'native test files must end with .xtest.tsx', file: abs }],
    };
  }
  const text = fs.readFileSync(abs, 'utf8');
  const sf = ts.createSourceFile(abs, text, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const diagnostics: Diagnostic[] = [];

  const addDiag = (node: ts.Node, code: string, message: string): void => {
    const lc = sf.getLineAndCharacterOfPosition(node.getStart(sf));
    diagnostics.push({ code, message, file: abs, line: lc.line + 1, column: lc.character + 1 });
  };

  let exportExpr: ts.Expression | undefined;
  for (const statement of sf.statements) {
    if (ts.isExportAssignment(statement)) {
      exportExpr = statement.expression;
    }
  }

  if (!exportExpr) {
    diagnostics.push({ code: 'TSPACK_TEST_INVALID_DEFAULT_EXPORT', message: 'default export must be a Suite JSX tree', file: abs });
    return { diagnostics, tests: [], facts: [], theories: [] };
  }

  const root = unwrap(exportExpr);
  if (!ts.isJsxElement(root) && !ts.isJsxSelfClosingElement(root)) {
    diagnostics.push({ code: 'TSPACK_TEST_INVALID_DEFAULT_EXPORT', message: 'default export must be JSX', file: abs });
    return { diagnostics, tests: [], facts: [], theories: [] };
  }

  const tests: string[] = [];
  const facts: DiscoveryResult['facts'] = [];
  const theories: DiscoveryResult['theories'] = [];

  const suiteName = parseName(getTagName(root), getAttributes(root), root, addDiag);
  if (getTagName(root) !== 'Suite') {
    addDiag(root, 'TSPACK_TEST_INVALID_DEFAULT_EXPORT', 'root element must be <Suite>');
  }

  for (const child of getChildren(root)) {
    if (!ts.isJsxElement(child) && !ts.isJsxSelfClosingElement(child)) {
      continue;
    }
    const tag = getTagName(child);
    if (!allowed.has(tag)) {
      addDiag(child, 'TSPACK_TEST_UNKNOWN_ELEMENT', `unknown element: ${tag}`);
      continue;
    }
    if (tag === 'Fact') {
      const factName = parseName(tag, getAttributes(child), child, addDiag);
      const body = getJsxBodyFunction(child);
      if (!body) {
        addDiag(child, 'TSPACK_TEST_MISSING_BODY', 'Fact requires callback body');
        continue;
      }
      const id = `${suiteName}/${factName}`;
      facts.push({ kind: 'fact', name: factName, id });
      tests.push(id);
    }
    if (tag === 'Theory') {
      const theoryName = parseName(tag, getAttributes(child), child, addDiag);
      const body = getJsxBodyFunction(child);
      if (!body) {
        addDiag(child, 'TSPACK_TEST_MISSING_BODY', 'Theory requires callback body');
        continue;
      }
      const cases = collectCases(child, suiteName, theoryName, addDiag);
      theories.push({ kind: 'theory', name: theoryName, cases });
      for (const entry of cases) {
        tests.push(entry.id);
      }
    }
  }

  return { suiteName, tests, facts, theories, diagnostics };
}

const defaultIgnore = new Set(['node_modules', '.git', 'dist']);

export function discoverNativeTestFiles(options: DiscoverOptions): DiscoverFilesResult {
  const rootDir = path.resolve(options.rootDir);
  const filePaths = collectNativeFiles(rootDir, options.ignore ?? []);
  const files: DiscoveredFile[] = [];
  const diagnostics: Diagnostic[] = [];

  for (const filePath of filePaths) {
    try {
      const discovered = discoverNativeTestFile(filePath);
      const relativeFilePath = path.relative(rootDir, filePath).split(path.sep).join('/');
      const tests = [
        ...discovered.facts.map((fact) => ({ id: `${relativeFilePath}::${fact.id}`, name: fact.name, kind: 'fact' as const, filePath })),
        ...discovered.theories.flatMap((theory) => theory.cases.map((c) => ({ id: `${relativeFilePath}::${c.id}`, name: theory.name, kind: 'theory' as const, filePath }))),
      ];
      files.push({ filePath, suiteName: discovered.suiteName ?? '', tests, diagnostics: discovered.diagnostics });
      diagnostics.push(...discovered.diagnostics);
    } catch (error) {
      diagnostics.push({
        code: 'TSPACK_TEST_DISCOVERY_FAILED',
        message: `failed to discover native test file: ${(error as Error).message}`,
        file: filePath,
      });
    }
  }

  files.sort((a, b) => a.filePath.localeCompare(b.filePath));
  diagnostics.sort((a, b) => `${a.file}:${a.line ?? 0}:${a.column ?? 0}:${a.code}:${a.message}`.localeCompare(`${b.file}:${b.line ?? 0}:${b.column ?? 0}:${b.code}:${b.message}`));
  return { files, diagnostics };
}

function collectNativeFiles(rootDir: string, ignore: string[]): string[] {
  const entries = fs.readdirSync(rootDir, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name));
  const filePaths: string[] = [];
  const ignoreSet = new Set(ignore);

  for (const entry of entries) {
    const fullPath = path.join(rootDir, entry.name);
    if (entry.isDirectory()) {
      if (defaultIgnore.has(entry.name) || ignoreSet.has(entry.name)) {
        continue;
      }
      filePaths.push(...collectNativeFiles(fullPath, ignore));
      continue;
    }
    if (entry.isFile() && entry.name.endsWith('.xtest.tsx')) {
      filePaths.push(fullPath);
    }
  }
  return filePaths;
}

function unwrap(expr: ts.Expression): ts.Expression {
  if (ts.isParenthesizedExpression(expr)) {
    return unwrap(expr.expression);
  }
  return expr;
}
function getTagName(node: ts.JsxElement | ts.JsxSelfClosingElement): string {
  return ts.isJsxElement(node) ? node.openingElement.tagName.getText() : node.tagName.getText();
}
function getAttributes(node: ts.JsxElement | ts.JsxSelfClosingElement): readonly ts.JsxAttributeLike[] {
  return ts.isJsxElement(node) ? node.openingElement.attributes.properties : node.attributes.properties;
}
function getChildren(node: ts.JsxElement | ts.JsxSelfClosingElement): ts.Node[] {
  return ts.isJsxElement(node) ? [...node.children] : [];
}
function parseName(tag: string, attrs: readonly ts.JsxAttributeLike[], node: ts.Node, addDiag: (node: ts.Node, code: string, message: string) => void): string {
  for (const attr of attrs) {
    if (ts.isJsxSpreadAttribute(attr)) {
      addDiag(attr, 'TSPACK_TEST_FORBIDDEN_SPREAD', 'spread attributes are forbidden');
      continue;
    }
    if (!ts.isJsxAttribute(attr) || attr.name.getText() !== 'name') {
      continue;
    }
    const init = attr.initializer;
    if (!init || !ts.isStringLiteral(init)) {
      addDiag(attr, 'TSPACK_TEST_INVALID_NAME', `${tag} name must be string literal`);
      return '';
    }
    return init.text;
  }
  addDiag(node, 'TSPACK_TEST_INVALID_NAME', `${tag} name is required`);
  return '';
}
function literalFrom(expr: ts.Expression): Literal | undefined {
  if (ts.isStringLiteral(expr) || ts.isNoSubstitutionTemplateLiteral(expr)) return expr.text;
  if (ts.isNumericLiteral(expr)) return Number(expr.text);
  if (expr.kind === ts.SyntaxKind.TrueKeyword) return true;
  if (expr.kind === ts.SyntaxKind.FalseKeyword) return false;
  if (expr.kind === ts.SyntaxKind.NullKeyword) return null;
  return undefined;
}
function collectCases(node: ts.JsxElement | ts.JsxSelfClosingElement, suite: string, theory: string, addDiag: (node: ts.Node, code: string, message: string) => void) {
  const out: Array<{ index: number; data: Record<string, Literal>; id: string }> = [];
  for (const child of getChildren(node)) {
    if (!ts.isJsxElement(child) && !ts.isJsxSelfClosingElement(child)) {
      continue;
    }
    if (getTagName(child) !== 'Case') {
      continue;
    }
    const data: Record<string, Literal> = {};
    let valid = true;
    for (const attr of getAttributes(child)) {
      if (ts.isJsxSpreadAttribute(attr)) {
        addDiag(attr, 'TSPACK_TEST_FORBIDDEN_SPREAD', 'spread attributes are forbidden');
        valid = false;
        continue;
      }
      if (!ts.isJsxAttribute(attr) || !attr.initializer) {
        addDiag(child, 'TSPACK_TEST_INVALID_CASE', 'invalid case attribute');
        valid = false;
        continue;
      }
      if (ts.isStringLiteral(attr.initializer)) {
        data[attr.name.getText()] = attr.initializer.text;
        continue;
      }
      if (ts.isJsxExpression(attr.initializer) && attr.initializer.expression) {
        const value = literalFrom(attr.initializer.expression);
        if (value === undefined) {
          addDiag(attr, 'TSPACK_TEST_INVALID_CASE', 'case props must be literal values');
          valid = false;
        } else {
          data[attr.name.getText()] = value;
        }
        continue;
      }
      addDiag(attr, 'TSPACK_TEST_INVALID_CASE', 'case props must be literals');
      valid = false;
    }
    if (valid) {
      const index = out.length;
      out.push({ index, data, id: `${suite}/${theory}[${index}]` });
    }
  }
  return out;
}
function getJsxBodyFunction(node: ts.JsxElement | ts.JsxSelfClosingElement): boolean {
  return getChildren(node).some((child) => ts.isJsxExpression(child) && !!child.expression && ts.isArrowFunction(child.expression));
}
