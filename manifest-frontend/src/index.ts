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
  workspace: { name: string; runtime: RuntimeProfile };
  security?: Record<string, unknown>;
  updatePolicy?: Record<string, unknown>;
  packages: Array<Record<string, unknown>>;
};

type RuntimeProfile = 'nodejs' | 'bun' | 'deno';
type ParseMode = 'root' | 'package';
type DocMode = 'single' | 'split' | 'package';
type PackageRow = { name: string; root: string; manifest: string };
type InternalDoc = { mode: DocMode; ir: ManifestIr; rows?: PackageRow[] };

const ALLOWED_IMPORT = 'tspack/manifest';
const APPROVED_HELPERS = new Set(['define', 'defineWorkspace', 'definePackage', 'defineDeps', 'npm', 'git', 'path', 'workspace', 'dep', 'peer', 'tool', 'Env']);
const APPROVED_ELEMENTS = new Set(['Workspace', 'Packages', 'Package', 'Policies', 'Targets', 'RunTargets', 'Tools', 'Boundaries', 'Publish', 'Security', 'UpdatePolicy']);

export function parseManifestFile(filePath: string): ManifestParseResult {
  const parsed = parseManifestDocument(filePath, 'root');
  if (parsed.diagnostics.length > 0 || !parsed.doc) {
    return { ok: false, diagnostics: parsed.diagnostics };
  }
  if (parsed.doc.mode === 'split') {
    return {
      ok: false,
      diagnostics: [{ code: 'TSPACK_MANIFEST_INVALID_MODE', severity: 'error', message: 'split workspace manifests must be parsed via parseWorkspace(...)', file: path.resolve(filePath) }],
    };
  }
  return { ok: true, ir: stableSort(parsed.doc.ir), diagnostics: [] };
}


export function parsePackageManifestFile(filePath: string): ManifestParseResult {
  const parsed = parseManifestDocument(filePath, 'package');
  if (parsed.diagnostics.length > 0 || !parsed.doc) {
    return { ok: false, diagnostics: parsed.diagnostics };
  }
  return { ok: true, ir: stableSort(parsed.doc.ir), diagnostics: [] };
}

export function parseWorkspace(rootManifestPath: string): ManifestParseResult {
  const rootParsed = parseManifestDocument(rootManifestPath, 'root');
  if (rootParsed.diagnostics.length > 0 || !rootParsed.doc) {
    return { ok: false, diagnostics: rootParsed.diagnostics };
  }
  if (rootParsed.doc.mode !== 'split') {
    return { ok: true, ir: stableSort(rootParsed.doc.ir), diagnostics: [] };
  }

  const wsDir = path.dirname(path.resolve(rootManifestPath));
  const diags = [...rootParsed.diagnostics];
  const rows = rootParsed.doc.rows ?? [];
  const mergedPackages: Array<Record<string, unknown>> = [];
  const seenNames = new Set<string>();
  const seenRoots = new Set<string>();

  for (const row of rows) {
    if (seenNames.has(row.name)) {
      diags.push({ code: 'TSPACK_MANIFEST_DUPLICATE_PACKAGE_ROW', severity: 'error', message: `duplicate package row name: ${row.name}`, file: path.resolve(rootManifestPath) });
    }
    seenNames.add(row.name);

    if (seenRoots.has(row.root)) {
      diags.push({ code: 'TSPACK_MANIFEST_DUPLICATE_PACKAGE_ROOT', severity: 'error', message: `duplicate package root: ${row.root}`, file: path.resolve(rootManifestPath) });
    }
    seenRoots.add(row.root);

    if (!isSafeRel(row.root)) {
      diags.push({ code: 'TSPACK_MANIFEST_INVALID_PACKAGE_ROOT', severity: 'error', message: 'package row root must be a safe relative path', file: path.resolve(rootManifestPath) });
      continue;
    }
    if (!isSafeRel(row.manifest)) {
      diags.push({ code: 'TSPACK_MANIFEST_INVALID_PACKAGE_MANIFEST_PATH', severity: 'error', message: 'package row manifest must be a safe relative path', file: path.resolve(rootManifestPath) });
      continue;
    }
    const normalizedRoot = path.normalize(row.root);
    const normalizedManifest = path.normalize(row.manifest);
    if (!normalizedManifest.startsWith(normalizedRoot + path.sep)) {
      diags.push({ code: 'TSPACK_MANIFEST_INVALID_PACKAGE_MANIFEST_PATH', severity: 'error', message: 'package manifest path must live under package root', file: path.resolve(rootManifestPath) });
      continue;
    }

    const packageManifestPath = path.join(wsDir, row.manifest);
    if (!fs.existsSync(packageManifestPath)) {
      diags.push({ code: 'TSPACK_MANIFEST_PACKAGE_MANIFEST_NOT_FOUND', severity: 'error', message: `package manifest not found: ${row.manifest}`, file: path.resolve(rootManifestPath) });
      continue;
    }

    const packageParsed = parseManifestDocument(packageManifestPath, 'package');
    diags.push(...packageParsed.diagnostics);
    if (packageParsed.diagnostics.length > 0 || !packageParsed.doc) {
      continue;
    }
    const pkg = packageParsed.doc.ir.packages[0];
    if ((pkg.name as string) !== row.name) {
      diags.push({ code: 'TSPACK_MANIFEST_PACKAGE_NAME_MISMATCH', severity: 'error', message: `package manifest name must match row.name (${row.name})`, file: path.resolve(packageManifestPath) });
      continue;
    }
    mergedPackages.push({ ...pkg, root: slashPath(row.root) });
  }

  const sorted = sortDiagnostics(diags);
  if (sorted.length > 0) {
    return { ok: false, diagnostics: sorted };
  }

  return {
    ok: true,
    ir: stableSort({ ...rootParsed.doc.ir, packages: mergedPackages }),
    diagnostics: [],
  };
}

function parseManifestDocument(filePath: string, modeHint: ParseMode): { doc?: InternalDoc; diagnostics: Diagnostic[] } {
  const diagnostics: Diagnostic[] = [];
  const normalizedPath = path.resolve(filePath);
  if (modeHint === 'root' && path.basename(normalizedPath) !== 'manifest.tsx') {
    diagnostics.push({ code: 'TSPACK_MANIFEST_NON_ROOT', severity: 'error', message: 'manifest must be root-level manifest.tsx', file: normalizedPath });
  }
  if (modeHint === 'package' && path.basename(normalizedPath) !== 'package.manifest.tsx') {
    diagnostics.push({ code: 'TSPACK_MANIFEST_INVALID_MODE', severity: 'error', message: 'package manifest file must be package.manifest.tsx', file: normalizedPath });
  }

  const sourceText = fs.readFileSync(normalizedPath, 'utf8');
  const sf = ts.createSourceFile(normalizedPath, sourceText, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const constEnv = new Map<string, unknown>();

  const addDiag = (node: ts.Node, code: string, message: string) => {
    const lc = sf.getLineAndCharacterOfPosition(node.getStart(sf));
    diagnostics.push({ code, severity: 'error', message, file: normalizedPath, line: lc.line + 1, column: lc.character + 1 });
  };

  for (const st of sf.statements) {
    if (ts.isVariableStatement(st) && st.declarationList.flags & ts.NodeFlags.Const) {
      for (const dec of st.declarationList.declarations) {
        if (ts.isIdentifier(dec.name) && dec.initializer) {
          constEnv.set(dec.name.text, evalNode(dec.initializer, sf, diagnostics, normalizedPath, constEnv));
        }
      }
    }
  }

  const walk = (node: ts.Node) => {
    if (ts.isImportDeclaration(node)) {
      const mod = (node.moduleSpecifier as ts.StringLiteral).text;
      if (mod !== ALLOWED_IMPORT) addDiag(node, 'TSPACK_MANIFEST_FORBIDDEN_IMPORT', `Only imports from "${ALLOWED_IMPORT}" are allowed.`);
    }
    if (ts.isFunctionDeclaration(node) || ts.isFunctionExpression(node) || ts.isArrowFunction(node)) addDiag(node, 'TSPACK_MANIFEST_FORBIDDEN_FUNCTION', 'Functions are forbidden in manifest subset.');
    if (ts.isClassDeclaration(node)) addDiag(node, 'TSPACK_MANIFEST_FORBIDDEN_CLASS', 'Classes are forbidden in manifest subset.');
    if (ts.isForStatement(node) || ts.isForInStatement(node) || ts.isForOfStatement(node) || ts.isWhileStatement(node) || ts.isDoStatement(node)) addDiag(node, 'TSPACK_MANIFEST_FORBIDDEN_LOOP', 'Loops are forbidden in manifest subset.');
    if (ts.isIfStatement(node) || ts.isConditionalExpression(node) || ts.isSwitchStatement(node)) addDiag(node, 'TSPACK_MANIFEST_FORBIDDEN_CONDITIONAL', 'Conditionals are forbidden in manifest subset.');
    if (ts.isPropertyAccessExpression(node) && node.getText(sf) === 'process.env') addDiag(node, 'TSPACK_MANIFEST_FORBIDDEN_PROCESS_ENV', 'process.env access is forbidden.');
    if (ts.isIdentifier(node) && node.text === 'fetch') addDiag(node, 'TSPACK_MANIFEST_FORBIDDEN_FETCH', 'fetch access is forbidden.');
    if (ts.isSpreadAssignment(node) || ts.isSpreadElement(node) || ts.isJsxSpreadAttribute(node)) addDiag(node, 'TSPACK_MANIFEST_FORBIDDEN_SPREAD', 'Spread syntax is forbidden in manifest subset.');
    if (ts.isCallExpression(node)) {
      const expr = node.expression;
      if (ts.isPropertyAccessExpression(expr) && ['map', 'filter', 'reduce'].includes(expr.name.text)) addDiag(node, 'TSPACK_MANIFEST_FORBIDDEN_DYNAMIC_EXPRESSION', 'Dynamic collection transforms are forbidden.');
      else if (ts.isIdentifier(expr) && !APPROVED_HELPERS.has(expr.text)) addDiag(node, 'TSPACK_MANIFEST_UNKNOWN_HELPER', `Unknown helper: ${expr.text}`);
    }
    if (ts.isJsxSelfClosingElement(node) || ts.isJsxOpeningElement(node)) {
      const tag = node.tagName.getText(sf);
      if (!APPROVED_ELEMENTS.has(tag)) addDiag(node, 'TSPACK_MANIFEST_UNKNOWN_ELEMENT', `Unknown JSX element: ${tag}`);
    }
    ts.forEachChild(node, walk);
  };
  walk(sf);

  const exp = sf.statements.find(ts.isExportAssignment);
  if (!exp || !ts.isCallExpression(exp.expression)) {
    diagnostics.push({ code: 'TSPACK_MANIFEST_INVALID_DEFAULT_EXPORT', severity: 'error', message: 'Default export must be define(...)', file: normalizedPath });
    return { diagnostics: sortDiagnostics(diagnostics) };
  }

  const helper = exp.expression.expression.getText(sf);
  if (modeHint === 'root' && !['define', 'defineWorkspace'].includes(helper)) {
    diagnostics.push({ code: 'TSPACK_MANIFEST_INVALID_DEFAULT_EXPORT', severity: 'error', message: 'Root manifest default export must be define(...) or defineWorkspace(...)', file: normalizedPath });
  }
  if (modeHint === 'package' && helper !== 'definePackage') {
    diagnostics.push({ code: 'TSPACK_MANIFEST_PACKAGE_MANIFEST_INVALID_DEFAULT_EXPORT', severity: 'error', message: 'Package manifest default export must be definePackage(...)', file: normalizedPath });
  }

  const evaluated = evalNode(exp.expression, sf, diagnostics, normalizedPath, constEnv) as any;
  const sorted = sortDiagnostics(diagnostics);
  if (sorted.length > 0) return { diagnostics: sorted };

  if (evaluated?.mode === 'split' && modeHint === 'root') return { diagnostics: [], doc: evaluated };
  if (evaluated?.mode === 'package' && modeHint === 'package') return { diagnostics: [], doc: evaluated };
  if (evaluated?.mode === 'single' && modeHint === 'root') return { diagnostics: [], doc: evaluated };

  return { diagnostics: [{ code: 'TSPACK_MANIFEST_INVALID_MODE', severity: 'error', message: 'Invalid manifest mode for file type', file: normalizedPath }] };
}

function evalNode(node: ts.Node, sf: ts.SourceFile, diags: Diagnostic[], file: string, env: Map<string, unknown>): unknown {
  if (ts.isParenthesizedExpression(node) || ts.isSatisfiesExpression(node)) return evalNode(node.expression, sf, diags, file, env);
  if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) return node.text;
  if (ts.isNumericLiteral(node)) return Number(node.text);
  if (node.kind === ts.SyntaxKind.TrueKeyword) return true;
  if (node.kind === ts.SyntaxKind.FalseKeyword) return false;
  if (node.kind === ts.SyntaxKind.NullKeyword) return null;
  if (ts.isIdentifier(node)) return env.get(node.text);
  if (ts.isPropertyAccessExpression(node)) {
    const base = evalNode(node.expression, sf, diags, file, env) as Record<string, unknown>;
    return base?.[node.name.text];
  }
  if (ts.isArrayLiteralExpression(node)) return node.elements.map((e) => evalNode(e, sf, diags, file, env));
  if (ts.isObjectLiteralExpression(node)) {
    const out: Record<string, unknown> = {};
    for (const p of node.properties) {
      if (ts.isPropertyAssignment(p)) out[p.name.getText(sf).replaceAll('"', '').replaceAll("'", '')] = evalNode(p.initializer, sf, diags, file, env);
    }
    return out;
  }
  if (ts.isCallExpression(node)) {
    const name = node.expression.getText(sf);
    const args = node.arguments.map((a) => evalNode(a, sf, diags, file, env));
    if (name === 'define' || name === 'defineWorkspace') return jsxToRootDoc(args[0], diags, file);
    if (name === 'definePackage') return jsxToPackageDoc(args[0]);
    if (name === 'defineDeps') return attachDependencyKeys(args[0]);
    if (name === 'npm') return { kind: 'npm', package: args[0], range: args[1] };
    if (name === 'git') return { kind: 'git', ref: args[0], ...(typeof args[1] === 'object' ? (args[1] as object) : {}) };
    if (name === 'path') return { kind: 'path', path: args[0] };
    if (name === 'dep' || name === 'peer' || name === 'tool') return { kind: name, source: args[0], ...(typeof args[1] === 'object' ? (args[1] as object) : {}) };
    if (name === 'workspace') return { kind: 'workspace', name: args[0], ...(typeof args[1] === 'object' ? (args[1] as object) : {}) };
    if (name === 'Env') return { name: args[0], ...(typeof args[1] === 'object' ? (args[1] as object) : {}) };
  }
  if (ts.isJsxElement(node) || ts.isJsxSelfClosingElement(node)) return jsxEval(node, sf, diags, file, env);
  diags.push({ code: 'TSPACK_MANIFEST_FORBIDDEN_DYNAMIC_EXPRESSION', severity: 'error', message: `Unsupported expression: ${node.kind}`, file });
  return undefined;
}

function jsxToRootDoc(root: unknown, diags: Diagnostic[], file: string): InternalDoc {
  const r = root as any;
  const children = r?.__children ?? [];
  const inlinePackages = children.filter((c: any) => c.__tag === 'Package');
  const packagesNode = children.find((c: any) => c.__tag === 'Packages');
  const securityNode = children.find((c: any) => c.__tag === 'Security');
  const updatePolicyNode = children.find((c: any) => c.__tag === 'UpdatePolicy');
  const baseIr = {
    format: 1 as const,
    workspace: { name: r?.name ?? 'workspace', runtime: runtimeProfile(r?.runtime, diags, file) },
    ...(securityNode ? { security: mapSecurity(securityNode) } : {}),
    ...(updatePolicyNode ? { updatePolicy: mapUpdatePolicy(updatePolicyNode) } : {}),
  };
  if (packagesNode && inlinePackages.length > 0) {
    return { mode: 'split', ir: { ...baseIr, packages: [] }, rows: [] };
  }
  if (packagesNode) {
    return { mode: 'split', ir: { ...baseIr, packages: [] }, rows: (packagesNode.rows as PackageRow[]) ?? [] };
  }
  return { mode: 'single', ir: { ...baseIr, packages: inlinePackages.map((p: any) => mapPackage(p, false)) } };
}


function runtimeProfile(value: unknown, diags: Diagnostic[], file: string): RuntimeProfile {
  if (value === undefined) {
    return 'nodejs';
  }

  if (value === 'nodejs' || value === 'bun' || value === 'deno') {
    return value;
  }

  diags.push({
    code: 'TSPACK_MANIFEST_INVALID_RUNTIME_PROFILE',
    severity: 'error',
    message: 'runtime profile must be nodejs, bun, or deno; package manager names such as npm/pnpm/yarn are not runtime profiles',
    file,
  });
  return 'nodejs';
}

function mapSecurity(security: any): Record<string, unknown> {
  return {
    acknowledgedCapabilities: security.acknowledgedCapabilities ?? [],
    acknowledgedLifecycleCategories: security.acknowledgedLifecycleCategories ?? [],
  };
}

function mapUpdatePolicy(updatePolicy: any): Record<string, unknown> {
  return {
    rows: updatePolicy.rows ?? [],
  };
}

function jsxToPackageDoc(root: unknown): InternalDoc {
  const p = root as any;
  return { mode: 'package', ir: { format: 1, workspace: { name: 'workspace', runtime: 'nodejs' }, packages: [mapPackage(p, false)] } };
}

function mapPackage(p: any, includeRoot: boolean): Record<string, unknown> {
  return {
    name: p.name,
    version: p.version,
    ...(includeRoot ? { root: '.' } : {}),
    license: p.license,
    kind: p.kind,
    dependencies: mapDependencies(p.dependencies?.values ?? []),
    targets: mapTargets(p.__children?.find((x: any) => x.__tag === 'Targets')?.rows ?? []),
    ...(p.__children?.find((x: any) => x.__tag === 'RunTargets') ? { runTargets: p.__children?.find((x: any) => x.__tag === 'RunTargets')?.rows ?? [] } : {}),
    tools: mapDependencyRefs(p.__children?.find((x: any) => x.__tag === 'Tools')?.values ?? []),
    boundaries: p.__children?.find((x: any) => x.__tag === 'Boundaries')?.rows ?? [],
    publish: { include: p.__children?.find((x: any) => x.__tag === 'Publish')?.include ?? [], exclude: p.__children?.find((x: any) => x.__tag === 'Publish')?.exclude ?? [] },
    policies: p.__children?.find((x: any) => x.__tag === 'Policies') ?? {},
  };
}

function mapDependencies(values: any[]): any[] {
  return values.map((value) => {
    if (!value || typeof value !== 'object' || Array.isArray(value)) {
      return value;
    }

    const { __key: _key, ...dependency } = value;
    return dependency;
  });
}

function mapTargets(rows: any[]): any[] {
  return rows.map((row) => {
    const deps = Array.isArray(row?.deps) ? row.deps : [];
    const peers = Array.isArray(row?.peers) ? row.peers : [];

    return {
      ...row,
      deps: mapDependencyRefs(deps),
      peers: mapDependencyRefs(peers),
    };
  });
}

function mapDependencyRefs(values: any[]): string[] {
  return values.map((value) => dependencyRefKey(value));
}

function dependencyRefKey(value: any): string {
  if (typeof value === 'string') {
    return value;
  }
  if (typeof value?.key === 'string') {
    return value.key;
  }
  if (typeof value?.__key === 'string') {
    return value.__key;
  }
  if (typeof value?.source?.package === 'string') {
    return value.source.package;
  }
  if (typeof value?.source?.ref === 'string') {
    return value.source.ref;
  }
  if (typeof value?.source?.path === 'string') {
    return value.source.path;
  }
  return value?.kind;
}

function attachDependencyKeys(value: unknown): unknown {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return value;
  }

  const out: Record<string, unknown> = {};
  for (const [key, dependency] of Object.entries(value as Record<string, unknown>)) {
    if (dependency && typeof dependency === 'object' && !Array.isArray(dependency)) {
      out[key] = { ...(dependency as Record<string, unknown>), __key: key };
    } else {
      out[key] = dependency;
    }
  }
  return out;
}

function jsxEval(node: ts.JsxElement | ts.JsxSelfClosingElement, sf: ts.SourceFile, diags: Diagnostic[], file: string, env: Map<string, unknown>): unknown {
  const open = ts.isJsxElement(node) ? node.openingElement : node;
  const tag = open.tagName.getText(sf);
  const props: Record<string, unknown> = {};
  for (const attr of open.attributes.properties) {
    if (ts.isJsxAttribute(attr) && attr.name) {
      const key = attr.name.getText(sf);
      if (attr.initializer && ts.isJsxExpression(attr.initializer) && attr.initializer.expression) props[key] = evalNode(attr.initializer.expression, sf, diags, file, env);
      else if (attr.initializer && ts.isStringLiteral(attr.initializer)) props[key] = attr.initializer.text;
    }
  }
  const children = ts.isJsxElement(node)
    ? node.children.filter((child): child is ts.JsxElement | ts.JsxSelfClosingElement => ts.isJsxElement(child) || ts.isJsxSelfClosingElement(child))
    : [];

  return { __tag: tag, ...props, __children: children.map((child) => jsxEval(child, sf, diags, file, env)) };
}

function isSafeRel(p: string): boolean {
  return !!p && !path.isAbsolute(p) && !p.includes('..') && !p.includes('\\');
}

function slashPath(p: string): string { return p.replaceAll('\\', '/'); }

function sortDiagnostics(diags: Diagnostic[]): Diagnostic[] {
  return diags.sort((a, b) => `${a.file}:${a.line ?? 0}:${a.column ?? 0}:${a.code}:${a.message}`.localeCompare(`${b.file}:${b.line ?? 0}:${b.column ?? 0}:${b.code}:${b.message}`));
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
