import crypto from "node:crypto";
import fs from "node:fs";
import fsp from "node:fs/promises";
import path from "node:path";
import os from "node:os";
import { performance } from "node:perf_hooks";
import { pathToFileURL } from "node:url";
import { discoverNativeTestFiles } from "./discover.js";
import {
  expect,
  clearPendingExpectations,
  verifyNoPendingExpectations,
} from "./expect.js";
import { assert } from "./assert.js";
import { isSkipSignal, skip } from "./skip.js";
import { runSuite } from "./runner.js";
import { markArtifactWriteActivity, setActivityTracker } from "./activity.js";
import { loadRuntimeSuiteForFile } from "./runtime-load.js";
import { createCommandContext } from "./command.js";
import type {
  ArtifactRunResult,
  Diagnostic,
  DiscoveredFile,
  RunArtifactsOptions,
  RunFilesOptions,
  RunFilesResult,
  StandaloneArtifactResult,
  TestArtifact,
  TestResult,
} from "./types.js";

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

export async function runNativeTestFiles(
  options: RunFilesOptions,
): Promise<RunFilesResult> {
  const discovered = discoverNativeTestFiles({ rootDir: options.rootDir });
  const diagnostics: Diagnostic[] = [...discovered.diagnostics];
  const results: TestResult[] = [];
  const rootDir = path.resolve(options.rootDir);
  const selectedFiles = filterByFileSelection(
    discovered.files,
    rootDir,
    options.files,
  );
  const runnableFiles = filterByTestSelection(
    selectedFiles,
    options.filter,
    diagnostics,
    rootDir,
  );

  if (options.listOnly) {
    for (const file of runnableFiles) {
      for (const test of file.tests) {
        if (matchesFilter(test.id, test.name, options.filter)) {
          results.push({ id: test.id, name: test.name, status: "passed" });
        }
      }
    }
    return { results, diagnostics };
  }

  if (options.batch) {
    return runNativeTestFilesInBatch(
      runnableFiles,
      diagnostics,
      rootDir,
      options,
    );
  }

  for (const file of runnableFiles) {
    const fileResult = await runNativeTestFile(file, rootDir, options);
    results.push(...fileResult.results);
    diagnostics.push(...fileResult.diagnostics);
  }

  return { results, diagnostics };
}

async function runNativeTestFilesInBatch(
  files: DiscoveredFile[],
  diagnostics: Diagnostic[],
  rootDir: string,
  options: RunFilesOptions,
): Promise<RunFilesResult> {
  const resultsByFile: TestResult[][] = files.map(() => []);
  const diagnosticsByFile: Diagnostic[][] = files.map(() => []);
  const workerCount = chooseBatchWorkerCount(files.length);
  let nextFileIndex = 0;

  async function runWorker(): Promise<void> {
    while (nextFileIndex < files.length) {
      const fileIndex = nextFileIndex;
      nextFileIndex += 1;

      const fileResult = await runNativeTestFile(
        files[fileIndex],
        rootDir,
        options,
      );
      resultsByFile[fileIndex] = fileResult.results;
      diagnosticsByFile[fileIndex] = fileResult.diagnostics;
    }
  }

  const workers = Array.from({ length: workerCount }, () => runWorker());
  await Promise.all(workers);

  return {
    results: resultsByFile.flat(),
    diagnostics: [...diagnostics, ...diagnosticsByFile.flat()],
  };
}

async function runNativeTestFile(
  file: DiscoveredFile,
  rootDir: string,
  options: RunFilesOptions,
): Promise<RunFilesResult> {
  const results: TestResult[] = [];
  const diagnostics: Diagnostic[] = [];

  try {
    const root = await loadRuntimeSuiteForFile(file.filePath, { rootDir });
    const artifactRoot =
      options.artifactRoot ?? path.join(rootDir, ".tspack", "test-artifacts");
    const filePrefix = normalizePublicTestPath(
      path.relative(rootDir, file.filePath),
    );
    const runResults = await runSuite(root, {
      artifactRoot,
      defaultTimeoutSeconds: options.defaultTimeoutSeconds,
      snapshotFilePath: file.filePath,
      snapshotRootDir: rootDir,
      updateSnapshots: options.updateSnapshots === true,
      shouldRunTest: (localId, name) =>
        matchesFilter(`${filePrefix}::${localId}`, name, options.filter),
    });

    for (const result of runResults) {
      const fullId = `${filePrefix}::${result.id}`;
      if (matchesFilter(fullId, result.name, options.filter)) {
        results.push({ ...result, id: fullId });
      }
    }
  } catch (error) {
    diagnostics.push({
      code: "TSPACK_TEST_MODULE_LOAD_FAILED",
      message: `failed to load module ${file.filePath}: ${(error as Error).message}`,
      file: file.filePath,
      severity: "error",
    });
  }

  return { results, diagnostics };
}

export function chooseBatchWorkerCount(fileCount: number): number {
  if (fileCount <= 0) {
    return 0;
  }

  const override = parseInternalBatchWorkerOverride();
  const availableParallelism = override ?? readAvailableParallelism();
  const cappedParallelism = Math.min(availableParallelism, 8);
  return Math.max(1, Math.min(cappedParallelism, fileCount));
}

function parseInternalBatchWorkerOverride(): number | undefined {
  const raw = process.env.TSPACK_TEST_BATCH_WORKERS;
  if (!raw) {
    return undefined;
  }

  const parsed = Number.parseInt(raw, 10);
  if (!Number.isFinite(parsed) || parsed < 1) {
    return undefined;
  }

  return parsed;
}

function readAvailableParallelism(): number {
  const availableParallelism = os.availableParallelism?.();
  if (availableParallelism && availableParallelism > 0) {
    return availableParallelism;
  }

  return Math.max(1, os.cpus().length);
}

export async function runNativeArtifacts(
  options: RunArtifactsOptions,
): Promise<ArtifactRunResult> {
  const discovered = discoverNativeTestFiles({ rootDir: options.rootDir });
  const diagnostics: Diagnostic[] = [...discovered.diagnostics];
  const rootDir = path.resolve(options.rootDir);
  const selectedFiles = filterByFileSelection(
    discovered.files,
    rootDir,
    options.files,
  );
  const selectedArtifacts = selectArtifacts(
    selectedFiles,
    options.filter,
    diagnostics,
    rootDir,
  );

  if (options.listOnly) {
    return {
      artifacts: selectedArtifacts.map((a) => ({
        id: a.id,
        name: a.name,
        status: "passed",
      })),
      diagnostics,
    };
  }

  const artifactRoot = path.resolve(
    options.artifactRoot ?? path.join(rootDir, ".tspack", "artifacts"),
  );
  const artifacts: StandaloneArtifactResult[] = [];

  for (const file of selectedFiles) {
    const fileIdPrefix = normalizePublicTestPath(
      path.relative(rootDir, file.filePath),
    );
    const fileArtifacts = selectedArtifacts.filter(
      (entry) => entry.filePath === fileIdPrefix,
    );
    if (fileArtifacts.length === 0) {
      continue;
    }
    try {
      const root = await loadRuntimeSuiteForFile(file.filePath, { rootDir });
      for (const declared of fileArtifacts) {
        artifacts.push(
          await runStandaloneArtifact(
            root,
            declared.id,
            declared.name,
            declared.path,
            declared.format,
            artifactRoot,
            options.defaultTimeoutSeconds ?? 30,
          ),
        );
      }
    } catch (error) {
      diagnostics.push({
        code: "TSPACK_ARTIFACT_FAILED",
        message: `failed to load module ${file.filePath}: ${(error as Error).message}`,
        file: file.filePath,
        severity: "error",
      });
    }
  }

  artifacts.sort((a, b) => a.id.localeCompare(b.id));
  return { artifacts, diagnostics };
}

async function runStandaloneArtifact(
  root: RuntimeNode,
  id: string,
  name: string,
  declaredPath: string,
  format: string | undefined,
  artifactRoot: string,
  defaultTimeoutSeconds: number,
): Promise<StandaloneArtifactResult> {
  const started = performance.now();
  const suiteChildren = root.children.filter(
    (entry) => isNode(entry) && entry.__tag === "Artifact",
  ) as RuntimeNode[];
  const node = suiteChildren.find(
    (entry) =>
      String(entry.props.name ?? "") === name &&
      String(entry.props.path ?? "") === declaredPath,
  );
  if (!node) {
    return {
      id,
      name,
      status: "failed",
      failure: {
        code: "TSPACK_ARTIFACT_UNKNOWN",
        message: `standalone artifact not found: ${name}`,
      },
      durationMs: performance.now() - started,
    };
  }

  const callback = node.children.find(
    (entry) => typeof entry === "function",
  ) as
    | ((ctx: { artifact: any; project?: any; command?: any }) => unknown)
    | undefined;
  const artifact = createSingleArtifactState(
    id,
    name,
    declaredPath,
    format,
    artifactRoot,
  );
  const projectNode = node.children.find(
    (entry) => isNode(entry) && entry.__tag === "Project",
  ) as RuntimeNode | undefined;
  const project = projectNode
    ? await createStandaloneProjectContext(projectNode)
    : undefined;
  const command = project
    ? createCommandContext({
        projectRoot: project.rootPath,
        evidenceDir: path.join(
          path.dirname(artifact.result.outputPath),
          "commands",
        ),
        tspackPath: process.env.TSPACK_TEST_TSPACK_PATH,
      })
    : undefined;
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
    const cycle = node.children.find(
      (entry) => isNode(entry) && entry.__tag === "CycleTime",
    ) as RuntimeNode | undefined;
    const timeoutSeconds =
      typeof cycle?.props.seconds === "number"
        ? cycle.props.seconds
        : defaultTimeoutSeconds;
    await Promise.race([
      Promise.resolve(
        callback?.({ artifact: artifact.writer, project, command }),
      ),
      new Promise((_, reject) =>
        setTimeout(
          () =>
            reject(
              Object.assign(
                new Error(`test timed out after ${timeoutSeconds} seconds`),
                { code: "TSPACK_TEST_TIMEOUT" },
              ),
            ),
          Math.max(1, timeoutSeconds * 1000),
        ),
      ),
    ]);
    verifyNoPendingExpectations();
    if (!artifact.result.written) {
      return {
        id,
        name,
        status: "failed",
        failure: {
          code: "TSPACK_ARTIFACT_REQUIRED_NOT_WRITTEN",
          message: `required artifact not written: ${name}`,
        },
        artifact: artifact.result,
        durationMs: performance.now() - started,
      };
    }
    if (
      activity.assertCount +
        activity.expectCount +
        activity.skipCount +
        activity.artifactWriteCount ===
      0
    ) {
      return {
        id,
        name,
        status: "failed",
        failure: {
          code: "TSPACK_TEST_NO_ASSERTION",
          message: "artifact completed without meaningful action",
        },
        artifact: artifact.result,
        durationMs: performance.now() - started,
      };
    }
    return {
      id,
      name,
      status: "passed",
      artifact: artifact.result,
      durationMs: performance.now() - started,
    };
  } catch (error) {
    if (isSkipSignal(error)) {
      clearPendingExpectations();
      return {
        id,
        name,
        status: "skipped",
        skipReason: error.skipReason,
        artifact: artifact.result,
        durationMs: performance.now() - started,
      };
    }
    const e = error as Error & { code?: string; reason?: string };
    return {
      id,
      name,
      status: "failed",
      failure: { code: e.code, message: e.message, reason: e.reason },
      artifact: artifact.result,
      durationMs: performance.now() - started,
    };
  } finally {
    setActivityTracker(undefined);
    if (project?.rootPath) {
      await fsp.rm(project.rootPath, { recursive: true, force: true });
    }
  }
}

async function createStandaloneProjectContext(
  node: RuntimeNode,
): Promise<{
  rootPath: string;
  readJson: (p: string) => Promise<any>;
  writeText: (p: string, t: string, r: string) => Promise<void>;
}> {
  const rootPath = await fsp.mkdtemp(
    path.join(os.tmpdir(), "tspack-standalone-project-"),
  );
  const from =
    typeof node.props.from === "string" ? node.props.from : undefined;
  if (from) {
    await fsp.cp(path.resolve(from), rootPath, { recursive: true });
  }
  return {
    rootPath,
    readJson: async (p) =>
      JSON.parse(await fsp.readFile(path.join(rootPath, p), "utf8")),
    writeText: async (p, t, r) => {
      if (!r || !r.trim()) throw new Error("reason required");
      const out = path.join(rootPath, p);
      await fsp.mkdir(path.dirname(out), { recursive: true });
      await fsp.writeFile(out, t);
    },
  };
}

function createSingleArtifactState(
  id: string,
  name: string,
  declaredPath: string,
  format: string | undefined,
  artifactRoot: string,
) {
  const outputPath = path.join(artifactRoot, sanitizeId(id), declaredPath);
  const result: TestArtifact = {
    name,
    declaredPath,
    outputPath,
    format,
    required: true,
    written: false,
  };
  const writer = {
    writeText: async (artifactName: string, text: string, reason: string) =>
      writeCommon(result, artifactName, reason, Buffer.from(text, "utf8")),
    writeJson: async (artifactName: string, value: unknown, reason: string) =>
      writeCommon(
        result,
        artifactName,
        reason,
        Buffer.from(`${JSON.stringify(value, null, 2)}\n`, "utf8"),
      ),
    writeBytes: async (
      artifactName: string,
      bytes: Uint8Array | Buffer,
      reason: string,
    ) => writeCommon(result, artifactName, reason, Buffer.from(bytes)),
  };
  return { writer, result };
}

async function writeCommon(
  artifact: TestArtifact,
  name: string,
  reason: string,
  data: Buffer,
): Promise<void> {
  if (!reason || !reason.trim()) {
    const error = new Error("artifact reason is required") as Error & {
      code: string;
    };
    error.code = "TSPACK_ARTIFACT_REASON_REQUIRED";
    throw error;
  }
  if (artifact.name !== name) {
    const error = new Error(`unknown artifact: ${name}`) as Error & {
      code: string;
    };
    error.code = "TSPACK_ARTIFACT_UNKNOWN";
    throw error;
  }
  if (artifact.written) {
    const error = new Error(`artifact already written: ${name}`) as Error & {
      code: string;
    };
    error.code = "TSPACK_ARTIFACT_ALREADY_WRITTEN";
    throw error;
  }
  try {
    await fsp.mkdir(path.dirname(artifact.outputPath), { recursive: true });
    await fsp.writeFile(artifact.outputPath, data);
    markArtifactWriteActivity();
    artifact.written = true;
    artifact.size = data.byteLength;
    artifact.reason = reason;
    artifact.hash = `sha256:${crypto.createHash("sha256").update(data).digest("hex")}`;
  } catch (cause) {
    const error = new Error(
      `artifact write failed: ${(cause as Error).message}`,
    ) as Error & { code: string };
    error.code = "TSPACK_ARTIFACT_WRITE_FAILED";
    throw error;
  }
}

function selectArtifacts(
  files: DiscoveredFile[],
  filter: string | undefined,
  diagnostics: Diagnostic[],
  rootDir: string,
) {
  const out = files.flatMap((file) => file.standaloneArtifacts);
  const selected = out.filter((entry) =>
    matchesFilter(entry.id, entry.name, filter),
  );
  if (filter && selected.length === 0) {
    diagnostics.push({
      code: "TSPACK_ARTIFACT_FILTER_NO_MATCH",
      message: `standalone artifact filter matched no artifacts: ${filter}`,
      file: path.resolve(rootDir),
      severity: "error",
    });
  }
  return selected.sort((a, b) => a.id.localeCompare(b.id));
}

function filterByFileSelection(
  files: DiscoveredFile[],
  rootDir: string,
  requested?: string[],
): DiscoveredFile[] {
  return files.filter((file) => {
    if (!requested || requested.length === 0) {
      return true;
    }
    return requested.some(
      (candidate) =>
        path.resolve(rootDir, candidate) === file.filePath ||
        candidate === file.filePath,
    );
  });
}

function filterByTestSelection(
  files: DiscoveredFile[],
  filter: string | undefined,
  diagnostics: Diagnostic[],
  rootDir: string,
): DiscoveredFile[] {
  if (!filter) return files;
  const matchedFiles = files.filter((file) =>
    file.tests.some((test) => matchesFilter(test.id, test.name, filter)),
  );
  if (matchedFiles.length === 0) {
    diagnostics.push({
      code: "TSPACK_TEST_FILTER_NO_MATCH",
      message: `native test filter matched no tests: ${filter}`,
      file: path.resolve(rootDir),
      severity: "error",
    });
  }
  return matchedFiles;
}

function matchesFilter(
  id: string,
  name: string,
  filter: string | undefined,
): boolean {
  if (!filter) return true;
  return id.includes(filter) || name.includes(filter);
}

function sanitizeId(value: string): string {
  return value
    .replace(/[^a-zA-Z0-9._-]+/g, "__")
    .replace(/^_+|_+$/g, "")
    .toLowerCase();
}

function isNode(value: unknown): value is RuntimeNode {
  return (
    !!value &&
    typeof value === "object" &&
    typeof (value as RuntimeNode).__tag === "string" &&
    Array.isArray((value as RuntimeNode).children)
  );
}

function normalizePublicTestPath(filePath: string): string {
  let normalized = filePath.replace(/\\/g, "/").split(path.sep).join("/");
  while (normalized.startsWith("./")) {
    normalized = normalized.slice(2);
  }
  return normalized;
}
