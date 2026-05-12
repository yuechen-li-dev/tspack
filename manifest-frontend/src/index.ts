import fs from 'node:fs';
import path from 'node:path';
import ts from 'typescript';

export type Diagnostic = {
  code: string;
  severity: 'error' | 'warning';
  message: string;
  file: string;
  line?: number;
  column?: number;
};

export type ManifestParseResult = {
  ok: boolean;
  ir?: ManifestIr;
  diagnostics: Diagnostic[];
};

export type ManifestIr = {
  format: 1;
  workspace: { name: string };
  packages: Array<Record<string, unknown>>;
};

const ALLOWED_IMPORT = 'tspack/manifest';
const APPROVED_HELPERS = new Set(['define', 'defineDeps', 'npm', 'git', 'path', 'workspace', 'dep', 'peer', 'tool']);
const APPROVED_ELEMENTS = new Set(['Workspace', 'Package', 'Policies', 'Targets', 'Tools', 'Boundaries', 'Publish']);

export function parseManifestFile(filePath: string): ManifestParseResult {
  const diagnostics: Diagnostic[] = [];
  const normalizedPath = path.resolve(filePath);
  if (path.basename(normalizedPath) !== 'manifest.tsx' || path.dirname(normalizedPath) !== path.dirname(path.dirname(normalizedPath)) + path.sep + path.basename(path.dirname(normalizedPath))) {
    // fallback robust root check below
  }
  if (path.basename(normalizedPath) !== 'manifest.tsx') {
    diagnostics.push({ code: 'TSPACK_MANIFEST_NON_ROOT', severity: 'error', message: 'manifest must be root-level manifest.tsx', file: normalizedPath });
  }

  const sourceText = fs.readFileSync(normalizedPath, 'utf8');
  const sf = ts.createSourceFile(normalizedPath, sourceText, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const constEnv = new Map<string, unknown>();

  const addDiag = (node: ts.Node, code: string, message: string) => {
    const lc = sf.getLineAndCharacterOfPosition(node.getStart(sf));
    diagnostics.push({ code, severity: 'error', message, file: normalizedPath, line: lc.line + 1, column: lc.character + 1 });
  };

  const walk = (node: ts.Node) => {
    if (ts.isImportDeclaration(node)) {
      const mod = (node.moduleSpecifier as ts.StringLiteral).text;
      if (mod !== ALLOWED_IMPORT) addDiag(node, 'TSPACK_MANIFEST_FORBIDDEN_IMPORT', `Only imports from "${ALLOWED_IMPORT}" are allowed.`);
    }
    if (ts.isFunctionDeclaration(node) || ts.isFunctionExpression(node) || ts.isArrowFunction(node)) addDiag(node, 'TSPACK_MANIFEST_FORBIDDEN_FUNCTION', 'Functions are forbidden in manifest subset.');
    if (ts.isClassDeclaration(node)) addDiag(node, 'TSPACK_MANIFEST_FORBIDDEN_CLASS', 'Classes are forbidden in manifest subset.');
    if (ts.isForStatement(node) || ts.isForInStatement(node) || ts.isForOfStatement(node) || ts.isWhileStatement(node) || ts.isDoStatement(node)) addDiag(node, 'TSPACK_MANIFEST_FORBIDDEN_LOOP', 'Loops are forbidden in manifest subset.');
    if (ts.isIfStatement(node) || ts.isConditionalExpression(node) || ts.isSwitchStatement(node)) addDiag(node, 'TSPACK_MANIFEST_FORBIDDEN_CONDITIONAL', 'Conditionals are forbidden in manifest subset.');
    if (ts.isAwaitExpression(node) || (ts.isFunctionLike(node) && node.modifiers?.some((m) => m.kind === ts.SyntaxKind.AsyncKeyword))) addDiag(node, 'TSPACK_MANIFEST_FORBIDDEN_ASYNC', 'Async/await is forbidden in manifest subset.');
    if (ts.isPropertyAccessExpression(node) && node.getText(sf) === 'process.env') addDiag(node, 'TSPACK_MANIFEST_FORBIDDEN_PROCESS_ENV', 'process.env access is forbidden.');
    if (ts.isIdentifier(node) && node.text === 'fetch') addDiag(node, 'TSPACK_MANIFEST_FORBIDDEN_FETCH', 'fetch access is forbidden.');
    if (ts.isPropertyAccessExpression(node) && (node.expression.getText(sf) === 'Date' || node.expression.getText(sf) === 'Math') && (node.name.text === 'now' || node.name.text === 'random')) {
      addDiag(node, node.name.text === 'now' ? 'TSPACK_MANIFEST_FORBIDDEN_DATE' : 'TSPACK_MANIFEST_FORBIDDEN_RANDOM', 'Non-deterministic runtime access is forbidden.');
    }
    if (ts.isCallExpression(node)) {
      const expr = node.expression;
      if (ts.isPropertyAccessExpression(expr) && ['map', 'filter', 'reduce'].includes(expr.name.text)) addDiag(node, 'TSPACK_MANIFEST_FORBIDDEN_DYNAMIC_EXPRESSION', 'Dynamic collection transforms are forbidden.');
      else if (ts.isIdentifier(expr) && !APPROVED_HELPERS.has(expr.text) && expr.text !== 'define') addDiag(node, 'TSPACK_MANIFEST_UNKNOWN_HELPER', `Unknown helper: ${expr.text}`);
    }
    if (ts.isJsxSelfClosingElement(node) || ts.isJsxOpeningElement(node)) {
      const tag = node.tagName.getText(sf);
      if (!APPROVED_ELEMENTS.has(tag)) addDiag(node, 'TSPACK_MANIFEST_UNKNOWN_ELEMENT', `Unknown JSX element: ${tag}`);
    }
    ts.forEachChild(node, walk);
  };

  for (const st of sf.statements) {
    if (ts.isVariableStatement(st) && st.declarationList.flags & ts.NodeFlags.Const) {
      for (const dec of st.declarationList.declarations) {
        if (ts.isIdentifier(dec.name) && dec.initializer) constEnv.set(dec.name.text, evalNode(dec.initializer, sf, diagnostics, normalizedPath, constEnv));
      }
    }
  }

  walk(sf);

  const exp = sf.statements.find(ts.isExportAssignment);
  let ir: ManifestIr | undefined;
  if (!exp || !ts.isCallExpression(exp.expression) || exp.expression.expression.getText(sf) !== 'define') {
    diagnostics.push({ code: 'TSPACK_MANIFEST_INVALID_DEFAULT_EXPORT', severity: 'error', message: 'Default export must be define(...)', file: normalizedPath });
  } else {
    const evaluated = evalNode(exp.expression, sf, diagnostics, normalizedPath, constEnv) as ManifestIr | undefined;
    if (evaluated && typeof evaluated === 'object') ir = stableSort(evaluated as ManifestIr);
  }

  const sortedDiagnostics = diagnostics.sort((a, b) => `${a.file}:${a.line ?? 0}:${a.column ?? 0}:${a.code}:${a.message}`.localeCompare(`${b.file}:${b.line ?? 0}:${b.column ?? 0}:${b.code}:${b.message}`));
  return { ok: sortedDiagnostics.length === 0, ir: sortedDiagnostics.length === 0 ? ir : undefined, diagnostics: sortedDiagnostics };
}

function evalNode(node: ts.Node, sf: ts.SourceFile, diags: Diagnostic[], file: string, env: Map<string, unknown>): unknown {
  if (ts.isParenthesizedExpression(node) || ts.isSatisfiesExpression(node)) return evalNode(node.expression, sf, diags, file, env);
  if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) return node.text;
  if (ts.isNumericLiteral(node)) return Number(node.text);
  if (node.kind === ts.SyntaxKind.TrueKeyword) return true;
  if (node.kind === ts.SyntaxKind.FalseKeyword) return false;
  if (node.kind === ts.SyntaxKind.NullKeyword) return null;
  if (ts.isIdentifier(node)) {
    if (!env.has(node.text)) { diags.push({ code: 'TSPACK_MANIFEST_UNRESOLVED_REFERENCE', severity: 'error', message: `Unresolved reference: ${node.text}`, file }); return undefined; }
    return env.get(node.text);
  }
  if (ts.isPropertyAccessExpression(node)) {
    const base = evalNode(node.expression, sf, diags, file, env) as Record<string, unknown>;
    return base?.[node.name.text];
  }
  if (ts.isArrayLiteralExpression(node)) return node.elements.map((e) => evalNode(e, sf, diags, file, env));
  if (ts.isObjectLiteralExpression(node)) {
    const out: Record<string, unknown> = {};
    for (const p of node.properties) {
      if (ts.isPropertyAssignment(p)) {
        const key = p.name.getText(sf).replaceAll('"', '').replaceAll("'", '');
        out[key] = evalNode(p.initializer, sf, diags, file, env);
      }
    }
    return out;
  }
  if (ts.isCallExpression(node)) {
    const name = node.expression.getText(sf);
    const args = node.arguments.map((a) => evalNode(a, sf, diags, file, env));
    if (name === 'define') return jsxToIr(args[0]);
    if (name === 'defineDeps') return args[0];
    if (name === 'npm') return { kind: 'npm', package: args[0], range: args[1] };
    if (name === 'git') return { kind: 'git', ref: args[0], ...(typeof args[1] === 'object' ? (args[1] as object) : {}) };
    if (name === 'path') return { kind: 'path', path: args[0] };
    if (name === 'dep' || name === 'peer' || name === 'tool' || name === 'workspace') {
      return { kind: name, source: args[0], ...(typeof args[1] === 'object' ? (args[1] as object) : {}) };
    }
  }
  if (ts.isJsxElement(node) || ts.isJsxSelfClosingElement(node)) return jsxEval(node, sf, diags, file, env);
  diags.push({ code: 'TSPACK_MANIFEST_FORBIDDEN_DYNAMIC_EXPRESSION', severity: 'error', message: `Unsupported expression: ${node.kind}`, file });
  return undefined;
}

function jsxEval(node: ts.JsxElement | ts.JsxSelfClosingElement, sf: ts.SourceFile, diags: Diagnostic[], file: string, env: Map<string, unknown>): unknown {
  const open = ts.isJsxElement(node) ? node.openingElement : node;
  const tag = open.tagName.getText(sf);
  const props: Record<string, unknown> = {};
  for (const attr of open.attributes.properties) {
    if (ts.isJsxAttribute(attr) && attr.name) {
      const key = attr.name.text;
      if (attr.initializer && ts.isJsxExpression(attr.initializer) && attr.initializer.expression) props[key] = evalNode(attr.initializer.expression, sf, diags, file, env);
      else if (attr.initializer && ts.isStringLiteral(attr.initializer)) props[key] = attr.initializer.text;
    }
  }
  const children = ts.isJsxElement(node)
    ? node.children.filter(ts.isJsxElement).concat(node.children.filter(ts.isJsxSelfClosingElement))
    : [];
  return { __tag: tag, ...props, __children: children.map((c) => jsxEval(c as any, sf, diags, file, env)) };
}

function jsxToIr(root: unknown): ManifestIr {
  const r = root as any;
  const workspaceName = r?.name ?? 'workspace';
  const packages = (r?.__children ?? []).filter((c: any) => c.__tag === 'Package').map((p: any) => ({
    name: p.name,
    version: p.version,
    license: p.license,
    kind: p.kind,
    dependencies: p.dependencies?.values ?? [],
    targets: p.__children?.find((x: any) => x.__tag === 'Targets')?.rows ?? [],
    tools: p.__children?.find((x: any) => x.__tag === 'Tools')?.values?.map((v: any) => v?.source?.package ?? v?.source?.ref ?? v?.source?.path ?? v?.kind) ?? [],
    boundaries: p.__children?.find((x: any) => x.__tag === 'Boundaries')?.rows ?? [],
    publish: { include: p.__children?.find((x: any) => x.__tag === 'Publish')?.include ?? [], exclude: p.__children?.find((x: any) => x.__tag === 'Publish')?.exclude ?? [] },
    policies: p.__children?.find((x: any) => x.__tag === 'Policies') ?? {},
  }));
  return { format: 1, workspace: { name: workspaceName }, packages };
}

function stableSort<T>(obj: T): T {
  if (Array.isArray(obj)) return obj.map((v) => stableSort(v)) as T;
  if (obj && typeof obj === 'object') {
    const out: Record<string, unknown> = {};
    for (const k of Object.keys(obj as Record<string, unknown>).sort()) out[k] = stableSort((obj as Record<string, unknown>)[k]);
    return out as T;
  }
  return obj;
}
