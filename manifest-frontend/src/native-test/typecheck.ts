import path from "node:path";
import ts from "typescript";
import type { Diagnostic } from "./types.js";

type TestContext = {
  localId: string;
  name: string;
  start: number;
  end: number;
};

export type TypeAssertionCall = {
  file: string;
  start: number;
  end: number;
  line: number;
  column: number;
  expectedTypeText: string;
  reason?: string;
  localTestId?: string;
  testName?: string;
};

export type TypeAssertionDiagnostic = Diagnostic & {
  reason?: string;
  expectedTypeText?: string;
  localTestId?: string;
  testName?: string;
  typescriptCode?: number;
  typescriptMessage?: string;
};

export type NativeTypecheckResult = {
  assertions: TypeAssertionCall[];
  diagnostics: TypeAssertionDiagnostic[];
};

const nativeTypecheckGlobals = `
type TspackNativeChild = unknown;

type TspackInspectBounds = {
  x: number;
  y: number;
  width: number;
  height: number;
};
type TspackInspectNode = {
  id: string;
  tag: string;
  role?: string;
  name?: string;
  text?: string;
  bounds: TspackInspectBounds;
  visible: boolean;
  focusable?: boolean;
  style?: Record<string, string | undefined>;
  children: TspackInspectNode[];
};
type TspackInspectHitTest = {
  point: { x: number; y: number };
  elements: TspackInspectNode[];
};
type TspackInspectResult = {
  target: { url: string };
  browser: { name: string; backend?: string };
  viewport: { width: number; height: number; deviceScaleFactor?: number };
  root: TspackInspectNode | null;
  hitTests: TspackInspectHitTest[];
  diagnostics: Array<{ code: string; message: string }>;
};
type TspackInspectPoint = { x: number; y: number };
type TspackInspectViewport = string | { width: number; height: number };
type TspackInspectUrlOptions = {
  browser?: "chromium" | "webkit" | "playwright-chromium" | "playwright-webkit";
  selector?: string;
  viewport?: TspackInspectViewport;
  points?: TspackInspectPoint[];
};
type TspackInspectCdpOptions = {
  target?: number | string;
  targetUrl?: string;
  selector?: string;
  viewport?: TspackInspectViewport;
  points?: TspackInspectPoint[];
};
type TspackNativeNode = unknown;
type TspackNativeProps = Record<string, unknown> | null | undefined;
type TspackNativeTag = (
  props?: TspackNativeProps,
  ...children: TspackNativeChild[]
) => TspackNativeNode;
declare const Suite: TspackNativeTag;
declare const Fact: TspackNativeTag;
declare const Theory: TspackNativeTag;
declare const Case: TspackNativeTag;
declare const Artifact: TspackNativeTag;
declare const Valid: TspackNativeTag;
declare const Invalid: TspackNativeTag;
declare const Project: TspackNativeTag;
declare const CycleTime: TspackNativeTag;
declare const Benchmark: TspackNativeTag;
declare const Iterations: TspackNativeTag;
declare const Warmup: TspackNativeTag;
declare const Prophecy: TspackNativeTag;
declare const Foretell: TspackNativeTag;
declare const assert: {
  type<TExpected>(value: TExpected, reason: string): void;
};
declare const expect: unknown;
declare const skip: unknown;
declare const inspect: {
  url(url: string, options?: TspackInspectUrlOptions): Promise<TspackInspectResult>;
  cdp(endpoint: string, options?: TspackInspectCdpOptions): Promise<TspackInspectResult>;
  target(endpoint: string, options?: TspackInspectCdpOptions): Promise<TspackInspectResult>;
  cdpTarget(endpoint: string, options?: TspackInspectCdpOptions): Promise<TspackInspectResult>;
  flatten(root: TspackInspectNode | null | undefined): TspackInspectNode[];
  findByRole(root: TspackInspectNode | null | undefined, role: string, name?: string | RegExp): TspackInspectNode | undefined;
  findByText(root: TspackInspectNode | null | undefined, text: string | RegExp): TspackInspectNode | undefined;
};
declare namespace JSX {
  interface IntrinsicElements {
    Suite: Record<string, unknown>;
    Fact: Record<string, unknown>;
    Theory: Record<string, unknown>;
    Case: Record<string, unknown>;
    Artifact: Record<string, unknown>;
    Valid: Record<string, unknown>;
    Invalid: Record<string, unknown>;
    Project: Record<string, unknown>;
    CycleTime: Record<string, unknown>;
    Benchmark: Record<string, unknown>;
    Iterations: Record<string, unknown>;
    Warmup: Record<string, unknown>;
    Prophecy: Record<string, unknown>;
    Foretell: Record<string, unknown>;
  }
}
`;

export function typecheckNativeTestFile(
  filePath: string,
  options: { rootDir?: string } = {},
): NativeTypecheckResult {
  const absoluteFilePath = path.resolve(filePath);
  const rootDir = path.resolve(
    options.rootDir ?? path.dirname(absoluteFilePath),
  );
  const sourceText = ts.sys.readFile(absoluteFilePath);
  if (!sourceText || !sourceText.includes("assert.type")) {
    return { assertions: [], diagnostics: [] };
  }

  const ambientFilePath = path.join(rootDir, ".tspack-native-typecheck.d.ts");
  const compilerOptions = createCompilerOptions();
  const host = createCompilerHostWithAmbientFile(
    compilerOptions,
    ambientFilePath,
  );
  const program = ts.createProgram({
    rootNames: [absoluteFilePath, ambientFilePath],
    options: compilerOptions,
    host,
  });
  const sourceFile = program.getSourceFile(absoluteFilePath);

  if (!sourceFile) {
    return {
      assertions: [],
      diagnostics: [
        {
          code: "TSPACK_TEST_TYPECHECK_FAILED",
          message: `failed to load TypeScript source file: ${absoluteFilePath}`,
          file: absoluteFilePath,
          severity: "error",
        },
      ],
    };
  }

  const testContexts = collectTestContexts(sourceFile);
  const assertions = collectTypeAssertionCalls(sourceFile, testContexts);
  const diagnostics = collectAssertionDiagnostics(program, assertions);

  return { assertions, diagnostics };
}

function createCompilerOptions(): ts.CompilerOptions {
  return {
    strict: true,
    target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.ESNext,
    moduleResolution: ts.ModuleResolutionKind.Bundler,
    jsx: ts.JsxEmit.Preserve,
    allowJs: true,
    checkJs: false,
    noEmit: true,
    skipLibCheck: true,
    allowSyntheticDefaultImports: true,
    esModuleInterop: true,
  };
}

function createCompilerHostWithAmbientFile(
  compilerOptions: ts.CompilerOptions,
  ambientFilePath: string,
): ts.CompilerHost {
  const host = ts.createCompilerHost(compilerOptions, true);
  const originalFileExists = host.fileExists.bind(host);
  const originalReadFile = host.readFile.bind(host);
  const originalGetSourceFile = host.getSourceFile.bind(host);

  host.fileExists = (filePath) => {
    if (path.resolve(filePath) === ambientFilePath) {
      return true;
    }
    return originalFileExists(filePath);
  };

  host.readFile = (filePath) => {
    if (path.resolve(filePath) === ambientFilePath) {
      return nativeTypecheckGlobals;
    }
    return originalReadFile(filePath);
  };

  host.getSourceFile = (
    filePath,
    languageVersion,
    onError,
    shouldCreateNewSourceFile,
  ) => {
    if (path.resolve(filePath) === ambientFilePath) {
      return ts.createSourceFile(
        ambientFilePath,
        nativeTypecheckGlobals,
        languageVersion,
        true,
        ts.ScriptKind.TS,
      );
    }
    return originalGetSourceFile(
      filePath,
      languageVersion,
      onError,
      shouldCreateNewSourceFile,
    );
  };

  return host;
}

function collectTypeAssertionCalls(
  sourceFile: ts.SourceFile,
  testContexts: TestContext[],
): TypeAssertionCall[] {
  const assertions: TypeAssertionCall[] = [];

  function visit(node: ts.Node): void {
    if (ts.isCallExpression(node) && isAssertTypeCall(node)) {
      const position = sourceFile.getLineAndCharacterOfPosition(
        node.getStart(sourceFile),
      );
      const typeArgument = node.typeArguments?.[0];
      const reasonArgument = node.arguments[1];
      const reason = readReasonArgument(reasonArgument);
      const testContext = findContainingTestContext(node, testContexts);

      assertions.push({
        file: sourceFile.fileName,
        start: node.getStart(sourceFile),
        end: node.getEnd(),
        line: position.line + 1,
        column: position.character + 1,
        expectedTypeText: typeArgument?.getText(sourceFile) ?? "unknown",
        reason,
        localTestId: testContext?.localId,
        testName: testContext?.name,
      });
    }

    ts.forEachChild(node, visit);
  }

  visit(sourceFile);
  return assertions;
}

function collectAssertionDiagnostics(
  program: ts.Program,
  assertions: TypeAssertionCall[],
): TypeAssertionDiagnostic[] {
  const diagnostics: TypeAssertionDiagnostic[] = [];

  for (const assertion of assertions) {
    if (!assertion.reason || assertion.reason.trim().length === 0) {
      diagnostics.push({
        code: "TSPACK_TYPE_ASSERTION_REASON_REQUIRED",
        message: "assert.type requires a non-empty string literal reason",
        file: assertion.file,
        line: assertion.line,
        column: assertion.column,
        severity: "error",
        reason: assertion.reason,
        expectedTypeText: assertion.expectedTypeText,
        localTestId: assertion.localTestId,
        testName: assertion.testName,
      });
    }
  }

  const semanticDiagnostics = program.getSemanticDiagnostics();
  for (const diagnostic of semanticDiagnostics) {
    const assertion = findOverlappingAssertion(diagnostic, assertions);
    if (!assertion) {
      continue;
    }
    if (diagnostics.some((entry) => sameLocation(entry, assertion))) {
      continue;
    }

    diagnostics.push({
      code: "TSPACK_TYPE_ASSERTION_FAILED",
      message: "type assertion failed",
      file: assertion.file,
      line: assertion.line,
      column: assertion.column,
      severity: "error",
      reason: assertion.reason,
      expectedTypeText: assertion.expectedTypeText,
      localTestId: assertion.localTestId,
      testName: assertion.testName,
      typescriptCode: diagnostic.code,
      typescriptMessage: ts.flattenDiagnosticMessageText(
        diagnostic.messageText,
        "\n",
      ),
    });
  }

  return diagnostics;
}

function sameLocation(
  diagnostic: TypeAssertionDiagnostic,
  assertion: TypeAssertionCall,
): boolean {
  return (
    diagnostic.file === assertion.file &&
    diagnostic.line === assertion.line &&
    diagnostic.column === assertion.column
  );
}

function findOverlappingAssertion(
  diagnostic: ts.Diagnostic,
  assertions: TypeAssertionCall[],
): TypeAssertionCall | undefined {
  if (!diagnostic.file || diagnostic.start === undefined) {
    return undefined;
  }

  const diagnosticStart = diagnostic.start;
  const diagnosticEnd = diagnostic.start + (diagnostic.length ?? 0);
  const diagnosticFile = path.resolve(diagnostic.file.fileName);

  return assertions.find((assertion) => {
    if (path.resolve(assertion.file) !== diagnosticFile) {
      return false;
    }
    return assertion.start <= diagnosticEnd && diagnosticStart <= assertion.end;
  });
}

function isAssertTypeCall(node: ts.CallExpression): boolean {
  const expression = node.expression;
  if (!ts.isPropertyAccessExpression(expression)) {
    return false;
  }
  if (expression.name.text !== "type") {
    return false;
  }
  return (
    ts.isIdentifier(expression.expression) &&
    expression.expression.text === "assert"
  );
}

function readReasonArgument(
  argument: ts.Expression | undefined,
): string | undefined {
  if (!argument || !ts.isStringLiteralLike(argument)) {
    return undefined;
  }
  return argument.text;
}

function collectTestContexts(sourceFile: ts.SourceFile): TestContext[] {
  const contexts: TestContext[] = [];
  const suite = findDefaultExportedSuite(sourceFile);
  if (!suite) {
    return contexts;
  }

  const suiteName = readJsxNameAttribute(suite) ?? "";
  for (const child of getJsxChildren(suite)) {
    const tagName = getJsxTagName(child);
    if (tagName !== "Fact" && tagName !== "Theory") {
      continue;
    }

    const name = readJsxNameAttribute(child) ?? "";
    const callback = findJsxCallbackChild(child);
    if (!callback) {
      continue;
    }

    contexts.push({
      localId: `${suiteName}/${name}`,
      name,
      start: callback.getStart(sourceFile),
      end: callback.getEnd(),
    });
  }

  return contexts;
}

function findDefaultExportedSuite(
  sourceFile: ts.SourceFile,
): ts.JsxElement | undefined {
  for (const statement of sourceFile.statements) {
    if (!ts.isExportAssignment(statement)) {
      continue;
    }

    const expression = unwrapExpression(statement.expression);
    if (ts.isJsxElement(expression) && getJsxTagName(expression) === "Suite") {
      return expression;
    }
  }

  return undefined;
}

function unwrapExpression(expression: ts.Expression): ts.Expression {
  let current = expression;
  while (ts.isParenthesizedExpression(current) || ts.isAsExpression(current)) {
    current = current.expression;
  }
  return current;
}

function getJsxChildren(node: ts.JsxElement): ts.JsxElement[] {
  const children: ts.JsxElement[] = [];
  for (const child of node.children) {
    if (ts.isJsxElement(child)) {
      children.push(child);
    }
  }
  return children;
}

function getJsxTagName(node: ts.JsxElement): string {
  return node.openingElement.tagName.getText();
}

function readJsxNameAttribute(node: ts.JsxElement): string | undefined {
  for (const property of node.openingElement.attributes.properties) {
    if (!ts.isJsxAttribute(property)) {
      continue;
    }
    if (property.name.text !== "name") {
      continue;
    }
    if (!property.initializer || !ts.isStringLiteral(property.initializer)) {
      return undefined;
    }
    return property.initializer.text;
  }
  return undefined;
}

function findJsxCallbackChild(node: ts.JsxElement): ts.Expression | undefined {
  for (const child of node.children) {
    if (!ts.isJsxExpression(child) || !child.expression) {
      continue;
    }
    if (
      ts.isArrowFunction(child.expression) ||
      ts.isFunctionExpression(child.expression)
    ) {
      return child.expression;
    }
  }
  return undefined;
}

function findContainingTestContext(
  node: ts.Node,
  contexts: TestContext[],
): TestContext | undefined {
  const start = node.getStart();
  const end = node.getEnd();
  return contexts.find(
    (context) => context.start <= start && end <= context.end,
  );
}
