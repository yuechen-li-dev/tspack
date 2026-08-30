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
  registryPolicy?: Record<string, unknown>;
  compatFiles?: Array<Record<string, unknown>>;
  packages: Array<Record<string, unknown>>;
  packageAnnotations?: Array<Record<string, unknown>>;
  workflows?: Array<Record<string, unknown>>;
};

type RuntimeProfile = 'nodejs' | 'bun' | 'deno';
type Compiler = 'tsc' | 'tscl' | 'scriptc' | 'perry' | 'rollup';
type ParseMode = 'root' | 'package';
type DocMode = 'single' | 'split' | 'package' | 'annotation';
type PackageRow = { name: string; root: string; manifest: string };
type InternalDoc = { mode: DocMode; ir: ManifestIr; rows?: PackageRow[] };
type AuthoringContext = {
  originKind: 'project-manifest' | 'package-manifest';
  sourcePath: string;
  layer: 'project' | 'package';
  declarationDefaults?: Record<string, unknown>;
};

export type DependencyIslandStatus =
  | 'OwnedCanonical'
  | 'OwnedRecognized'
  | 'UserDynamic'
  | 'Ambiguous'
  | 'Unsupported'
  | 'Absent';

export type SourceRange = {
  start: number;
  end: number;
};

export type DependencyIslandElement = SourceRange & {
  fullStart: number;
};

export type DependencySourceAnalysis = {
  status: DependencyIslandStatus;
  authority?: 'native' | 'annotation';
  manifestPath: string;
  packageName?: string;
  island?: SourceRange & {
    contentStart: number;
    contentEnd: number;
    elements: DependencyIslandElement[];
  };
  insertion?: {
    offset: number;
    multiline: boolean;
    attributeIndent: string;
    closingIndent: string;
  };
  manifestImport?: {
    contentStart: number;
    contentEnd: number;
    names: string[];
  };
  diagnostics: Diagnostic[];
};

const ALLOWED_IMPORT = 'tspack/manifest';
const APPROVED_HELPERS = new Set(['define', 'defineWorkspace', 'definePackage', 'annotatePackage', 'defineDeps', 'npm', 'jsr', 'git', 'path', 'workspace', 'dep', 'peer', 'tool', 'localFixture', 'builtArtifactFixture', 'Env', 'Service', 'json', 'Workflow', 'Job', 'Sequence', 'Parallel', 'Branch', 'On', 'MatchResult', 'Finally', 'ForEach', 'ParallelForEach', 'CollectAll', 'FailFast', 'Transfer', 'When', 'GreaterThan', 'LessThan', 'NotEmpty', 'IsEmpty', 'And', 'Or', 'Not', 'Manual', 'Push', 'PullRequest', 'Linux', 'Windows', 'MacOS', 'CurrentHost', 'Sync', 'Check', 'Build', 'Test', 'Pack', 'Audit', 'Process', 'ShellScript', 'Plain', 'Secret', 'WorkflowEnv']);
const APPROVED_ELEMENTS = new Set(['Workspace', 'Packages', 'Package', 'PackageAnnotations', 'Policies', 'Targets', 'RunTargets', 'TestTargets', 'Workflows', 'SkyrimTarget', 'Tools', 'Boundaries', 'Publish', 'Security', 'UpdatePolicy', 'RegistryPolicy', 'RegistrySource', 'CompatFiles', 'JsonFile']);
const APPROVED_PROPERTY_HELPERS = new Set(['TsConfig.manifestEditor', 'VSCode.settings', 'VSCode.extensions']);
const DEFAULT_MANIFEST_EDITOR_INCLUDE = [
  'manifest.tsx',
  'package.manifest.tsx',
  '**/*.manifest.tsx',
  '**/*.xtest.tsx',
  '.tspack/types/**/*.d.ts',
] as const;
const DEFAULT_MANIFEST_EDITOR_EXCLUDE = [
  'dist/**',
  'node_modules/**',
  '.tspack/store/**',
  'tspack-artifacts/**',
] as const;

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
    const manifestLivesUnderRoot = normalizedRoot === '.'
      || normalizedManifest.startsWith(normalizedRoot + path.sep);
    if (!manifestLivesUnderRoot) {
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
    if (packageParsed.doc.mode === 'annotation') {
      diags.push({ code: 'TSPACK_MANIFEST_PACKAGE_ANNOTATION_NOT_FULL_PACKAGE', severity: 'error', message: 'annotation package manifests are for incremental adoption and do not define full package contracts', file: path.resolve(packageManifestPath) });
      continue;
    }
    const pkg = packageParsed.doc.ir.packages[0];
    if ((pkg.name as string) !== row.name) {
      diags.push({ code: 'TSPACK_MANIFEST_PACKAGE_NAME_MISMATCH', severity: 'error', message: `package manifest name must match row.name (${row.name})`, file: path.resolve(packageManifestPath) });
      continue;
    }
    mergedPackages.push({
      ...withDependencyAuthoringSourcePath(pkg, slashPath(row.manifest)),
      root: slashPath(row.root),
      manifestPath: slashPath(row.manifest),
    });
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
    if ((ts.isFunctionDeclaration(node) || ts.isFunctionExpression(node) || ts.isArrowFunction(node)) && !isWorkflowCallback(node)) addDiag(node, 'TSPACK_MANIFEST_FORBIDDEN_FUNCTION', 'Functions are forbidden in manifest subset.');
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
      else if (ts.isPropertyAccessExpression(expr) && !APPROVED_PROPERTY_HELPERS.has(expr.getText(sf))) addDiag(node, 'TSPACK_MANIFEST_UNKNOWN_HELPER', `Unknown helper: ${expr.getText(sf)}`);
    }
    if (ts.isJsxSelfClosingElement(node) || ts.isJsxOpeningElement(node)) {
      const tag = node.tagName.getText(sf);
      if (!APPROVED_ELEMENTS.has(tag)) addDiag(node, 'TSPACK_MANIFEST_UNKNOWN_ELEMENT', `Unknown JSX element: ${tag}`);
      if (tag === 'SkyrimTarget') {
        validateSkyrimTargetSyntax(node, sf, addDiag);
      }
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
  if (modeHint === 'package' && !['definePackage', 'annotatePackage'].includes(helper)) {
    diagnostics.push({ code: 'TSPACK_MANIFEST_PACKAGE_MANIFEST_INVALID_DEFAULT_EXPORT', severity: 'error', message: 'Package manifest default export must be definePackage(...) or annotatePackage(...)', file: normalizedPath });
  }

  const evaluated = evalNode(exp.expression, sf, diagnostics, normalizedPath, constEnv) as any;
  const sorted = sortDiagnostics(diagnostics);
  if (sorted.length > 0) return { diagnostics: sorted };

  if (evaluated?.mode === 'split' && modeHint === 'root') return { diagnostics: [], doc: evaluated };
  if (evaluated?.mode === 'package' && modeHint === 'package') return { diagnostics: [], doc: evaluated };
  if (evaluated?.mode === 'annotation' && modeHint === 'package') return { diagnostics: [], doc: evaluated };
  if (evaluated?.mode === 'single' && modeHint === 'root') return { diagnostics: [], doc: evaluated };

  return { diagnostics: [{ code: 'TSPACK_MANIFEST_INVALID_MODE', severity: 'error', message: 'Invalid manifest mode for file type', file: normalizedPath }] };
}

function evalNode(node: ts.Node, sf: ts.SourceFile, diags: Diagnostic[], file: string, env: Map<string, unknown>): unknown {
  if (ts.isParenthesizedExpression(node) || ts.isSatisfiesExpression(node)) return evalNode(node.expression, sf, diags, file, env);
  if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) return node.text;
  if (ts.isNumericLiteral(node)) return Number(node.text);
  if (ts.isPrefixUnaryExpression(node) && ts.isNumericLiteral(node.operand)) {
    const value = Number(node.operand.text);
    if (node.operator === ts.SyntaxKind.MinusToken) return -value;
    if (node.operator === ts.SyntaxKind.PlusToken) return value;
  }
  if (node.kind === ts.SyntaxKind.TrueKeyword) return true;
  if (node.kind === ts.SyntaxKind.FalseKeyword) return false;
  if (node.kind === ts.SyntaxKind.NullKeyword) return null;
  if (ts.isIdentifier(node)) return env.get(node.text);
  if (ts.isPropertyAccessExpression(node)) {
    const base = evalNode(node.expression, sf, diags, file, env) as Record<string, unknown>;
    if (base?.__workflowRef && !Object.prototype.hasOwnProperty.call(base, node.name.text)) {
      const reference = base.__workflowRef as Record<string, unknown>;
      diags.push({
        code: 'TSPACK_WORKFLOW_PROJECTION_INVALID',
        severity: 'error',
        message: `${String(reference.resultType)} results do not contain field ${node.name.text}.`,
        file,
      });
    }
    return base?.[node.name.text];
  }
  if (ts.isElementAccessExpression(node)) {
    const base = evalNode(node.expression, sf, diags, file, env) as Record<string, unknown> | undefined;
    const aggregate = base?.__workflowAggregate as Record<string, unknown> | undefined;
    const index = node.argumentExpression
      ? evalNode(node.argumentExpression, sf, diags, file, env)
      : undefined;
    if (!aggregate) {
      diags.push({
        code: 'TSPACK_WORKFLOW_AGGREGATE_INDEX_TARGET_INVALID',
        severity: 'error',
        message: 'Only workflow aggregates support bounded indexing.',
        file,
      });
      return undefined;
    }
    if (aggregate.complete !== true) {
      diags.push({
        code: 'TSPACK_WORKFLOW_AGGREGATE_INCOMPLETE',
        severity: 'error',
        message: 'Fail-fast fan-out is a partial aggregate and cannot be indexed; use CollectAll for complete aggregate consumption.',
        file,
      });
      return undefined;
    }
    if (typeof index !== 'number' || !Number.isInteger(index)) {
      diags.push({
        code: 'TSPACK_WORKFLOW_AGGREGATE_INDEX_INVALID',
        severity: 'error',
        message: 'Workflow aggregate indexes must be statically known integers.',
        file,
      });
      return undefined;
    }
    const elements = aggregate.elements as unknown[];
    if (index < 0 || index >= elements.length) {
      diags.push({
        code: 'TSPACK_WORKFLOW_AGGREGATE_INDEX_OUT_OF_RANGE',
        severity: 'error',
        message: `Workflow aggregate index ${index} is outside the bounded range 0..${elements.length - 1}.`,
        file,
      });
      return undefined;
    }
    return elements[index];
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
    if (name === 'MatchResult') return evalMatchResult(node, sf, diags, file, env);
    if (name === 'ForEach') return evalForEach(node, sf, diags, file, env);
    const args = node.arguments.map((a) => evalNode(a, sf, diags, file, env));
    if (name === 'define' || name === 'defineWorkspace') return jsxToRootDoc(args[0], diags, file);
    if (name === 'definePackage') return jsxToPackageDoc(args[0]);
    if (name === 'annotatePackage') return jsxToPackageAnnotationDoc(args[0], diags, file);
    if (name === 'defineDeps') return attachDependencyKeys(args[0]);
    if (name === 'npm') return { kind: 'npm', package: args[0], range: args[1] };
    if (name === 'jsr') return { kind: 'jsr', package: args[0], range: args[1] };
    if (name === 'git') return { kind: 'git', ref: args[0], ...(typeof args[1] === 'object' ? (args[1] as object) : {}) };
    if (name === 'path') return { kind: 'path', path: args[0] };
    if (name === 'dep' || name === 'peer' || name === 'tool') return { kind: name, source: args[0], ...(typeof args[1] === 'object' ? (args[1] as object) : {}) };
    if (name === 'localFixture') return { dependency: args[0], ...(typeof args[1] === 'object' ? (args[1] as object) : {}) };
    if (name === 'builtArtifactFixture') return { target: args[0], ...(typeof args[1] === 'object' ? (args[1] as object) : {}) };
    if (name === 'workspace') return { kind: 'workspace', name: args[0], ...(typeof args[1] === 'object' ? (args[1] as object) : {}) };
    if (name === 'Env') return { name: args[0], ...(typeof args[1] === 'object' ? (args[1] as object) : {}) };
    if (name === 'Service') return { kind: 'service', name: args[0], ...(typeof args[1] === 'object' ? (args[1] as object) : {}) };
    if (name === 'Workflow') return { identity: args[0], ...normalizeWorkflowOptions(args[1]) };
    if (name === 'Job') return { identity: args[0], ...(typeof args[1] === 'object' ? (args[1] as object) : {}) };
    if (name === 'Sequence') return { kind: 'sequence', children: args.map(asWorkflowFlowNode) };
    if (name === 'Parallel') return { kind: 'parallel', children: args.map(asWorkflowFlowNode) };
    if (name === 'Branch') return { kind: 'branch', identity: args[0], children: args.slice(1).map(asWorkflowFlowNode) };
    if (name === 'On') {
      const second = args[1];
      const secondRecord = second as Record<string, unknown> | undefined;
      const secondIsNode = !!secondRecord
        && typeof secondRecord === 'object'
        && ('operation' in secondRecord || 'kind' in secondRecord);
      const options = !secondIsNode && typeof second === 'object' && second !== null
        ? second as Record<string, unknown>
        : {};
      const children = secondIsNode ? args.slice(1) : args.slice(2);
      return {
        kind: 'region',
        runsOn: args[0],
        ...options,
        children: children.map(asWorkflowFlowNode),
      };
    }
    if (name === 'Finally') {
      return {
        kind: 'finally',
        body: asWorkflowFlowNode(args[0]),
        cleanup: asWorkflowFlowNode(args[1]),
      };
    }
    if (name === 'ParallelForEach') {
      const options = typeof args[0] === 'object' && args[0] !== null
        ? args[0] as Record<string, unknown>
        : {};
      return { kind: 'parallel', concurrency: options.concurrency };
    }
    if (name === 'CollectAll') return { kind: 'collectAll' };
    if (name === 'FailFast') return { kind: 'failFast' };
    if (name === 'GreaterThan' || name === 'LessThan') {
      return {
        kind: name === 'GreaterThan' ? 'greaterThan' : 'lessThan',
        input: args[0],
        number: args[1],
      };
    }
    if (name === 'NotEmpty' || name === 'IsEmpty') {
      return {
        kind: name === 'NotEmpty' ? 'notEmpty' : 'isEmpty',
        input: args[0],
      };
    }
    if (name === 'And' || name === 'Or') {
      return { kind: name.toLowerCase(), children: args };
    }
    if (name === 'Not') return { kind: 'not', children: [args[0]] };
    if (name === 'When') {
      return {
        kind: 'when',
        predicate: args[0],
        then: asWorkflowFlowNode(args[1]),
        ...(args[2] === undefined ? {} : { else: asWorkflowFlowNode(args[2]) }),
      };
    }
    if (name === 'Manual' || name === 'Push' || name === 'PullRequest') {
      const kind = name === 'Manual' ? 'manual' : name === 'Push' ? 'push' : 'pullRequest';
      return { kind, ...(typeof args[0] === 'object' ? (args[0] as object) : {}) };
    }
    if (name === 'Linux' || name === 'Windows' || name === 'MacOS' || name === 'CurrentHost') {
      return name === 'MacOS' ? 'macos' : name === 'CurrentHost' ? 'currentHost' : name.toLowerCase();
    }
    if (name === 'Sync' || name === 'Check' || name === 'Build' || name === 'Test' || name === 'Pack' || name === 'Audit') {
      const operation = name.toLowerCase();
      const resultIdentity = `effect/${node.getStart(sf)}`;
      if (name === 'Pack' && isWorkflowValueRef(args[0])) {
        return workflowEffect(operation, resultIdentity, {}, [args[0]]);
      }
      const options = typeof args[0] === 'object' && args[0] !== null
        ? args[0] as Record<string, unknown>
        : {};
      return workflowEffect(operation, resultIdentity, options);
    }
    if (name === 'Transfer') {
      return workflowEffect('transfer', `effect/${node.getStart(sf)}`, { transferTarget: args[1] }, [args[0]]);
    }
    if (name === 'Process') return { operation: 'process', name: args[0], ...(typeof args[1] === 'object' ? (args[1] as object) : {}) };
    if (name === 'ShellScript') return { operation: 'shellScript', name: args[0], ...(typeof args[1] === 'object' ? (args[1] as object) : {}) };
    if (name === 'Plain') return { kind: 'plain', value: args[0] };
    if (name === 'Secret') return { kind: 'secret', name: args[0] };
    if (name === 'WorkflowEnv') return { name: args[0], value: args[1] };
    if (name === 'json') return args[0];
    if (name === 'TsConfig.manifestEditor') return manifestEditorTsConfig(args[0], diags, file);
    if (name === 'VSCode.settings') return buildVSCodeSettings(args[0], diags, file);
    if (name === 'VSCode.extensions') return args.length > 0 ? args[0] : defaultVSCodeExtensions();
  }
  if (ts.isJsxElement(node) || ts.isJsxSelfClosingElement(node)) return jsxEval(node, sf, diags, file, env);
  diags.push({ code: 'TSPACK_MANIFEST_FORBIDDEN_DYNAMIC_EXPRESSION', severity: 'error', message: `Unsupported expression: ${node.kind}`, file });
  return undefined;
}


function manifestEditorTsConfig(options: unknown, diags: Diagnostic[], file: string): Record<string, unknown> | undefined {
  const validated = validateManifestEditorTsConfigOptions(options, diags, file);
  if (!validated) {
    return undefined;
  }

  return {
    compilerOptions: {
      target: 'ES2022',
      module: 'ESNext',
      moduleResolution: 'Bundler',
      jsx: 'preserve',
      strict: true,
      noEmit: true,
      types: [],
      baseUrl: '.',
      ignoreDeprecations: '5.0',
      paths: {
        'tspack/manifest': ['.tspack/types/tspack-manifest.d.ts'],
      },
    },
    include: validated.include,
    exclude: validated.exclude,
  };
}

function asWorkflowFlowNode(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return { kind: 'invalid' };
  }
  const record = value as Record<string, unknown>;
  if (typeof record.operation === 'string') {
    return { kind: 'effect', effect: workflowStep(record) };
  }
  if (record.kind === 'forEach' && record.__workflowAggregate) {
    const { __workflowAggregate: _aggregate, length: _length, ...flow } = record;
    return flow;
  }
  return record;
}

function workflowEffect(
  operation: string,
  resultIdentity: string,
  options: Record<string, unknown>,
  inputs: unknown[] = [],
): Record<string, unknown> {
  const effect: Record<string, unknown> = {
    operation,
    resultIdentity,
    ...options,
  };
  const references = inputs.filter(isWorkflowValueRef);
  if (references.length > 0) effect.inputs = references;

  const result = workflowResultRef(operation, resultIdentity);
  return {
    ...effect,
    ...result,
    __workflowRef: rootWorkflowValueRef(operation, resultIdentity),
    __workflowStep: effect,
  };
}

function workflowStep(record: Record<string, unknown>): Record<string, unknown> {
  if (record.__workflowStep && typeof record.__workflowStep === 'object') {
    return record.__workflowStep as Record<string, unknown>;
  }
  const step: Record<string, unknown> = {};
  const fields = [
    'resultIdentity',
    'inputs',
    'name',
    'operation',
    'packages',
    'targets',
    'filter',
    'auditLevel',
    'requireCoverage',
    'command',
    'script',
    'shell',
    'cwd',
    'capabilities',
    'env',
    'timeoutSeconds',
    'transferTarget',
  ];
  for (const field of fields) {
    if (record[field] !== undefined) {
      step[field] = record[field];
    }
  }
  return step;
}

function normalizeWorkflowOptions(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return {};
  }
  const options = value as Record<string, unknown>;
  if (!Array.isArray(options.jobs)) {
    return options;
  }
  return {
    ...options,
    jobs: options.jobs.map((job) => {
      if (!job || typeof job !== 'object' || Array.isArray(job)) {
        return job;
      }
      const record = job as Record<string, unknown>;
      return {
        ...record,
        steps: Array.isArray(record.steps)
          ? record.steps.map((step) => {
            if (step && typeof step === 'object') {
              return workflowStep(step as Record<string, unknown>);
            }
            return step;
          })
          : record.steps,
      };
    }),
  };
}

function workflowResultRef(operation: string, source: string): Record<string, unknown> {
  const fields: Record<string, Array<[string, string]>> = {
    build: [
      ['artifacts', 'artifactReference'],
      ['targets', 'smallSerialized'],
      ['diagnostics', 'smallSerialized'],
    ],
    test: [
      ['passed', 'control'],
      ['failed', 'control'],
      ['skipped', 'control'],
      ['durationMs', 'control'],
      ['tests', 'smallSerialized'],
      ['diagnostics', 'smallSerialized'],
    ],
    audit: [
      ['source', 'control'],
      ['auditLevel', 'control'],
      ['failing', 'control'],
      ['report', 'smallSerialized'],
      ['diagnostics', 'smallSerialized'],
    ],
    transfer: [
      ['artifacts', 'artifactReference'],
      ['targets', 'smallSerialized'],
      ['diagnostics', 'smallSerialized'],
    ],
  };
  const result: Record<string, unknown> = {};
  for (const [field, category] of fields[operation] ?? []) {
    result[field] = {
      identity: `${source}.${field}`,
      source,
      resultType: operation,
      fieldPath: [field],
      category,
    };
  }
  return result;
}

function rootWorkflowValueRef(operation: string, source: string): Record<string, unknown> {
  return {
    identity: source,
    source,
    resultType: operation,
    category: 'regionLocal',
  };
}

function isWorkflowValueRef(value: unknown): value is Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return false;
  }
  const record = value as Record<string, unknown>;
  return typeof record.identity === 'string'
    && typeof record.source === 'string'
    && typeof record.resultType === 'string';
}

function evalMatchResult(
  node: ts.CallExpression,
  sf: ts.SourceFile,
  diags: Diagnostic[],
  file: string,
  env: Map<string, unknown>,
): Record<string, unknown> {
  const source = evalNode(node.arguments[0], sf, diags, file, env) as Record<string, unknown>;
  const sourceRef = source?.__workflowRef;
  const armsNode = node.arguments[1];
  const arms: Array<Record<string, unknown>> = [];
  const expected = ['succeeded', 'failed', 'cancelled', 'timedOut'];
  if (!isWorkflowValueRef(sourceRef) || !armsNode || !ts.isObjectLiteralExpression(armsNode)) {
    diags.push({
      code: 'TSPACK_WORKFLOW_MATCH_INVALID',
      severity: 'error',
      message: 'MatchResult requires an effect result and an exhaustive arm object.',
      file,
    });
    return { kind: 'invalid' };
  }
  const properties = new Map<string, ts.Expression>();
  for (const property of armsNode.properties) {
    if (!ts.isPropertyAssignment(property)) {
      continue;
    }
    const key = property.name.getText(sf).replaceAll('"', '').replaceAll("'", '');
    if (properties.has(key)) {
      diags.push({
        code: 'TSPACK_WORKFLOW_MATCH_KIND_DUPLICATE',
        severity: 'error',
        message: `MatchResult repeats the ${key} arm.`,
        file,
      });
    }
    properties.set(key, property.initializer);
  }
  for (const kind of expected) {
    const expression = properties.get(kind);
    if (!expression) {
      diags.push({
        code: 'TSPACK_WORKFLOW_MATCH_NON_EXHAUSTIVE',
        severity: 'error',
        message: `MatchResult is missing the ${kind} arm.`,
        file,
      });
      continue;
    }
    const binding = kind === 'succeeded' ? source : failureBinding(sourceRef, kind);
    const flow = evalWorkflowArm(expression, binding, sf, diags, file, env);
    arms.push({ kind, flow: asWorkflowFlowNode(flow) });
  }
  for (const key of properties.keys()) {
    if (!expected.includes(key)) {
      diags.push({
        code: 'TSPACK_WORKFLOW_MATCH_KIND_INVALID',
        severity: 'error',
        message: `MatchResult has unknown outcome arm ${key}.`,
        file,
      });
    }
  }
  return { kind: 'match', source: sourceRef, effect: workflowStep(source), arms };
}

function failureBinding(source: Record<string, unknown>, kind: string): Record<string, unknown> {
  return {
    kind,
    error: {
      identity: `${source.source}.${kind}.error`,
      source: source.source,
      resultType: `${source.resultType}Failure`,
      fieldPath: ['error'],
      category: 'smallSerialized',
    },
    diagnostics: {
      identity: `${source.source}.${kind}.diagnostics`,
      source: source.source,
      resultType: `${source.resultType}Failure`,
      fieldPath: ['diagnostics'],
      category: 'smallSerialized',
    },
  };
}

function evalWorkflowArm(
  expression: ts.Expression,
  binding: unknown,
  sf: ts.SourceFile,
  diags: Diagnostic[],
  file: string,
  env: Map<string, unknown>,
): unknown {
  if (!ts.isArrowFunction(expression)) {
    return evalNode(expression, sf, diags, file, env);
  }
  if (!ts.isExpression(expression.body)) {
    diags.push({
      code: 'TSPACK_WORKFLOW_CALLBACK_INVALID',
      severity: 'error',
      message: 'Workflow callbacks must have a single expression body.',
      file,
    });
    return undefined;
  }
  const callbackEnv = new Map(env);
  const parameter = expression.parameters[0];
  if (parameter && ts.isIdentifier(parameter.name)) {
    callbackEnv.set(parameter.name.text, binding);
  }
  return evalNode(expression.body, sf, diags, file, callbackEnv);
}

function evalForEach(
  node: ts.CallExpression,
  sf: ts.SourceFile,
  diags: Diagnostic[],
  file: string,
  env: Map<string, unknown>,
): Record<string, unknown> {
  const identity = evalNode(node.arguments[0], sf, diags, file, env);
  const source = evalNode(node.arguments[1], sf, diags, file, env);
  const sourceAggregate = source && typeof source === 'object' && !Array.isArray(source)
    ? (source as Record<string, unknown>).__workflowAggregate as Record<string, unknown> | undefined
    : undefined;
  const values = Array.isArray(source)
    ? source
    : sourceAggregate?.elements;
	if (sourceAggregate && sourceAggregate.complete !== true) {
		diags.push({
			code: 'TSPACK_WORKFLOW_AGGREGATE_INCOMPLETE',
			severity: 'error',
			message: 'Fail-fast fan-out is a partial aggregate and cannot be iterated; use CollectAll for complete aggregate consumption.',
			file,
		});
	}
  const callback = node.arguments[2];
  const options = node.arguments[3]
    ? evalNode(node.arguments[3], sf, diags, file, env) as Record<string, unknown>
    : {};
  if (typeof identity !== 'string' || !Array.isArray(values) || !callback || !ts.isArrowFunction(callback)) {
    diags.push({
      code: 'TSPACK_WORKFLOW_FOREACH_SOURCE_INVALID',
      severity: 'error',
      message: 'ForEach requires a stable identity, a finite literal or complete workflow aggregate, and an expression callback.',
      file,
    });
    return { kind: 'invalid' };
  }
  if (values.length === 0 || values.length > 256) {
    diags.push({
      code: 'TSPACK_WORKFLOW_FOREACH_LIMIT_INVALID',
      severity: 'error',
      message: 'ForEach source must contain between 1 and 256 items.',
      file,
    });
  }
  const elementSources: string[] = [];
  const elementBindings: Array<Record<string, unknown>> = [];
  let resultType = '';
  const items = values.slice(0, 256).map((value, index) => {
    const evaluated = evalWorkflowArm(callback, value, sf, diags, file, env);
    const producedSources = collectProducedWorkflowSources(evaluated);
    const namespaced = namespaceWorkflowValues(
      evaluated,
      `foreach/${identity}/${index}`,
      producedSources,
    ) as Record<string, unknown>;
    const reference = namespaced?.__workflowRef as Record<string, unknown> | undefined;
    if (isWorkflowValueRef(reference)) {
      elementSources.push(reference.source as string);
      resultType ||= reference.resultType as string;
      elementBindings.push(aggregateElementBinding({
        source: reference.source as string,
        resultType: reference.resultType as string,
      }, '', index));
    } else {
      const typedEffect = singleWorkflowTypedEffect(asWorkflowFlowNode(namespaced));
      if (typedEffect) {
        elementSources.push(typedEffect.source);
        resultType ||= typedEffect.resultType;
        elementBindings.push(aggregateElementBinding(typedEffect, '', index));
      }
    }
    return {
      index,
      value: workflowIterationValue(value),
      flow: asWorkflowFlowNode(namespaced),
    };
  });
  const mode = options.mode as Record<string, unknown> | undefined;
  const failure = options.failure as Record<string, unknown> | undefined;
  const aggregateIdentity = `aggregate/${identity}/${node.getStart(sf)}`;
  const complete = failure?.kind === 'collectAll';
  for (const binding of elementBindings) {
    const reference = binding.__workflowRef as Record<string, unknown>;
    reference.aggregate = aggregateIdentity;
  }
  const result = {
    kind: 'forEach',
    identity,
    items,
    mode: mode?.kind === 'parallel' ? 'parallel' : 'sequential',
    ...(mode?.kind === 'parallel' ? { concurrency: mode.concurrency } : {}),
    failurePolicy: failure?.kind === 'collectAll' ? 'collectAll' : 'failFast',
    ...(sourceAggregate ? { sourceAggregate: sourceAggregate.identity } : {}),
    ...(elementSources.length === items.length ? {
      aggregate: {
        identity: aggregateIdentity,
        resultType,
        elements: elementSources,
      },
    } : {}),
  };
  return {
    ...result,
    length: items.length,
    __workflowAggregate: {
      identity: aggregateIdentity,
      resultType: complete ? `iterationOutcome<${resultType}>` : resultType,
      elements: elementBindings,
      complete,
    },
  };
}

function aggregateElementBinding(
  source: { source: string; resultType: string },
  aggregate: string,
  index: number,
): Record<string, unknown> {
  const reference = {
    ...rootWorkflowValueRef(source.resultType, source.source),
    aggregate,
    index,
  };
  return {
    operation: source.resultType,
    resultIdentity: source.source,
    ...workflowResultRef(source.resultType, source.source),
    __workflowRef: reference,
    __workflowStep: {
      operation: source.resultType,
      resultIdentity: source.source,
    },
  };
}

function singleWorkflowTypedEffect(node: Record<string, unknown>): { source: string; resultType: string } | undefined {
  if (node.kind === 'effect') {
    const effect = node.effect as Record<string, unknown> | undefined;
    const operation = effect?.operation;
    const resultIdentity = effect?.resultIdentity;
    if (typeof resultIdentity === 'string' && (operation === 'build' || operation === 'test' || operation === 'audit' || operation === 'transfer')) {
      return { source: resultIdentity, resultType: operation };
    }
  }
  if (node.kind === 'region' && Array.isArray(node.children) && node.children.length === 1) {
    return singleWorkflowTypedEffect(node.children[0] as Record<string, unknown>);
  }
  return undefined;
}

function workflowIterationValue(value: unknown): Record<string, unknown> {
	if (value && typeof value === 'object' && !Array.isArray(value)) {
		const reference = (value as Record<string, unknown>).__workflowRef;
		if (isWorkflowValueRef(reference)) {
			return { kind: 'aggregateElement', source: reference };
		}
	}
  if (typeof value === 'string') {
    const platforms = ['linux', 'windows', 'macos', 'currentHost'];
    return {
      kind: platforms.includes(value) ? 'platform' : 'string',
      string: value,
    };
  }
  if (typeof value === 'number') {
    return { kind: 'number', number: value };
  }
  return { kind: 'boolean', boolean: value };
}

function namespaceWorkflowValues(
  value: unknown,
  suffix: string,
  producedSources: ReadonlySet<string> = collectProducedWorkflowSources(value),
): unknown {
  if (Array.isArray(value)) {
    return value.map((item) => namespaceWorkflowValues(item, suffix, producedSources));
  }
  if (!value || typeof value !== 'object') {
    return value;
  }
  const record = value as Record<string, unknown>;
  const recordKind = record.kind;
  if (isWorkflowValueRef(record)) {
    if (!producedSources.has(record.source as string)) {
      return record;
    }
    return {
      ...record,
      identity: `${record.identity}/${suffix}`,
      source: `${record.source}/${suffix}`,
    };
  }
  const namespaced: Record<string, unknown> = {};
  for (const [key, item] of Object.entries(record)) {
    namespaced[key] = key === 'resultIdentity'
      && typeof item === 'string'
      && producedSources.has(item)
      ? `${item}/${suffix}`
      : namespaceWorkflowValues(item, suffix, producedSources);
  }
  if (recordKind === 'forEach') {
    if (typeof namespaced.identity === 'string') {
      const identitySuffix = suffix.replaceAll(/[^A-Za-z0-9-]/g, '-');
      namespaced.identity = `${namespaced.identity}-${identitySuffix}`;
    }
    const aggregate = namespaced.aggregate as Record<string, unknown> | undefined;
    if (aggregate && typeof aggregate.identity === 'string') {
      aggregate.identity = `${aggregate.identity}/${suffix}`;
      if (Array.isArray(aggregate.elements)) {
        aggregate.elements = aggregate.elements.map((element) =>
          typeof element === 'string' ? `${element}/${suffix}` : element,
        );
      }
    }
    const authoringAggregate = namespaced.__workflowAggregate as Record<string, unknown> | undefined;
    if (authoringAggregate && typeof authoringAggregate.identity === 'string') {
      authoringAggregate.identity = `${authoringAggregate.identity}/${suffix}`;
    }
  }
  return namespaced;
}

function collectProducedWorkflowSources(value: unknown): Set<string> {
  const sources = new Set<string>();
  const visit = (current: unknown) => {
    if (Array.isArray(current)) {
      for (const item of current) visit(item);
      return;
    }
    if (!current || typeof current !== 'object') return;
    const record = current as Record<string, unknown>;
    if (record.kind === 'effect') {
      const effect = record.effect as Record<string, unknown> | undefined;
      if (typeof effect?.resultIdentity === 'string') sources.add(effect.resultIdentity);
    } else if (typeof record.operation === 'string' && record.__workflowRef) {
      if (typeof record.resultIdentity === 'string') sources.add(record.resultIdentity);
    }
    for (const item of Object.values(record)) visit(item);
  };
  visit(value);
  return sources;
}

function isWorkflowCallback(node: ts.Node): boolean {
  if (!ts.isArrowFunction(node)) {
    return false;
  }
  const parent = node.parent;
  if (ts.isCallExpression(parent)) {
    const name = parent.expression.getText();
    return name === 'ForEach';
  }
  if (
    ts.isPropertyAssignment(parent)
    && ts.isObjectLiteralExpression(parent.parent)
    && ts.isCallExpression(parent.parent.parent)
  ) {
    return parent.parent.parent.expression.getText() === 'MatchResult';
  }
  return false;
}

export function analyzeDependencySource(
  filePath: string,
  requestedPackageName?: string,
): DependencySourceAnalysis {
  const normalizedPath = path.resolve(filePath);
  const sourceText = fs.readFileSync(normalizedPath, 'utf8');
  const sourceFile = ts.createSourceFile(
    normalizedPath,
    sourceText,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX,
  );
  const manifestImport = analyzeManifestImport(sourceFile, sourceText);
  const candidates: Array<{
    node: ts.JsxOpeningLikeElement;
    packageName?: string;
    authority: 'native' | 'annotation';
  }> = [];

  const visit = (node: ts.Node): void => {
    if (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) {
      if (node.tagName.getText(sourceFile) === 'Package' || node.tagName.getText(sourceFile) === 'PackageAnnotations') {
        candidates.push({
          node,
          packageName: jsxStringAttribute(node, 'name'),
          authority: node.tagName.getText(sourceFile) === 'PackageAnnotations' ? 'annotation' : 'native',
        });
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(sourceFile);

  const matching = requestedPackageName
    ? candidates.filter((candidate) => candidate.packageName === requestedPackageName)
    : candidates;
  if (matching.length > 1) {
    return dependencyAnalysisFailure(
      'Ambiguous',
      normalizedPath,
      requestedPackageName,
      'TSPACK_MANIFEST_DEPENDENCY_ISLAND_AMBIGUOUS',
      'Several package dependency surfaces match this projection request. Select one package explicitly.',
    );
  }
  if (matching.length === 0) {
    return dependencyAnalysisFailure(
      'Unsupported',
      normalizedPath,
      requestedPackageName,
      'TSPACK_MANIFEST_DEPENDENCY_ISLAND_NOT_FOUND',
      'No structurally recognizable Package or PackageAnnotations element matches this projection request.',
    );
  }

  const candidate = matching[0];
  const dependencyAttributes = jsxAttributes(candidate.node, 'dependencies');
  if (dependencyAttributes.length > 1) {
    return dependencyAnalysisFailure(
      'Ambiguous',
      normalizedPath,
      candidate.packageName,
      'TSPACK_MANIFEST_DEPENDENCY_ISLAND_AMBIGUOUS',
      'The selected package has several dependency attributes. TSPack will not guess which source surface owns the declaration.',
    );
  }
  const dependencies = dependencyAttributes[0];
  if (!dependencies) {
    const offset = ts.isJsxSelfClosingElement(candidate.node)
      ? candidate.node.getEnd() - 2
      : candidate.node.getEnd() - 1;
    const lineStart = sourceText.lastIndexOf('\n', offset - 1) + 1;
    const closingIndent = sourceText.slice(lineStart, offset).match(/^\s*/)?.[0] ?? '';
    const multiline = sourceText.slice(candidate.node.getStart(sourceFile), offset).includes('\n');
    return {
      status: 'Absent',
      authority: candidate.authority,
      manifestPath: normalizedPath,
      packageName: candidate.packageName,
      insertion: {
        offset: utf8Offset(sourceText, multiline ? lineStart : offset),
        multiline,
        attributeIndent: multiline ? closingIndent + detectIndentUnit(sourceText) : '',
        closingIndent,
      },
      ...(manifestImport ? { manifestImport } : {}),
      diagnostics: [],
    };
  }
  if (!dependencies.initializer || !ts.isJsxExpression(dependencies.initializer) || !dependencies.initializer.expression) {
    return dynamicDependencyAnalysis(normalizedPath, candidate.packageName);
  }
  const dependencyObject = dependencies.initializer.expression;
  if (!ts.isObjectLiteralExpression(dependencyObject)) {
    return dynamicDependencyAnalysis(normalizedPath, candidate.packageName);
  }
  const valuesProperties = dependencyObject.properties.filter((property): property is ts.PropertyAssignment => {
    return ts.isPropertyAssignment(property) && property.name.getText(sourceFile) === 'values';
  });
  if (valuesProperties.length > 1) {
    return dependencyAnalysisFailure(
      'Ambiguous',
      normalizedPath,
      candidate.packageName,
      'TSPACK_MANIFEST_DEPENDENCY_ISLAND_AMBIGUOUS',
      'The selected dependency object has several values properties. TSPack will not guess which array owns the declaration.',
    );
  }
  const valuesProperty = valuesProperties[0];
  if (!valuesProperty || !ts.isArrayLiteralExpression(valuesProperty.initializer)) {
    return dynamicDependencyAnalysis(normalizedPath, candidate.packageName);
  }

  const values = valuesProperty.initializer;
  const openBracket = values.getFirstToken(sourceFile);
  const closeBracket = values.getLastToken(sourceFile);
  if (!openBracket || !closeBracket) {
    return dependencyAnalysisFailure(
      'Unsupported',
      normalizedPath,
      candidate.packageName,
      'TSPACK_MANIFEST_DEPENDENCY_PROJECTION_UNSAFE',
      'The dependency values array has no stable source range.',
    );
  }
  const elements = values.elements.map((element) => ({
    start: utf8Offset(sourceText, element.getStart(sourceFile)),
    end: utf8Offset(sourceText, element.getEnd()),
    fullStart: utf8Offset(sourceText, element.getFullStart()),
  }));
  return {
    status: 'OwnedCanonical',
    authority: candidate.authority,
    manifestPath: normalizedPath,
    packageName: candidate.packageName,
    island: {
      start: utf8Offset(sourceText, dependencies.getStart(sourceFile)),
      end: utf8Offset(sourceText, dependencies.getEnd()),
      contentStart: utf8Offset(sourceText, openBracket.getEnd()),
      contentEnd: utf8Offset(sourceText, closeBracket.getStart(sourceFile)),
      elements,
    },
    ...(manifestImport ? { manifestImport } : {}),
    diagnostics: [],
  };
}

function analyzeManifestImport(
  sourceFile: ts.SourceFile,
  sourceText: string,
): DependencySourceAnalysis['manifestImport'] {
  const declarations = sourceFile.statements.filter((statement): statement is ts.ImportDeclaration => {
    return ts.isImportDeclaration(statement)
      && ts.isStringLiteral(statement.moduleSpecifier)
      && statement.moduleSpecifier.text === ALLOWED_IMPORT;
  });
  if (declarations.length !== 1) {
    return undefined;
  }
  const namedBindings = declarations[0].importClause?.namedBindings;
  if (!namedBindings || !ts.isNamedImports(namedBindings)) {
    return undefined;
  }
  const openBrace = namedBindings.getFirstToken(sourceFile);
  const closeBrace = namedBindings.getLastToken(sourceFile);
  if (!openBrace || !closeBrace) {
    return undefined;
  }
  return {
    contentStart: utf8Offset(sourceText, openBrace.getEnd()),
    contentEnd: utf8Offset(sourceText, closeBrace.getStart(sourceFile)),
    names: namedBindings.elements.map((element) => element.name.text),
  };
}

function jsxAttribute(node: ts.JsxOpeningLikeElement, name: string): ts.JsxAttribute | undefined {
  return jsxAttributes(node, name)[0];
}

function jsxAttributes(node: ts.JsxOpeningLikeElement, name: string): ts.JsxAttribute[] {
  const attributes: ts.JsxAttribute[] = [];
  for (const property of node.attributes.properties) {
    if (ts.isJsxAttribute(property) && property.name.getText() === name) {
      attributes.push(property);
    }
  }
  return attributes;
}

function jsxStringAttribute(node: ts.JsxOpeningLikeElement, name: string): string | undefined {
  const attribute = jsxAttribute(node, name);
  if (attribute?.initializer && ts.isStringLiteral(attribute.initializer)) {
    return attribute.initializer.text;
  }
  return undefined;
}

function dynamicDependencyAnalysis(manifestPath: string, packageName?: string): DependencySourceAnalysis {
  return dependencyAnalysisFailure(
    'UserDynamic',
    manifestPath,
    packageName,
    'TSPACK_MANIFEST_DEPENDENCIES_DYNAMIC',
    'Package dependencies are not a literal { values: [...] } surface. TSPack can execute supported manifests without claiming this source is safely editable.',
  );
}

function dependencyAnalysisFailure(
  status: DependencyIslandStatus,
  manifestPath: string,
  packageName: string | undefined,
  code: string,
  message: string,
): DependencySourceAnalysis {
  return {
    status,
    manifestPath,
    ...(packageName ? { packageName } : {}),
    diagnostics: [{
      code,
      severity: 'error',
      message,
      file: manifestPath,
    }],
  };
}

function utf8Offset(sourceText: string, utf16Offset: number): number {
  return Buffer.byteLength(sourceText.slice(0, utf16Offset), 'utf8');
}

function detectIndentUnit(sourceText: string): string {
  const indents = sourceText.match(/^[\t ]+(?=\S)/gm) ?? [];
  if (indents.some((indent) => indent.startsWith('\t'))) {
    return '\t';
  }
  let smallest = Number.POSITIVE_INFINITY;
  for (const indent of indents) {
    if (indent.length > 0 && indent.length < smallest) {
      smallest = indent.length;
    }
  }
  return Number.isFinite(smallest) ? ' '.repeat(smallest) : '  ';
}

function validateSkyrimTargetSyntax(
  node: ts.JsxSelfClosingElement | ts.JsxOpeningElement,
  sf: ts.SourceFile,
  addDiag: (node: ts.Node, code: string, message: string) => void,
): void {
  const attributes = new Map<string, ts.JsxAttribute>();
  for (const property of node.attributes.properties) {
    if (ts.isJsxAttribute(property)) {
      attributes.set(property.name.getText(sf), property);
    }
  }
  const required = [
    'name', 'host', 'runtimeVersion', 'bridge', 'nativeConfigure', 'nativeBuild',
    'nativeTests', 'nativeDll', 'assetCompilerProject', 'assetTestsProject',
    'assetPacks', 'assetOutput', 'runtimeConfig', 'dllDestination',
    'configDestination', 'expectedRecords', 'runtimeEvidencePattern', 'readyMarker',
  ];
  for (const name of required) {
    if (!attributes.has(name)) {
      addDiag(node, 'TSPACK_SKYRIM_REQUIRED_FIELD', `SkyrimTarget ${name} is required.`);
    }
  }
  const stringValue = (name: string): string | undefined => {
    const initializer = attributes.get(name)?.initializer;
    return initializer && ts.isStringLiteral(initializer) ? initializer.text : undefined;
  };
  if (stringValue('bridge') !== undefined && stringValue('bridge') !== 'MarionetteSSE.esp') {
    addDiag(attributes.get('bridge')!, 'TSPACK_SKYRIM_BRIDGE_INVALID', 'SkyrimTarget bridge must be MarionetteSSE.esp.');
  }
  if (stringValue('assetOutput') !== undefined && stringValue('assetOutput') !== 'build/assets/MarionetteSSE.esp') {
    addDiag(attributes.get('assetOutput')!, 'TSPACK_SKYRIM_BRIDGE_INVALID', 'SkyrimTarget assetOutput must be build/assets/MarionetteSSE.esp.');
  }
  for (const name of ['nativeDll', 'assetCompilerProject', 'assetTestsProject', 'assetOutput', 'runtimeConfig', 'runtimeEvidencePattern']) {
    const value = stringValue(name);
    if (value !== undefined && (path.isAbsolute(value) || /^[A-Za-z]:[\\/]/.test(value))) {
      addDiag(attributes.get(name)!, 'TSPACK_SKYRIM_PATH_INVALID', `SkyrimTarget ${name} must be project-relative.`);
    }
  }
}

function validateManifestEditorTsConfigOptions(
  options: unknown,
  diags: Diagnostic[],
  file: string,
): { include: string[]; exclude: string[] } | undefined {
  if (options === undefined) {
    return {
      include: [...DEFAULT_MANIFEST_EDITOR_INCLUDE],
      exclude: [...DEFAULT_MANIFEST_EDITOR_EXCLUDE],
    };
  }

  if (!options || typeof options !== 'object' || Array.isArray(options)) {
    diags.push({
      code: 'TSPACK_MANIFEST_INVALID_HELPER_ARGUMENT',
      severity: 'error',
      message: 'TsConfig.manifestEditor options must be an object with optional include/exclude string arrays',
      file,
    });
    return undefined;
  }

  const record = options as Record<string, unknown>;
  const include = validateManifestEditorPatternList(
    'include',
    record.include,
    DEFAULT_MANIFEST_EDITOR_INCLUDE,
    diags,
    file,
  );
  const exclude = validateManifestEditorPatternList(
    'exclude',
    record.exclude,
    DEFAULT_MANIFEST_EDITOR_EXCLUDE,
    diags,
    file,
  );

  if (!include || !exclude) {
    return undefined;
  }

  return { include, exclude };
}

function validateManifestEditorPatternList(
  key: 'include' | 'exclude',
  value: unknown,
  defaults: readonly string[],
  diags: Diagnostic[],
  file: string,
): string[] | undefined {
  if (value === undefined) {
    return [...defaults];
  }
  if (!Array.isArray(value)) {
    diags.push({
      code: 'TSPACK_MANIFEST_INVALID_HELPER_ARGUMENT',
      severity: 'error',
      message: `TsConfig.manifestEditor ${key} must be an array of safe non-empty relative path globs`,
      file,
    });
    return undefined;
  }

  const out: string[] = [];
  for (const entry of value) {
    if (typeof entry !== 'string') {
      diags.push({
        code: 'TSPACK_MANIFEST_INVALID_HELPER_ARGUMENT',
        severity: 'error',
        message: `TsConfig.manifestEditor ${key} entries must be strings`,
        file,
      });
      return undefined;
    }

    if (!isSafeRelativeGlob(entry)) {
      diags.push({
        code: 'TSPACK_MANIFEST_INVALID_HELPER_ARGUMENT',
        severity: 'error',
        message: `TsConfig.manifestEditor ${key} entries must be safe non-empty relative path globs`,
        file,
      });
      return undefined;
    }

    out.push(entry);
  }

  return out;
}

function buildVSCodeSettings(input: unknown, diags: Diagnostic[], file: string): Record<string, unknown> | undefined {
  if (input === undefined) {
    return defaultVSCodeSettings();
  }

  if (!input || typeof input !== 'object' || Array.isArray(input)) {
    diags.push({
      code: 'TSPACK_MANIFEST_INVALID_HELPER_ARGUMENT',
      severity: 'error',
      message: 'VSCode.settings options must be a JSON-compatible object',
      file,
    });
    return undefined;
  }

  const record = { ...(input as Record<string, unknown>) };
  if (record.typescriptTsdk !== undefined) {
    if (typeof record.typescriptTsdk !== 'string' || !isSafeRelativeVSCodePath(record.typescriptTsdk)) {
      diags.push({
        code: 'TSPACK_MANIFEST_INVALID_HELPER_ARGUMENT',
        severity: 'error',
        message: 'VSCode.settings typescriptTsdk must be a safe non-empty relative path',
        file,
      });
      return undefined;
    }

    record['typescript.tsdk'] = record.typescriptTsdk;
    delete record.typescriptTsdk;
  }

  return {
    ...defaultVSCodeSettings(),
    ...record,
  };
}

function defaultVSCodeSettings(): Record<string, unknown> {
  return {
    'typescript.tsdk': 'node_modules/typescript/lib',
    'typescript.enablePromptUseWorkspaceTsdk': true,
  };
}

function defaultVSCodeExtensions(): Record<string, unknown> {
  return {
    recommendations: ['biomejs.biome'],
  };
}

function jsxToRootDoc(root: unknown, diags: Diagnostic[], file: string): InternalDoc {
  const r = root as any;
  const children = r?.__children ?? [];
  const inlinePackages = children.filter((c: any) => c.__tag === 'Package');
  const packagesNode = children.find((c: any) => c.__tag === 'Packages');
  const securityNode = children.find((c: any) => c.__tag === 'Security');
  const updatePolicyNode = children.find((c: any) => c.__tag === 'UpdatePolicy');
  const registryPolicyNode = children.find((c: any) => c.__tag === 'RegistryPolicy');
  const compatFilesNode = children.find((c: any) => c.__tag === 'CompatFiles');
  const workflowsNode = children.find((c: any) => c.__tag === 'Workflows');
  const baseIr = {
    format: 1 as const,
    workspace: { name: r?.name ?? 'workspace', runtime: runtimeProfile(r?.runtime, diags, file) },
    ...(securityNode ? { security: mapSecurity(securityNode) } : {}),
    ...(updatePolicyNode ? { updatePolicy: mapUpdatePolicy(updatePolicyNode) } : {}),
    ...(registryPolicyNode ? { registryPolicy: mapRegistryPolicy(registryPolicyNode) } : {}),
    ...(compatFilesNode ? { compatFiles: mapCompatFiles(compatFilesNode, diags, file) } : {}),
    ...(workflowsNode ? { workflows: workflowsNode.rows ?? [] } : {}),
  };
  if (packagesNode && inlinePackages.length > 0) {
    return { mode: 'split', ir: { ...baseIr, packages: [] }, rows: [] };
  }
  if (packagesNode) {
    return { mode: 'split', ir: { ...baseIr, packages: [] }, rows: (packagesNode.rows as PackageRow[]) ?? [] };
  }
  const context: AuthoringContext = {
    originKind: 'project-manifest',
    sourcePath: 'manifest.tsx',
    layer: 'project',
  };
  return {
    mode: 'single',
    ir: { ...baseIr, packages: inlinePackages.map((p: any) => mapPackage(p, false, context)) },
  };
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

function mapCompatFiles(compatFiles: any, diags: Diagnostic[], file: string): Array<Record<string, unknown>> {
  const rows = compatFiles.__children?.filter((child: any) => child.__tag === 'JsonFile') ?? [];
  return rows.map((row: any) => {
    if (typeof row.path !== 'string') {
      diags.push({ code: 'TSPACK_COMPAT_PATH_INVALID', severity: 'error', message: 'JsonFile path is required', file });
    }
    if (!Object.prototype.hasOwnProperty.call(row, 'value')) {
      diags.push({ code: 'TSPACK_COMPAT_VALUE_INVALID', severity: 'error', message: 'JsonFile value is required', file });
    }
    if (!isJSONCompatible(row.value)) {
      diags.push({ code: 'TSPACK_COMPAT_VALUE_INVALID', severity: 'error', message: 'JsonFile value must be JSON-compatible', file });
    }
    return { path: row.path, format: 'json', value: row.value };
  });
}

function isJSONCompatible(value: unknown): boolean {
  if (value === null) return true;
  if (typeof value === 'string' || typeof value === 'boolean') return true;
  if (typeof value === 'number') return Number.isFinite(value);
  if (Array.isArray(value)) return value.every((item) => isJSONCompatible(item));
  if (value && typeof value === 'object') {
    return Object.values(value as Record<string, unknown>).every((item) => isJSONCompatible(item));
  }
  return false;
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


function jsxToPackageAnnotationDoc(root: unknown, diags: Diagnostic[], file: string): InternalDoc {
  const p = root as any;
  if (p?.__tag !== 'PackageAnnotations') {
    diags.push({ code: 'TSPACK_MANIFEST_PACKAGE_ANNOTATION_INVALID', severity: 'error', message: 'annotatePackage(...) must wrap <PackageAnnotations />', file });
  }
  return {
    mode: 'annotation',
    ir: {
      format: 1,
      workspace: { name: 'workspace', runtime: 'nodejs' },
      packages: [],
      packageAnnotations: [mapPackageAnnotation(p)],
    },
  };
}

function mapPackageAnnotation(p: any): Record<string, unknown> {
  const values = p.dependencies?.values ?? [];
  const context: AuthoringContext = {
    originKind: 'package-manifest',
    sourcePath: 'package.manifest.tsx',
    layer: 'package',
    ...(p.dependencyDeclaration !== undefined
      ? { declarationDefaults: p.dependencyDeclaration as Record<string, unknown> }
      : {}),
  };
  return {
    name: p.name,
    manifestPath: context.sourcePath,
    dependencies: mapDependencies(values),
    dependencyAuthoring: mapDependencyAuthoring(values, context),
  };
}

function mapRegistryPolicy(policy: any): Record<string, unknown> {
  const sources = policy.__children
    ?.filter((child: any) => child.__tag === 'RegistrySource')
    .map((source: any) => ({
      kind: source.kind,
      endpoints: source.endpoints ?? [],
    })) ?? [];
  return {
    ...(policy.allowedSources !== undefined ? { allowedSources: policy.allowedSources } : {}),
    offline: policy.offline ?? false,
    requireIntegrity: policy.requireIntegrity ?? false,
    requireAuditCoverage: policy.requireAuditCoverage ?? false,
    sources,
  };
}

function jsxToPackageDoc(root: unknown): InternalDoc {
  const p = root as any;
  const context: AuthoringContext = {
    originKind: 'package-manifest',
    sourcePath: 'package.manifest.tsx',
    layer: 'package',
  };
  return {
    mode: 'package',
    ir: {
      format: 1,
      workspace: { name: 'workspace', runtime: 'nodejs' },
      packages: [mapPackage(p, false, context)],
    },
  };
}

function mapPackage(p: any, includeRoot: boolean, context: AuthoringContext): Record<string, unknown> {
  const dependencyValues = p.dependencies?.values ?? [];
  const authoringContext = {
    ...context,
    ...(p.dependencyDeclaration !== undefined
      ? { declarationDefaults: p.dependencyDeclaration as Record<string, unknown> }
      : {}),
  };
  return {
    name: p.name,
    ...(p.publicationName !== undefined ? { publicationName: p.publicationName } : {}),
    version: p.version,
    ...(includeRoot ? { root: '.' } : {}),
    license: p.license,
    kind: p.kind,
    ...(p.compiler !== undefined ? { compiler: compilerIdentity(p.compiler) } : {}),
    ...(p.compilerPath !== undefined ? { compilerPath: p.compilerPath } : {}),
	...(p.devBackend !== undefined ? { devBackend: p.devBackend } : {}),
    dependencies: mapDependencies(dependencyValues),
    dependencyAuthoring: mapDependencyAuthoring(dependencyValues, authoringContext),
    targets: mapTargets(p.__children?.find((x: any) => x.__tag === 'Targets')?.rows ?? []),
    ...(p.__children?.find((x: any) => x.__tag === 'RunTargets') ? { runTargets: p.__children?.find((x: any) => x.__tag === 'RunTargets')?.rows ?? [] } : {}),
    ...(p.__children?.find((x: any) => x.__tag === 'TestTargets') ? { testTargets: mapTestTargets(p.__children?.find((x: any) => x.__tag === 'TestTargets')?.rows ?? []) } : {}),
    ...(p.__children?.find((x: any) => x.__tag === 'SkyrimTarget') ? { skyrim: mapSkyrimTarget(p.__children?.find((x: any) => x.__tag === 'SkyrimTarget')) } : {}),
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

    const { __key: _key, declaration: _declaration, ...dependency } = value;
    return dependency;
  });
}

function mapDependencyAuthoring(values: any[], context: AuthoringContext): Record<string, unknown> {
  return {
    declarations: values.map((value, order) => mapDependencyDeclaration(value, order, context)),
  };
}

function mapDependencyDeclaration(value: any, order: number, context: AuthoringContext): Record<string, unknown> {
  const metadata = {
    ...(context.declarationDefaults ?? {}),
    ...(value?.declaration ?? {}),
  };
  const source = value?.source ?? {};
  const sourceName = source.package ?? source.name ?? source.ref ?? source.path ?? source.repo ?? '';
  const origin = metadata.origin ?? {
    kind: context.originKind,
    sourcePath: context.sourcePath,
  };
  const originKind = origin.kind ?? context.originKind;
  return {
    ...(metadata.id !== undefined ? { id: metadata.id } : {}),
    ...(value?.key !== undefined ? { key: value.key } : {}),
    identity: {
      source: source.kind,
      name: sourceName,
    },
    source,
    ...(source.range !== undefined ? { constraint: source.range } : {}),
    kind: value?.kind,
    ...(value?.optional === true ? { optional: true } : {}),
    ...(value?.patch !== undefined ? { patch: value.patch } : {}),
    origin: {
      ...origin,
      ...(origin.sourcePath === undefined ? { sourcePath: context.sourcePath } : {}),
    },
    layer: metadata.layer ?? context.layer,
    ...(metadata.layerOrder !== undefined ? { layerOrder: metadata.layerOrder } : {}),
    order,
    authority: metadata.authority ?? 'owned',
    editability: metadata.editability ?? defaultEditability(originKind),
  };
}

function defaultEditability(originKind: string): string {
  if (originKind === 'concept') {
    return 'concept-owned';
  }
  if (originKind === 'template') {
    return 'generated';
  }
  if (originKind === 'compatibility') {
    return 'observed';
  }
  return 'editable';
}

function withDependencyAuthoringSourcePath(
  pkg: Record<string, unknown>,
  sourcePath: string,
): Record<string, unknown> {
  const authoring = pkg.dependencyAuthoring as { declarations?: Array<Record<string, unknown>> } | undefined;
  if (!authoring?.declarations) {
    return pkg;
  }
  return {
    ...pkg,
    dependencyAuthoring: {
      ...authoring,
      declarations: authoring.declarations.map((declaration) => {
        const origin = declaration.origin as Record<string, unknown> | undefined;
        return {
          ...declaration,
          origin: {
            ...origin,
            sourcePath,
          },
        };
      }),
    },
  };
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

function mapTestTargets(rows: any[]): any[] {
  return rows.map((row) => {
    const requirements = Array.isArray(row?.requirements) ? row.requirements : [];
    const fixtures = Array.isArray(row?.fixtures) ? row.fixtures : [];
    const builtFixtures = Array.isArray(row?.builtFixtures) ? row.builtFixtures : [];
    return {
      ...row,
      ...(requirements.length > 0 ? { requirements: mapDependencyRefs(requirements) } : {}),
      ...(fixtures.length > 0
        ? {
            fixtures: fixtures.map((fixture: any) => ({
              ...fixture,
              dependency: dependencyRefKey(fixture?.dependency),
              mode: fixture?.mode ?? 'package',
            })),
          }
        : {}),
      ...(builtFixtures.length > 0
        ? {
            builtFixtures: builtFixtures.map((fixture: any) => ({
              ...fixture,
              target: String(fixture?.target ?? ''),
            })),
          }
        : {}),
    };
  });
}

function mapSkyrimTarget(target: any): Record<string, unknown> {
  const { __tag: _tag, __children: _children, ...value } = target;
  return value;
}

function compilerIdentity(value: unknown): Compiler {
  if (value === undefined) return 'tsc';
  if (value === 'tsc' || value === 'tscl' || value === 'scriptc' || value === 'perry' || value === 'rollup') return value;
  return value as Compiler;
}

function isSafeRelativeGlob(value: string): boolean {
  return !!value && !path.isAbsolute(value) && !value.includes('..') && !value.includes('\\') && !value.includes('://') && !value.includes(':');
}

function isSafeRelativeVSCodePath(value: string): boolean {
  return !!value && !path.isAbsolute(value) && !value.includes('..') && !value.includes('\\') && !value.includes('://') && !value.includes(':');
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
