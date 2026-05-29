import crypto from "node:crypto";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { performance } from "node:perf_hooks";
import { markArtifactWriteActivity, setActivityTracker } from "./activity.js";
import {
  clearPendingExpectations,
  verifyNoPendingExpectations,
} from "./expect.js";
import { setSnapshotContext } from "./snapshot.js";
import { isSkipSignal } from "./skip.js";
import type {
  DiscoveredArtifact,
  DiscoveredProjectFixture,
  ProjectResultInfo,
  TestArtifact,
  TestResult,
  TestSnapshotUpdate,
} from "./types.js";
import { createCommandContext, type CommandContext } from "./command.js";

type RuntimeNode = {
  __tag:
    | "Suite"
    | "Fact"
    | "Theory"
    | "Case"
    | "Artifact"
    | "Valid"
    | "Invalid"
    | "Project"
    | "CycleTime";
  props: Record<string, unknown>;
  children: unknown[];
};

type RunSuiteOptions = {
  artifactRoot?: string;
  defaultTimeoutSeconds?: number;
  snapshotFilePath?: string;
  snapshotRootDir?: string;
  updateSnapshots?: boolean;
  shouldRunTest?: (localId: string, name: string) => boolean;
};
type TestContext = {
  artifact: ArtifactWriter;
  project?: ProjectContext;
  command?: CommandContext;
};

type ArtifactWriter = {
  writeText: (name: string, text: string, reason: string) => Promise<void>;
  writeJson: (name: string, value: unknown, reason: string) => Promise<void>;
  writeBytes: (
    name: string,
    bytes: Uint8Array | Buffer,
    reason: string,
  ) => Promise<void>;
};

type ProjectContext = {
  rootPath: string;
  sourcePath?: string;
  name?: string;
  path: (relativePath: string) => string;
  readText: (relativePath: string) => Promise<string>;
  readJson: <T = unknown>(relativePath: string) => Promise<T>;
  writeText: (
    relativePath: string,
    text: string,
    reason: string,
  ) => Promise<void>;
  writeJson: (
    relativePath: string,
    value: unknown,
    reason: string,
  ) => Promise<void>;
  writeBytes: (
    relativePath: string,
    bytes: Uint8Array | Buffer,
    reason: string,
  ) => Promise<void>;
};

const SKIP_FIXTURE_DIRS = new Set([
  "node_modules",
  ".git",
  ".tspack",
  "tspack-artifacts",
  "dist-packages",
]);

export async function runSuite(
  root: RuntimeNode,
  options: RunSuiteOptions = {},
): Promise<TestResult[]> {
  if (!root || root.__tag !== "Suite")
    throw new Error("suite root must be Suite node");
  const artifactRoot =
    options.artifactRoot ??
    (await fs.mkdtemp(path.join(os.tmpdir(), "tspack-artifacts-")));
  const suiteName = String(root.props.name ?? "");
  const results: TestResult[] = [];
  for (const child of root.children) {
    if (!isNode(child)) continue;
    if (child.__tag === "Fact") {
      const declarations = collectDeclarations(child);
      const project = collectProject(child);
      const factName = String(child.props.name ?? "");
      const id = `${suiteName}/${factName}`;
      const cb = child.children.find((e) => typeof e === "function") as
        | ((ctx?: TestContext) => unknown)
        | undefined;
      if (!shouldRun(options, id, factName)) {
        continue;
      }
      results.push(
        await runSingle(
          id,
          factName,
          declarations,
          project,
          artifactRoot,
          options.defaultTimeoutSeconds ?? 30,
          snapshotOptions(options),
          async (ctx) => cb?.(ctx),
        ),
      );
    }
    if (child.__tag === "Theory") {
      const declarations = collectDeclarations(child);
      const project = collectProject(child);
      const theoryName = String(child.props.name ?? "");
      const callbacks = child.children.filter(
        (entry) => typeof entry === "function",
      ) as Array<(data: Record<string, unknown>, ctx?: TestContext) => unknown>;
      const cycleTimeSeconds =
        readCycleTimeSeconds(child) ?? options.defaultTimeoutSeconds ?? 30;
      const cases = child.children.filter(
        (entry) => isNode(entry) && entry.__tag === "Case",
      ) as RuntimeNode[];
      const structureError = getTheoryStructureError(
        callbacks.length,
        cases.length,
      );
      if (structureError) {
        results.push(
          makeStructureFailure(
            `${suiteName}/${theoryName}`,
            theoryName,
            structureError.code,
            structureError.message,
          ),
        );
        continue;
      }
      const cb = callbacks[0];
      for (let i = 0; i < cases.length; i += 1) {
        const id = `${suiteName}/${theoryName}[${i}]`;
        if (!shouldRun(options, id, theoryName)) {
          continue;
        }
        results.push(
          await runSingle(
            id,
            theoryName,
            declarations,
            project,
            artifactRoot,
            cycleTimeSeconds,
            snapshotOptions(options),
            async (ctx) => cb({ ...cases[i].props }, ctx),
          ),
        );
      }
    }
    if (child.__tag === "Valid" || child.__tag === "Invalid") {
      const project = collectProject(child);
      const invariantName = String(child.props.name ?? "");
      const kind = child.__tag === "Valid" ? "valid" : "invalid";
      const id = `${suiteName}/${kind}/${invariantName}`;
      const cb = child.children.find((e) => typeof e === "function") as
        | ((ctx?: TestContext) => unknown)
        | undefined;
      if (!shouldRun(options, id, invariantName)) {
        continue;
      }
      results.push(
        await runSingle(
          id,
          invariantName,
          [],
          project,
          artifactRoot,
          options.defaultTimeoutSeconds ?? 30,
          snapshotOptions(options),
          async (ctx) => cb?.(ctx),
        ),
      );
    }
  }
  return results;
}


function shouldRun(options: RunSuiteOptions, localId: string, name: string): boolean {
  if (!options.shouldRunTest) {
    return true;
  }
  return options.shouldRunTest(localId, name);
}

function getTheoryStructureError(
  callbackCount: number,
  caseCount: number,
): { code: string; message: string } | undefined {
  if (callbackCount === 0) {
    return {
      code: "TSPACK_TEST_THEORY_MISSING_BODY",
      message: "Theory requires exactly one callback body",
    };
  }
  if (callbackCount > 1) {
    return {
      code: "TSPACK_TEST_THEORY_DUPLICATE_BODY",
      message: "Theory allows only one callback body",
    };
  }
  if (caseCount === 0) {
    return {
      code: "TSPACK_TEST_THEORY_NO_CASES",
      message: "Theory requires at least one Case child",
    };
  }
  return undefined;
}

function makeStructureFailure(
  id: string,
  name: string,
  code: string,
  message: string,
): TestResult {
  const error = new Error(message) as Error & { code: string };
  error.code = code;
  return { id, name, status: "failed", durationMs: 0, error, artifacts: [] };
}


type SingleSnapshotOptions = {
  testFilePath?: string;
  rootDir?: string;
  updateSnapshots: boolean;
};

function snapshotOptions(options: RunSuiteOptions): SingleSnapshotOptions {
  return {
    testFilePath: options.snapshotFilePath,
    rootDir: options.snapshotRootDir,
    updateSnapshots: options.updateSnapshots === true,
  };
}

function createSnapshotContext(
  options: SingleSnapshotOptions,
  updates: TestSnapshotUpdate[],
) {
  if (!options.testFilePath || !options.rootDir) {
    return undefined;
  }
  return {
    testFilePath: options.testFilePath,
    rootDir: options.rootDir,
    updateSnapshots: options.updateSnapshots,
    updates,
  };
}

async function runSingle(
  id: string,
  name: string,
  declarations: DiscoveredArtifact[],
  projectDecl: DiscoveredProjectFixture | undefined,
  artifactRoot: string,
  timeoutSeconds: number,
  snapshotOptions: SingleSnapshotOptions,
  fn: (ctx: TestContext) => unknown,
): Promise<TestResult> {
  const started = performance.now();
  const state = createArtifactState(id, declarations, artifactRoot);
  const snapshotUpdates: TestSnapshotUpdate[] = [];
  let projectResult: ProjectResultInfo | undefined;
  let sandboxRoot: string | undefined;
  const activity = {
    assertCount: 0,
    expectCount: 0,
    skipCount: 0,
    artifactWriteCount: 0,
  };
  setActivityTracker({
    markAssert: () => {
      activity.assertCount += 1;
    },
    markExpect: () => {
      activity.expectCount += 1;
    },
    markSkip: () => {
      activity.skipCount += 1;
    },
    markArtifactWrite: () => {
      activity.artifactWriteCount += 1;
    },
  });
  try {
    setSnapshotContext(createSnapshotContext(snapshotOptions, snapshotUpdates));
    const projectContext = projectDecl
      ? await createProjectContext(id, projectDecl)
      : undefined;
    sandboxRoot = projectContext?.rootPath;
    const command = projectContext
      ? createCommandContext({
          projectRoot: projectContext.rootPath,
          evidenceDir: path.join(state.baseDir, "commands"),
          tspackPath: process.env.TSPACK_TEST_TSPACK_PATH,
        })
      : undefined;
    await withTimeout(
      fn({ artifact: state.writer, project: projectContext, command }),
      timeoutSeconds,
    );
    verifyNoPendingExpectations();
    for (const entry of state.results)
      if (entry.required && !entry.written)
        throw withCode(
          "TSPACK_ARTIFACT_REQUIRED_NOT_WRITTEN",
          `required artifact not written: ${entry.name}`,
        );
    if (
      activity.assertCount + activity.expectCount + activity.skipCount ===
      0
    ) {
      throw withCode(
        "TSPACK_TEST_NO_ASSERTION",
        "test completed without meaningful action",
      );
    }
    if (projectDecl && sandboxRoot) {
      await fs.rm(sandboxRoot, { recursive: true, force: true });
      projectResult = {
        sourcePath: projectDecl.from,
        name: projectDecl.name,
        kept: false,
      };
    }
    return {
      id,
      name,
      status: "passed",
      durationMs: performance.now() - started,
      artifacts: state.results,
      project: projectResult,
      snapshots: snapshotUpdates,
    };
  } catch (error) {
    if (isSkipSignal(error)) {
      clearPendingExpectations();
      if (sandboxRoot)
        await fs.rm(sandboxRoot, { recursive: true, force: true });
      return {
        id,
        name,
        status: "skipped",
        durationMs: performance.now() - started,
        skipReason: error.skipReason,
        artifacts: state.results,
        snapshots: snapshotUpdates,
        project: projectDecl
          ? {
              sourcePath: projectDecl.from,
              name: projectDecl.name,
              kept: false,
            }
          : undefined,
      };
    }
    if (projectDecl && sandboxRoot) {
      const keep = projectDecl.keepOnFailure;
      if (!keep) await fs.rm(sandboxRoot, { recursive: true, force: true });
      projectResult = {
        sourcePath: projectDecl.from,
        name: projectDecl.name,
        kept: keep,
        rootPath: keep ? sandboxRoot : undefined,
      };
    }
    return {
      id,
      name,
      status: "failed",
      durationMs: performance.now() - started,
      error: error as Error,
      artifacts: state.results,
      project: projectResult,
      snapshots: snapshotUpdates,
    };
  } finally {
    setActivityTracker(undefined);
    setSnapshotContext(undefined);
  }
}
function readCycleTimeSeconds(node: RuntimeNode): number | undefined {
  const cycleTime = node.children.find(
    (entry) => isNode(entry) && entry.__tag === "CycleTime",
  ) as RuntimeNode | undefined;
  if (!cycleTime) return undefined;
  const raw = cycleTime.props.seconds;
  if (typeof raw === "number" && Number.isFinite(raw) && raw > 0) return raw;
  return undefined;
}
async function withTimeout<T>(
  value: Promise<T> | T,
  timeoutSeconds: number,
): Promise<T> {
  const timeoutMs = Math.max(1, timeoutSeconds * 1000);
  let handle: NodeJS.Timeout | undefined;
  try {
    return await Promise.race([
      Promise.resolve(value),
      new Promise<T>((_, reject) => {
        handle = setTimeout(
          () =>
            reject(
              withCode(
                "TSPACK_TEST_TIMEOUT",
                `test timed out after ${timeoutSeconds} seconds`,
              ),
            ),
          timeoutMs,
        );
      }),
    ]);
  } finally {
    if (handle) clearTimeout(handle);
  }
}

async function createProjectContext(
  id: string,
  projectDecl: DiscoveredProjectFixture,
): Promise<ProjectContext> {
  const rootPath = await fs.mkdtemp(
    path.join(os.tmpdir(), `tspack-project-${sanitizeId(id)}-`),
  );
  if (projectDecl.from)
    await copyFixtureDirectory(path.resolve(projectDecl.from), rootPath);
  const resolveSafe = (rel: string): string => {
    if (
      !rel ||
      !rel.trim() ||
      rel.startsWith("/") ||
      rel.includes("..") ||
      rel.includes("\\")
    )
      throw withCode(
        "TSPACK_PROJECT_INVALID_PATH",
        `invalid project path: ${rel}`,
      );
    const resolved = path.resolve(rootPath, rel);
    if (!resolved.startsWith(`${rootPath}${path.sep}`) && resolved !== rootPath)
      throw withCode(
        "TSPACK_PROJECT_PATH_ESCAPE",
        `project path escapes sandbox: ${rel}`,
      );
    return resolved;
  };
  const requireReason = (reason: string): void => {
    if (!reason || !reason.trim())
      throw withCode(
        "TSPACK_PROJECT_WRITE_REASON_REQUIRED",
        "project write reason is required",
      );
  };
  return {
    rootPath,
    sourcePath: projectDecl.from,
    name: projectDecl.name,
    path: (relativePath) => resolveSafe(relativePath),
    readText: async (relativePath) => {
      try {
        return await fs.readFile(resolveSafe(relativePath), "utf8");
      } catch (e) {
        throw withCode("TSPACK_PROJECT_READ_FAILED", (e as Error).message);
      }
    },
    readJson: async (relativePath) =>
      JSON.parse(await fs.readFile(resolveSafe(relativePath), "utf8")),
    writeText: async (relativePath, text, reason) => {
      requireReason(reason);
      await writeProjectFile(
        resolveSafe(relativePath),
        Buffer.from(text, "utf8"),
      );
    },
    writeJson: async (relativePath, value, reason) => {
      requireReason(reason);
      await writeProjectFile(
        resolveSafe(relativePath),
        Buffer.from(`${JSON.stringify(value, null, 2)}\n`, "utf8"),
      );
    },
    writeBytes: async (relativePath, bytes, reason) => {
      requireReason(reason);
      await writeProjectFile(resolveSafe(relativePath), Buffer.from(bytes));
    },
  };
}

async function writeProjectFile(
  targetPath: string,
  data: Buffer,
): Promise<void> {
  try {
    await fs.mkdir(path.dirname(targetPath), { recursive: true });
    await fs.writeFile(targetPath, data);
  } catch (e) {
    throw withCode("TSPACK_PROJECT_WRITE_FAILED", (e as Error).message);
  }
}
async function copyFixtureDirectory(
  sourcePath: string,
  targetPath: string,
): Promise<void> {
  for (const entry of (
    await fs.readdir(sourcePath, { withFileTypes: true })
  ).sort((a, b) => a.name.localeCompare(b.name))) {
    if (SKIP_FIXTURE_DIRS.has(entry.name)) continue;
    const src = path.join(sourcePath, entry.name);
    const dest = path.join(targetPath, entry.name);
    if (entry.isSymbolicLink())
      throw withCode(
        "TSPACK_PROJECT_SYMLINK_UNSUPPORTED",
        `symlink unsupported: ${src}`,
      );
    if (entry.isDirectory()) {
      await fs.mkdir(dest, { recursive: true });
      await copyFixtureDirectory(src, dest);
      continue;
    }
    if (entry.isFile()) {
      await fs.mkdir(path.dirname(dest), { recursive: true });
      await fs.copyFile(src, dest);
    }
  }
}
function withCode(code: string, message: string): Error & { code: string } {
  const e = new Error(message) as Error & { code: string };
  e.code = code;
  return e;
}
function createArtifactState(
  id: string,
  declarations: DiscoveredArtifact[],
  artifactRoot: string,
) {
  const baseDir = path.join(artifactRoot, sanitizeId(id));
  const results: TestArtifact[] = declarations.map((item) => ({
    name: item.name,
    declaredPath: item.path,
    outputPath: path.join(baseDir, item.path),
    format: item.format,
    required: item.required,
    written: false,
  }));
  const writeCommon = async (
    name: string,
    reason: string,
    data: Buffer,
  ): Promise<void> => {
    if (!reason || !reason.trim())
      throw withCode(
        "TSPACK_ARTIFACT_REASON_REQUIRED",
        "artifact reason is required",
      );
    const artifact = results.find((entry) => entry.name === name);
    if (!artifact)
      throw withCode("TSPACK_ARTIFACT_UNKNOWN", `unknown artifact: ${name}`);
    if (artifact.written)
      throw withCode(
        "TSPACK_ARTIFACT_ALREADY_WRITTEN",
        `artifact already written: ${name}`,
      );
    try {
      await fs.mkdir(path.dirname(artifact.outputPath), { recursive: true });
      await fs.writeFile(artifact.outputPath, data);
      markArtifactWriteActivity();
      artifact.written = true;
      artifact.size = data.byteLength;
      artifact.reason = reason;
      artifact.hash = `sha256:${crypto.createHash("sha256").update(data).digest("hex")}`;
    } catch (cause) {
      throw withCode(
        "TSPACK_ARTIFACT_WRITE_FAILED",
        `artifact write failed: ${(cause as Error).message}`,
      );
    }
  };
  return {
    baseDir,
    writer: {
      writeText: async (name, text, reason) =>
        writeCommon(name, reason, Buffer.from(text, "utf8")),
      writeJson: async (name, value, reason) =>
        writeCommon(
          name,
          reason,
          Buffer.from(`${JSON.stringify(value, null, 2)}\n`, "utf8"),
        ),
      writeBytes: async (name, bytes, reason) =>
        writeCommon(name, reason, Buffer.from(bytes)),
    },
    results,
  };
}
function collectDeclarations(node: RuntimeNode): DiscoveredArtifact[] {
  return node.children
    .filter((c) => isNode(c) && c.__tag === "Artifact")
    .map((child) => ({
      name: String((child as RuntimeNode).props.name ?? ""),
      path: String((child as RuntimeNode).props.path ?? ""),
      format:
        typeof (child as RuntimeNode).props.format === "string"
          ? ((child as RuntimeNode).props.format as string)
          : undefined,
      required: (child as RuntimeNode).props.optional !== true,
    }));
}
function collectProject(
  node: RuntimeNode,
): DiscoveredProjectFixture | undefined {
  const child = node.children.find(
    (c) => isNode(c) && c.__tag === "Project",
  ) as RuntimeNode | undefined;
  if (!child) return undefined;
  return {
    from: typeof child.props.from === "string" ? child.props.from : undefined,
    name: typeof child.props.name === "string" ? child.props.name : undefined,
    keepOnFailure: child.props.keepOnFailure === true,
  };
}
function sanitizeId(value: string): string {
  return value
    .replace(/[^a-zA-Z0-9._-]+/g, "__")
    .replace(/^_+|_+$/g, "")
    .toLowerCase();
}
function isNode(value: unknown): value is RuntimeNode {
  if (!value || typeof value !== "object") return false;
  const candidate = value as RuntimeNode;
  return (
    typeof candidate.__tag === "string" &&
    !!candidate.props &&
    Array.isArray(candidate.children)
  );
}
