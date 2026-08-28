import fs from "node:fs";
import path from "node:path";
import ts from "typescript";
import type {
  Diagnostic,
  DiscoverFilesResult,
  DiscoverOptions,
  DiscoveredFile,
  DiscoveryResult,
  DiscoveredStandaloneArtifact,
  Literal,
} from "./types.js";

type NativeFileKind = "xtest" | "valid" | "invalid" | "benchmark" | "prophecy";

type AllowedElement =
  | "Fact"
  | "Theory"
  | "Artifact"
  | "Valid"
  | "Invalid"
  | "Project"
  | "CycleTime"
  | "Benchmark"
  | "Iterations"
  | "Warmup"
  | "Prophecy"
  | "Foretell";

const allAllowed = new Set<AllowedElement | "Suite" | "Case">([
  "Suite",
  "Fact",
  "Theory",
  "Case",
  "Artifact",
  "Valid",
  "Invalid",
  "Project",
  "CycleTime",
  "Benchmark",
  "Iterations",
  "Warmup",
  "Prophecy",
  "Foretell",
]);

function classifyNativeFile(filePath: string): NativeFileKind | undefined {
  if (filePath.endsWith(".xtest.tsx")) return "xtest";
  if (filePath.endsWith(".valid.tsx")) return "valid";
  if (filePath.endsWith(".invalid.tsx")) return "invalid";
  if (filePath.endsWith(".benchmark.tsx")) return "benchmark";
  if (filePath.endsWith(".prophecy.tsx")) return "prophecy";
  return undefined;
}

function allowedChildrenFor(kind: NativeFileKind): Set<AllowedElement> {
  if (kind === "xtest") {
    return new Set(["Fact", "Theory", "Artifact"]);
  }
  if (kind === "valid") {
    return new Set(["Valid"]);
  }
  if (kind === "benchmark") return new Set(["Benchmark"]);
  if (kind === "prophecy") return new Set(["Prophecy"]);
  return new Set(["Invalid"]);
}

export function discoverNativeTestFile(filePath: string): DiscoveryResult {
  const abs = path.resolve(filePath);
  const fileKind = classifyNativeFile(abs);
  if (!fileKind) {
    return {
      tests: [],
      facts: [],
      theories: [],
      invariants: [],
      standaloneArtifacts: [],
      diagnostics: [
        {
          code: "TSPACK_TEST_NON_NATIVE_FILE",
          message:
            "native test files must end with .xtest.tsx, .valid.tsx, .invalid.tsx, .benchmark.tsx, or .prophecy.tsx",
          file: abs,
        },
      ],
    };
  }

  const text = fs.readFileSync(abs, "utf8");
  const sf = ts.createSourceFile(
    abs,
    text,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX,
  );
  const diagnostics: Diagnostic[] = [];
  const addDiag = (node: ts.Node, code: string, message: string): void => {
    const lc = sf.getLineAndCharacterOfPosition(node.getStart(sf));
    diagnostics.push({
      code,
      message,
      file: abs,
      line: lc.line + 1,
      column: lc.character + 1,
    });
  };

  let exportExpr: ts.Expression | undefined;
  for (const statement of sf.statements) {
    if (ts.isExportAssignment(statement)) exportExpr = statement.expression;
  }
  if (!exportExpr) {
    diagnostics.push({
      code: "TSPACK_TEST_INVALID_DEFAULT_EXPORT",
      message: "default export must be a Suite JSX tree",
      file: abs,
    });
    return {
      diagnostics,
      tests: [],
      facts: [],
      theories: [],
      invariants: [],
      standaloneArtifacts: [],
      benchmarks: [],
      prophecies: [],
    };
  }

  const root = unwrap(exportExpr);
  if (!ts.isJsxElement(root) && !ts.isJsxSelfClosingElement(root)) {
    diagnostics.push({
      code: "TSPACK_TEST_INVALID_DEFAULT_EXPORT",
      message: "default export must be JSX",
      file: abs,
    });
    return {
      diagnostics,
      tests: [],
      facts: [],
      theories: [],
      invariants: [],
      standaloneArtifacts: [],
      benchmarks: [],
      prophecies: [],
    };
  }

  const tests: string[] = [];
  const facts: DiscoveryResult["facts"] = [];
  const theories: DiscoveryResult["theories"] = [];
  const invariants: DiscoveryResult["invariants"] = [];
  const standaloneArtifacts: DiscoveredStandaloneArtifact[] = [];
  const benchmarks: DiscoveryResult["benchmarks"] = [];
  const prophecies: DiscoveryResult["prophecies"] = [];

  const suiteName = parseName(
    getTagName(root),
    getAttributes(root),
    root,
    addDiag,
  );
  if (getTagName(root) !== "Suite") {
    addDiag(
      root,
      "TSPACK_TEST_INVALID_DEFAULT_EXPORT",
      "root element must be <Suite>",
    );
  }

  const allowedChildren = allowedChildrenFor(fileKind);
  const standaloneNames = new Set<string>();
  for (const child of getChildren(root)) {
    if (!ts.isJsxElement(child) && !ts.isJsxSelfClosingElement(child)) continue;

    const tag = getTagName(child) as AllowedElement;
    if (!allAllowed.has(tag)) {
      addDiag(child, "TSPACK_TEST_UNKNOWN_ELEMENT", `unknown element: ${tag}`);
      continue;
    }
    if (!allowedChildren.has(tag)) {
      addDiag(
        child,
        "TSPACK_TEST_INVALID_FILE_KIND_ELEMENT",
        `<${tag}> is only allowed in ${renderAllowedFileKind(tag)} files`,
      );
      continue;
    }

    if (tag === "Project") {
      addDiag(
        child,
        "TSPACK_PROJECT_FIXTURE_INVALID_DECLARATION",
        "<Project> must be declared inside an executable unit",
      );
      continue;
    }
    if (tag === "CycleTime") {
      addDiag(
        child,
        "TSPACK_TEST_CYCLETIME_NOT_ALLOWED",
        "<CycleTime> is only allowed under Theory or suite-level Artifact",
      );
      continue;
    }

    if (tag === "Artifact") {
      const suiteArtifact = parseSuiteArtifact(
        child,
        suiteName,
        abs,
        path.dirname(abs),
        addDiag,
      );
      if (suiteArtifact) {
        if (standaloneNames.has(suiteArtifact.name)) {
          addDiag(
            child,
            "TSPACK_ARTIFACT_DUPLICATE_NAME",
            `duplicate Artifact name: ${suiteArtifact.name}`,
          );
        } else {
          standaloneNames.add(suiteArtifact.name);
          standaloneArtifacts.push(suiteArtifact);
        }
      }
      continue;
    }

    if (tag === "Fact") {
      const factName = parseName(tag, getAttributes(child), child, addDiag);
      if (!getJsxBodyFunction(child)) {
        addDiag(
          child,
          "TSPACK_TEST_MISSING_BODY",
          "Fact requires callback body",
        );
        continue;
      }
      const id = `${suiteName}/${factName}`;
      collectCycleTimeForbiddenChildren(child, addDiag);
      facts.push({
        kind: "fact",
        name: factName,
        id,
        artifacts: collectArtifacts(child, addDiag),
        project: collectProject(child, path.dirname(abs), addDiag),
      });
      tests.push(id);
      continue;
    }

    if (tag === "Theory") {
      const theoryName = parseName(tag, getAttributes(child), child, addDiag);
      const validation = validateTheoryStructure(child, addDiag);
      const cases = collectCases(child, suiteName, theoryName, addDiag);
      if (!validation.hasBody) {
        addDiag(
          child,
          "TSPACK_TEST_THEORY_MISSING_BODY",
          "Theory requires exactly one callback body",
        );
      }
      if (validation.hasDuplicateBody) {
        addDiag(
          validation.duplicateBodyNode ?? child,
          "TSPACK_TEST_THEORY_DUPLICATE_BODY",
          "Theory allows only one callback body",
        );
      }
      if (cases.length === 0) {
        addDiag(
          child,
          "TSPACK_TEST_THEORY_NO_CASES",
          "Theory requires at least one Case child",
        );
      }
      if (!validation.isValid || cases.length === 0) {
        continue;
      }
      theories.push({
        kind: "theory",
        name: theoryName,
        cases,
        artifacts: collectArtifacts(child, addDiag),
        project: collectProject(child, path.dirname(abs), addDiag),
        cycleTimeSeconds: collectCycleTimeAllowed(child, addDiag),
      });
      for (const entry of cases) tests.push(entry.id);
      continue;
    }
    if (tag === "Benchmark") {
      const benchmarkName = parseName(
        tag,
        getAttributes(child),
        child,
        addDiag,
      );
      if (!getJsxBodyFunction(child)) {
        addDiag(
          child,
          "TSPACK_TEST_MISSING_BODY",
          "Benchmark requires callback body",
        );
        continue;
      }
      const id = `${suiteName}/benchmark/${benchmarkName}`;
      const iterations = collectBenchmarkCount(
        child,
        "Iterations",
        1000,
        addDiag,
      );
      const warmup = collectBenchmarkCount(child, "Warmup", 100, addDiag);
      const cycleTimeSeconds = collectCycleTimeAllowed(child, addDiag) ?? 60;
      const project = collectProject(child, path.dirname(abs), addDiag);
      benchmarks.push({
        id: `${path.basename(abs)}::${id}`,
        filePath: abs,
        suiteName,
        name: benchmarkName,
        iterations,
        warmup,
        cycleTimeSeconds,
        project,
      });
      tests.push(id);
      continue;
    }
    if (tag === "Prophecy") {
      const prophecyName = parseName(tag, getAttributes(child), child, addDiag);
      if (!getJsxBodyFunction(child)) {
        addDiag(
          child,
          "TSPACK_DOOM_MISSING_BODY",
          "Prophecy requires callback body",
        );
        continue;
      }
      const foretell = collectForetell(child, addDiag);
      if (!foretell) {
        continue;
      }
      const cycleTimeSeconds = collectCycleTimeAllowed(child, addDiag) ?? 30;
      const id = `${path.basename(abs)}::${suiteName}/prophecy/${prophecyName}`;
      prophecies.push({
        id,
        filePath: abs,
        suiteName,
        name: prophecyName,
        foretell,
        cycleTimeSeconds,
      });
      continue;
    }

    if (tag === "Valid" || tag === "Invalid") {
      const invariantName = parseName(
        tag,
        getAttributes(child),
        child,
        addDiag,
      );
      if (!getJsxBodyFunction(child)) {
        addDiag(
          child,
          "TSPACK_TEST_MISSING_BODY",
          `${tag} requires callback body`,
        );
        continue;
      }
      const kind = tag === "Valid" ? "valid" : "invalid";
      const id = `${suiteName}/${kind}/${invariantName}`;
      collectCycleTimeForbiddenChildren(child, addDiag);
      invariants.push({
        kind,
        name: invariantName,
        id,
        project: collectProject(child, path.dirname(abs), addDiag),
      });
      tests.push(id);
    }
  }

  return {
    suiteName,
    tests,
    facts,
    theories,
    invariants,
    standaloneArtifacts,
    benchmarks,
    prophecies,
    diagnostics,
  };
}

const defaultIgnore = new Set([
  "node_modules",
  ".git",
  ".tspack",
  "dist",
  "fixtures",
  "tspack-artifacts",
]);
export function discoverNativeTestFiles(
  options: DiscoverOptions,
): DiscoverFilesResult {
  const rootDir = path.resolve(options.rootDir);
  const filePaths = collectNativeFiles(rootDir, options.ignore ?? []);
  const files: DiscoveredFile[] = [];
  const diagnostics: Diagnostic[] = [];

  for (const filePath of filePaths) {
    try {
      const discovered = discoverNativeTestFile(filePath);
      const relativeFilePath = normalizePublicTestPath(
        path.relative(rootDir, filePath),
      );
      const tests = [
        ...discovered.facts.map((fact) => ({
          id: `${relativeFilePath}::${fact.id}`,
          name: fact.name,
          kind: "fact" as const,
          filePath,
        })),
        ...discovered.theories.flatMap((theory) =>
          theory.cases.map((c) => ({
            id: `${relativeFilePath}::${c.id}`,
            name: theory.name,
            kind: "theory" as const,
            filePath,
          })),
        ),
        ...discovered.invariants.map((entry) => ({
          id: `${relativeFilePath}::${entry.id}`,
          name: entry.name,
          kind: entry.kind,
          filePath,
        })),
        ...discovered.tests
          .filter((id) => id.includes("/benchmark/"))
          .map((id) => ({
            id: `${relativeFilePath}::${id}`,
            name: id.split("/").at(-1) ?? id,
            kind: "fact" as const,
            filePath,
          })),
      ];
      const standaloneArtifacts = discovered.standaloneArtifacts.map(
        (entry) => ({
          ...entry,
          id: `${relativeFilePath}::${entry.id.split("::").pop() ?? entry.id}`,
          filePath: relativeFilePath,
        }),
      );
      const benchmarks = discovered.benchmarks.map((entry) => ({
        ...entry,
        id: `${relativeFilePath}::${entry.id.split("::").pop() ?? entry.id}`,
        filePath: relativeFilePath,
      }));
      const prophecies = discovered.prophecies.map((entry) => ({
        ...entry,
        id: `${relativeFilePath}::${entry.id.split("::").pop() ?? entry.id}`,
        filePath: relativeFilePath,
      }));
      files.push({
        filePath,
        suiteName: discovered.suiteName ?? "",
        tests,
        standaloneArtifacts,
        benchmarks,
        prophecies,
        diagnostics: discovered.diagnostics,
      });
      diagnostics.push(...discovered.diagnostics);
    } catch (error) {
      diagnostics.push({
        code: "TSPACK_TEST_DISCOVERY_FAILED",
        message: `failed to discover native test file: ${(error as Error).message}`,
        file: filePath,
      });
    }
  }

  files.sort((a, b) => a.filePath.localeCompare(b.filePath));
  diagnostics.sort((a, b) =>
    `${a.file}:${a.line ?? 0}:${a.column ?? 0}:${a.code}:${a.message}`.localeCompare(
      `${b.file}:${b.line ?? 0}:${b.column ?? 0}:${b.code}:${b.message}`,
    ),
  );
  return { rootDir, files, diagnostics };
}

function collectNativeFiles(rootDir: string, ignore: string[]): string[] {
  const entries = fs
    .readdirSync(rootDir, { withFileTypes: true })
    .sort((a, b) => a.name.localeCompare(b.name));
  const filePaths: string[] = [];
  const ignoreSet = new Set(ignore);
  for (const entry of entries) {
    const fullPath = path.join(rootDir, entry.name);
    if (entry.isDirectory()) {
      if (defaultIgnore.has(entry.name) || ignoreSet.has(entry.name)) continue;
      filePaths.push(...collectNativeFiles(fullPath, ignore));
      continue;
    }
    if (entry.isFile() && classifyNativeFile(entry.name)) {
      filePaths.push(fullPath);
    }
  }
  return filePaths;
}

const unwrap = (expr: ts.Expression): ts.Expression =>
  ts.isParenthesizedExpression(expr) ? unwrap(expr.expression) : expr;
const getTagName = (node: ts.JsxElement | ts.JsxSelfClosingElement): string =>
  ts.isJsxElement(node)
    ? node.openingElement.tagName.getText()
    : node.tagName.getText();
const getAttributes = (
  node: ts.JsxElement | ts.JsxSelfClosingElement,
): readonly ts.JsxAttributeLike[] =>
  ts.isJsxElement(node)
    ? node.openingElement.attributes.properties
    : node.attributes.properties;
const getChildren = (
  node: ts.JsxElement | ts.JsxSelfClosingElement,
): ts.Node[] => (ts.isJsxElement(node) ? [...node.children] : []);
function renderAllowedFileKind(tag: string): string {
  if (tag === "Valid") return "*.valid.tsx";
  if (tag === "Invalid") return "*.invalid.tsx";
  if (tag === "Benchmark") return "*.benchmark.tsx";
  if (tag === "Prophecy") return "*.prophecy.tsx";
  return "*.xtest.tsx";
}
function collectForetell(
  node: ts.JsxElement | ts.JsxSelfClosingElement,
  addDiag: (node: ts.Node, code: string, message: string) => void,
): { reason: string } | undefined {
  const matches = getChildren(node).filter(
    (child) =>
      (ts.isJsxElement(child) || ts.isJsxSelfClosingElement(child)) &&
      getTagName(child) === "Foretell",
  ) as Array<ts.JsxElement | ts.JsxSelfClosingElement>;
  if (matches.length === 0) {
    addDiag(
      node,
      "TSPACK_DOOM_MISSING_FORETELL",
      'Prophecy requires one <Foretell reason=\"...\" />',
    );
    return undefined;
  }
  if (matches.length > 1) {
    addDiag(
      matches[1],
      "TSPACK_DOOM_DUPLICATE_FORETELL",
      "Prophecy allows only one Foretell",
    );
    return undefined;
  }
  let reason: string | undefined;
  for (const attr of getAttributes(matches[0])) {
    if (ts.isJsxSpreadAttribute(attr)) {
      addDiag(
        attr,
        "TSPACK_TEST_FORBIDDEN_SPREAD",
        "spread attributes are forbidden",
      );
      return undefined;
    }
    if (!ts.isJsxAttribute(attr) || attr.name.getText() !== "reason") continue;
    if (
      !attr.initializer ||
      !ts.isStringLiteral(attr.initializer) ||
      attr.initializer.text.trim().length === 0
    ) {
      addDiag(
        attr,
        "TSPACK_DOOM_INVALID_FORETELL",
        "Foretell reason must be a non-empty string literal",
      );
      return undefined;
    }
    reason = attr.initializer.text;
  }
  if (!reason) {
    addDiag(
      matches[0],
      "TSPACK_DOOM_INVALID_FORETELL",
      "Foretell reason is required",
    );
    return undefined;
  }
  return { reason };
}
function parseName(
  tag: string,
  attrs: readonly ts.JsxAttributeLike[],
  node: ts.Node,
  addDiag: (node: ts.Node, code: string, message: string) => void,
): string {
  for (const attr of attrs) {
    if (ts.isJsxSpreadAttribute(attr)) {
      addDiag(
        attr,
        "TSPACK_TEST_FORBIDDEN_SPREAD",
        "spread attributes are forbidden",
      );
      continue;
    }
    if (!ts.isJsxAttribute(attr) || attr.name.getText() !== "name") {
      continue;
    }
    const init = attr.initializer;
    if (!init || !ts.isStringLiteral(init)) {
      addDiag(
        attr,
        "TSPACK_TEST_INVALID_NAME",
        `${tag} name must be string literal`,
      );
      return "";
    }
    return init.text;
  }
  addDiag(node, "TSPACK_TEST_INVALID_NAME", `${tag} name is required`);
  return "";
}
function literalFrom(expr: ts.Expression): Literal | undefined {
  if (ts.isStringLiteral(expr) || ts.isNoSubstitutionTemplateLiteral(expr))
    return expr.text;
  if (ts.isNumericLiteral(expr)) return Number(expr.text);
  if (expr.kind === ts.SyntaxKind.TrueKeyword) return true;
  if (expr.kind === ts.SyntaxKind.FalseKeyword) return false;
  if (expr.kind === ts.SyntaxKind.NullKeyword) return null;
  return undefined;
}
function collectCases(
  node: ts.JsxElement | ts.JsxSelfClosingElement,
  suite: string,
  theory: string,
  addDiag: (node: ts.Node, code: string, message: string) => void,
): Array<{ index: number; data: Record<string, Literal>; id: string }> {
  const out: Array<{
    index: number;
    data: Record<string, Literal>;
    id: string;
  }> = [];
  for (const child of getChildren(node)) {
    if (!ts.isJsxElement(child) && !ts.isJsxSelfClosingElement(child)) {
      continue;
    }
    if (getTagName(child) !== "Case") {
      continue;
    }

    const data: Record<string, Literal> = {};
    let valid = true;
    for (const attr of getAttributes(child)) {
      if (ts.isJsxSpreadAttribute(attr)) {
        addDiag(
          attr,
          "TSPACK_TEST_FORBIDDEN_SPREAD",
          "spread attributes are forbidden",
        );
        valid = false;
        continue;
      }
      if (!ts.isJsxAttribute(attr) || !attr.initializer) {
        addDiag(child, "TSPACK_TEST_INVALID_CASE", "invalid case attribute");
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
          addDiag(
            attr,
            "TSPACK_TEST_INVALID_CASE",
            "case props must be literal values",
          );
          valid = false;
        } else {
          data[attr.name.getText()] = value;
        }
        continue;
      }
      addDiag(attr, "TSPACK_TEST_INVALID_CASE", "case props must be literals");
      valid = false;
    }
    if (valid) {
      const index = out.length;
      out.push({ index, data, id: `${suite}/${theory}[${index}]` });
    }
  }
  return out;
}

function getJsxBodyFunction(
  node: ts.JsxElement | ts.JsxSelfClosingElement,
): boolean {
  return getDirectCallbackBodies(getChildren(node)).length > 0;
}

type TheoryStructureValidation = {
  hasBody: boolean;
  hasDuplicateBody: boolean;
  duplicateBodyNode?: ts.Node;
  isValid: boolean;
};

function validateTheoryStructure(
  node: ts.JsxElement | ts.JsxSelfClosingElement,
  addDiag: (node: ts.Node, code: string, message: string) => void,
): TheoryStructureValidation {
  const directCallbacks = getDirectCallbackBodies(getChildren(node));
  const nestedCallbackCount = countCallbackBodies(getChildren(node));
  let isValid = directCallbacks.length === 1 && nestedCallbackCount.count === 1;
  const allowedElementChildren = new Set([
    "Case",
    "Artifact",
    "Project",
    "CycleTime",
  ]);

  for (const child of getChildren(node)) {
    if (ts.isJsxText(child)) {
      if (child.getText().trim().length > 0) {
        addDiag(
          child,
          "TSPACK_TEST_INVALID_THEORY_STRUCTURE",
          "Theory text children are not allowed",
        );
        isValid = false;
      }
      continue;
    }
    if (ts.isJsxExpression(child)) {
      if (!isIgnorableTheoryExpression(child)) {
        if (!isDirectCallbackBody(child.expression)) {
          addDiag(
            child,
            "TSPACK_TEST_INVALID_THEORY_STRUCTURE",
            "Theory expression children must be the callback body",
          );
          isValid = false;
        }
      }
      continue;
    }
    if (ts.isJsxElement(child) || ts.isJsxSelfClosingElement(child)) {
      if (!allowedElementChildren.has(getTagName(child))) {
        addDiag(
          child,
          "TSPACK_TEST_INVALID_THEORY_STRUCTURE",
          "Theory children must be Case, Artifact, Project, CycleTime, or callback body",
        );
        isValid = false;
      }
      continue;
    }
  }

  return {
    hasBody: directCallbacks.length > 0,
    hasDuplicateBody:
      nestedCallbackCount.count > 1 || directCallbacks.length > 1,
    duplicateBodyNode:
      nestedCallbackCount.duplicateNode ?? directCallbacks[1] ?? undefined,
    isValid,
  };
}

function isIgnorableTheoryExpression(node: ts.JsxExpression): boolean {
  return !node.expression;
}

function getDirectCallbackBodies(
  nodes: readonly ts.Node[],
): ts.JsxExpression[] {
  const callbacks: ts.JsxExpression[] = [];
  for (const node of nodes) {
    if (!ts.isJsxExpression(node) || !node.expression) {
      continue;
    }
    if (isDirectCallbackBody(node.expression)) {
      callbacks.push(node);
    }
  }
  return callbacks;
}

function isDirectCallbackBody(expression: ts.Expression | undefined): boolean {
  if (!expression) {
    return false;
  }
  const unwrapped = unwrap(expression);
  return ts.isArrowFunction(unwrapped) || ts.isFunctionExpression(unwrapped);
}

function countCallbackBodies(nodes: readonly ts.Node[]): {
  count: number;
  duplicateNode?: ts.Node;
} {
  let count = 0;
  let duplicateNode: ts.Node | undefined;
  for (const node of nodes) {
    if (!ts.isJsxExpression(node) || !node.expression) {
      continue;
    }
    const expressionCount = countCallbackBodiesInExpression(node.expression);
    if (expressionCount.count === 0) {
      continue;
    }
    if (count + expressionCount.count > 1 && !duplicateNode) {
      duplicateNode = expressionCount.duplicateNode ?? node;
    }
    count += expressionCount.count;
  }
  return { count, duplicateNode };
}

function countCallbackBodiesInExpression(expression: ts.Expression): {
  count: number;
  duplicateNode?: ts.Node;
} {
  const unwrapped = unwrap(expression);
  if (ts.isArrowFunction(unwrapped) || ts.isFunctionExpression(unwrapped)) {
    return { count: 1 };
  }
  if (ts.isArrayLiteralExpression(unwrapped)) {
    let count = 0;
    let duplicateNode: ts.Node | undefined;
    for (const element of unwrapped.elements) {
      const elementCount = countCallbackBodiesInExpression(
        element as ts.Expression,
      );
      if (elementCount.count === 0) {
        continue;
      }
      if (count + elementCount.count > 1 && !duplicateNode) {
        duplicateNode = elementCount.duplicateNode ?? element;
      }
      count += elementCount.count;
    }
    return { count, duplicateNode };
  }
  return { count: 0 };
}

function collectArtifacts(
  node: ts.JsxElement | ts.JsxSelfClosingElement,
  addDiag: (node: ts.Node, code: string, message: string) => void,
) {
  const artifacts: Array<{
    name: string;
    path: string;
    format?: string;
    required: boolean;
  }> = [];
  const seen = new Set<string>();
  for (const child of getChildren(node)) {
    if (!ts.isJsxElement(child) && !ts.isJsxSelfClosingElement(child)) continue;
    if (getTagName(child) !== "Artifact") continue;
    const parsed = parseArtifact(child, addDiag);
    if (!parsed) continue;
    if (seen.has(parsed.name)) {
      addDiag(
        child,
        "TSPACK_ARTIFACT_DUPLICATE_NAME",
        `duplicate Artifact name: ${parsed.name}`,
      );
      continue;
    }
    seen.add(parsed.name);
    artifacts.push(parsed);
  }
  return artifacts;
}
function parseArtifact(
  node: ts.JsxElement | ts.JsxSelfClosingElement,
  addDiag: (node: ts.Node, code: string, message: string) => void,
) {
  let name: string | undefined;
  let declaredPath: string | undefined;
  let format: string | undefined;
  let optional = false;
  for (const attr of getAttributes(node)) {
    if (ts.isJsxSpreadAttribute(attr)) {
      addDiag(
        attr,
        "TSPACK_ARTIFACT_INVALID_DECLARATION",
        "Artifact does not allow spread attributes",
      );
      return undefined;
    }
    if (!ts.isJsxAttribute(attr) || !attr.initializer) continue;
    const attrName = attr.name.getText();
    if (attrName === "name" || attrName === "path" || attrName === "format") {
      if (!ts.isStringLiteral(attr.initializer)) {
        addDiag(
          attr,
          "TSPACK_ARTIFACT_INVALID_DECLARATION",
          `Artifact ${attrName} must be string literal`,
        );
        return undefined;
      }
      if (attrName === "name") name = attr.initializer.text;
      if (attrName === "path") declaredPath = attr.initializer.text;
      if (attrName === "format") format = attr.initializer.text;
      continue;
    }
    if (attrName === "optional") {
      if (
        !ts.isJsxExpression(attr.initializer) ||
        !attr.initializer.expression
      ) {
        addDiag(
          attr,
          "TSPACK_ARTIFACT_INVALID_DECLARATION",
          "Artifact optional must be boolean literal",
        );
        return undefined;
      }
      if (attr.initializer.expression.kind === ts.SyntaxKind.TrueKeyword)
        optional = true;
      else if (attr.initializer.expression.kind === ts.SyntaxKind.FalseKeyword)
        optional = false;
      else {
        addDiag(
          attr,
          "TSPACK_ARTIFACT_INVALID_DECLARATION",
          "Artifact optional must be boolean literal",
        );
        return undefined;
      }
    }
  }
  if (!name) {
    addDiag(
      node,
      "TSPACK_ARTIFACT_INVALID_DECLARATION",
      "Artifact name is required",
    );
    return undefined;
  }
  if (!declaredPath) {
    addDiag(
      node,
      "TSPACK_ARTIFACT_INVALID_DECLARATION",
      "Artifact path is required",
    );
    return undefined;
  }
  if (
    declaredPath.startsWith("/") ||
    declaredPath.includes("..") ||
    declaredPath.includes("\\")
  ) {
    addDiag(
      node,
      "TSPACK_ARTIFACT_INVALID_PATH",
      `Artifact path is unsafe: ${declaredPath}`,
    );
    return undefined;
  }
  return { name, path: declaredPath, format, required: !optional };
}

function parseSuiteArtifact(
  node: ts.JsxElement | ts.JsxSelfClosingElement,
  suiteName: string,
  filePath: string,
  fileDir: string,
  addDiag: (node: ts.Node, code: string, message: string) => void,
): DiscoveredStandaloneArtifact | undefined {
  const parsed = parseArtifact(node, addDiag);
  if (!parsed) return undefined;
  if (!parsed.required) {
    addDiag(
      node,
      "TSPACK_ARTIFACT_OPTIONAL_NOT_ALLOWED",
      "suite-level Artifact does not allow optional",
    );
    return undefined;
  }
  if (!getJsxBodyFunction(node)) {
    addDiag(
      node,
      "TSPACK_ARTIFACT_MISSING_BODY",
      "suite-level Artifact requires callback body",
    );
    return undefined;
  }
  const id = `${filePath.split(path.sep).join("/")}::${suiteName}/artifact/${parsed.name}`;
  return {
    id,
    filePath,
    suiteName,
    name: parsed.name,
    path: parsed.path,
    format: parsed.format,
    project: collectProject(node, fileDir, addDiag),
    cycleTimeSeconds: collectCycleTimeAllowed(node, addDiag),
  };
}

function collectProject(
  node: ts.JsxElement | ts.JsxSelfClosingElement,
  fileDir: string,
  addDiag: (node: ts.Node, code: string, message: string) => void,
) {
  const projects = getChildren(node).filter(
    (child) =>
      (ts.isJsxElement(child) || ts.isJsxSelfClosingElement(child)) &&
      getTagName(child) === "Project",
  ) as Array<ts.JsxElement | ts.JsxSelfClosingElement>;
  if (projects.length === 0) return undefined;
  if (projects.length > 1) {
    addDiag(
      projects[1],
      "TSPACK_PROJECT_FIXTURE_DUPLICATE",
      "only one Project is allowed per executable unit",
    );
    return undefined;
  }
  const project = projects[0];
  let from: string | undefined;
  let name: string | undefined;
  let keepOnFailure = false;
  for (const attr of getAttributes(project)) {
    if (ts.isJsxSpreadAttribute(attr)) {
      addDiag(
        attr,
        "TSPACK_PROJECT_FIXTURE_INVALID_DECLARATION",
        "Project does not allow spread attributes",
      );
      return undefined;
    }
    if (!ts.isJsxAttribute(attr) || !attr.initializer) continue;
    const n = attr.name.getText();
    if (n === "from" || n === "name") {
      if (!ts.isStringLiteral(attr.initializer)) {
        addDiag(
          attr,
          "TSPACK_PROJECT_FIXTURE_INVALID_DECLARATION",
          `Project ${n} must be string literal`,
        );
        return undefined;
      }
      if (n === "from") from = attr.initializer.text;
      else name = attr.initializer.text;
      continue;
    }
    if (n === "keepOnFailure") {
      if (
        !ts.isJsxExpression(attr.initializer) ||
        !attr.initializer.expression
      ) {
        addDiag(
          attr,
          "TSPACK_PROJECT_FIXTURE_INVALID_DECLARATION",
          "Project keepOnFailure must be boolean literal",
        );
        return undefined;
      }
      if (attr.initializer.expression.kind === ts.SyntaxKind.TrueKeyword)
        keepOnFailure = true;
      else if (attr.initializer.expression.kind === ts.SyntaxKind.FalseKeyword)
        keepOnFailure = false;
      else {
        addDiag(
          attr,
          "TSPACK_PROJECT_FIXTURE_INVALID_DECLARATION",
          "Project keepOnFailure must be boolean literal",
        );
        return undefined;
      }
    }
  }
  if (from) {
    if (from.startsWith("/") || from.includes("..") || from.includes("\\")) {
      addDiag(
        project,
        "TSPACK_PROJECT_FIXTURE_INVALID_PATH",
        `Project from path is unsafe: ${from}`,
      );
      return undefined;
    }
    const sourcePath = path.resolve(fileDir, from);
    if (!fs.existsSync(sourcePath) || !fs.statSync(sourcePath).isDirectory()) {
      addDiag(
        project,
        "TSPACK_PROJECT_FIXTURE_NOT_FOUND",
        `Project fixture not found: ${from}`,
      );
      return undefined;
    }
  }
  return { from, name, keepOnFailure };
}

function collectCycleTimeForbiddenChildren(
  node: ts.JsxElement | ts.JsxSelfClosingElement,
  addDiag: (node: ts.Node, code: string, message: string) => void,
): void {
  for (const child of getChildren(node)) {
    if (
      (ts.isJsxElement(child) || ts.isJsxSelfClosingElement(child)) &&
      getTagName(child) === "CycleTime"
    ) {
      addDiag(
        child,
        "TSPACK_TEST_CYCLETIME_NOT_ALLOWED",
        "<CycleTime> is not allowed in this location",
      );
    }
  }
}

function collectCycleTimeAllowed(
  node: ts.JsxElement | ts.JsxSelfClosingElement,
  addDiag: (node: ts.Node, code: string, message: string) => void,
): number | undefined {
  const cycleNodes = getChildren(node).filter(
    (child) =>
      (ts.isJsxElement(child) || ts.isJsxSelfClosingElement(child)) &&
      getTagName(child) === "CycleTime",
  ) as Array<ts.JsxElement | ts.JsxSelfClosingElement>;
  if (cycleNodes.length === 0) return undefined;
  if (cycleNodes.length > 1)
    addDiag(
      cycleNodes[1],
      "TSPACK_TEST_DUPLICATE_CYCLETIME",
      "only one CycleTime is allowed per parent",
    );
  const cycle = cycleNodes[0];
  let seconds: number | undefined;
  for (const attr of getAttributes(cycle)) {
    if (ts.isJsxSpreadAttribute(attr)) {
      addDiag(
        attr,
        "TSPACK_TEST_INVALID_CYCLETIME",
        "CycleTime does not allow spread attributes",
      );
      return undefined;
    }
    if (!ts.isJsxAttribute(attr) || attr.name.getText() !== "seconds") continue;
    if (
      !attr.initializer ||
      !ts.isJsxExpression(attr.initializer) ||
      !attr.initializer.expression ||
      !ts.isNumericLiteral(attr.initializer.expression)
    ) {
      addDiag(
        attr,
        "TSPACK_TEST_INVALID_CYCLETIME",
        "CycleTime seconds must be a number literal",
      );
      return undefined;
    }
    seconds = Number(attr.initializer.expression.text);
  }
  if (seconds === undefined || !Number.isFinite(seconds) || seconds <= 0) {
    addDiag(
      cycle,
      "TSPACK_TEST_INVALID_CYCLETIME",
      "CycleTime seconds must be a positive finite number literal",
    );
    return undefined;
  }
  return seconds;
}

function collectBenchmarkCount(
  node: ts.JsxElement | ts.JsxSelfClosingElement,
  tag: "Iterations" | "Warmup",
  fallback: number,
  addDiag: (node: ts.Node, code: string, message: string) => void,
): number {
  const nodes = getChildren(node).filter(
    (child) =>
      (ts.isJsxElement(child) || ts.isJsxSelfClosingElement(child)) &&
      getTagName(child) === tag,
  ) as Array<ts.JsxElement | ts.JsxSelfClosingElement>;
  if (nodes.length === 0) {
    return fallback;
  }
  if (nodes.length > 1) {
    addDiag(
      nodes[1],
      tag === "Iterations"
        ? "TSPACK_BENCHMARK_DUPLICATE_ITERATIONS"
        : "TSPACK_BENCHMARK_DUPLICATE_WARMUP",
      `only one ${tag} is allowed per Benchmark`,
    );
  }
  const target = nodes[0];
  for (const attr of getAttributes(target)) {
    if (ts.isJsxSpreadAttribute(attr)) {
      addDiag(
        attr,
        tag === "Iterations"
          ? "TSPACK_BENCHMARK_INVALID_ITERATIONS"
          : "TSPACK_BENCHMARK_INVALID_WARMUP",
        `${tag} does not allow spread attributes`,
      );
      return fallback;
    }
    if (!ts.isJsxAttribute(attr) || attr.name.getText() !== "count") {
      continue;
    }
    if (
      !attr.initializer ||
      !ts.isJsxExpression(attr.initializer) ||
      !attr.initializer.expression ||
      !ts.isNumericLiteral(attr.initializer.expression)
    ) {
      addDiag(
        attr,
        tag === "Iterations"
          ? "TSPACK_BENCHMARK_INVALID_ITERATIONS"
          : "TSPACK_BENCHMARK_INVALID_WARMUP",
        `${tag} count must be a positive integer literal`,
      );
      return fallback;
    }
    const parsed = Number(attr.initializer.expression.text);
    if (!Number.isInteger(parsed) || parsed <= 0) {
      addDiag(
        attr,
        tag === "Iterations"
          ? "TSPACK_BENCHMARK_INVALID_ITERATIONS"
          : "TSPACK_BENCHMARK_INVALID_WARMUP",
        `${tag} count must be a positive integer literal`,
      );
      return fallback;
    }
    return parsed;
  }
  addDiag(
    target,
    tag === "Iterations"
      ? "TSPACK_BENCHMARK_INVALID_ITERATIONS"
      : "TSPACK_BENCHMARK_INVALID_WARMUP",
    `${tag} count is required`,
  );
  return fallback;
}

function normalizePublicTestPath(filePath: string): string {
  let normalized = filePath.replace(/\\/g, "/").split(path.sep).join("/");
  while (normalized.startsWith("./")) {
    normalized = normalized.slice(2);
  }
  return normalized;
}
