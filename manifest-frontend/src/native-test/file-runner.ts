import crypto from 'node:crypto';
import fs from 'node:fs';
import fsp from 'node:fs/promises';
import path from 'node:path';
import { performance } from 'node:perf_hooks';
import { pathToFileURL } from 'node:url';
import ts from 'typescript';
import { discoverNativeTestFiles } from './discover.js';
import { expect, clearPendingExpectations, verifyNoPendingExpectations } from './expect.js';
import { assert } from './assert.js';
import { isSkipSignal, skip } from './skip.js';
import { runSuite } from './runner.js';
import type { ArtifactRunResult, Diagnostic, DiscoveredFile, RunArtifactsOptions, RunFilesOptions, RunFilesResult, StandaloneArtifactResult, TestArtifact, TestResult } from './types.js';

type RuntimeNode = {
  __tag: 'Suite' | 'Fact' | 'Theory' | 'Case' | 'Artifact';
  props: Record<string, unknown>;
  children: unknown[];
};

export async function runNativeTestFiles(options: RunFilesOptions): Promise<RunFilesResult> {
  const discovered = discoverNativeTestFiles({ rootDir: options.rootDir });
  const diagnostics: Diagnostic[] = [...discovered.diagnostics];
  const results: TestResult[] = [];
  const selectedFiles = filterByFileSelection(discovered.files, options.rootDir, options.files);
  const runnableFiles = filterByTestSelection(selectedFiles, options.filter, diagnostics, options.rootDir);

  if (options.listOnly) {
    for (const file of runnableFiles) {
      for (const test of file.tests) {
        if (matchesFilter(test.id, test.name, options.filter)) {
          results.push({ id: test.id, name: test.name, status: 'passed' });
        }
      }
    }
    return { results, diagnostics };
  }

  for (const file of runnableFiles) {
    try {
      const root = await loadRuntimeSuite(file.filePath);
      const artifactRoot = options.artifactRoot ?? path.join(options.rootDir, '.tspack', 'test-artifacts');
      const runResults = await runSuite(root, { artifactRoot });
      for (const result of runResults) {
        const fullId = `${path.relative(options.rootDir, file.filePath).split(path.sep).join('/')}::${result.id}`;
        if (matchesFilter(fullId, result.name, options.filter)) {
          results.push({ ...result, id: fullId });
        }
      }
    } catch (error) {
      diagnostics.push({ code: 'TSPACK_TEST_MODULE_LOAD_FAILED', message: `failed to load module ${file.filePath}: ${(error as Error).message}`, file: file.filePath, severity: 'error' });
    }
  }

  return { results, diagnostics };
}

export async function runNativeArtifacts(options: RunArtifactsOptions): Promise<ArtifactRunResult> {
  const discovered = discoverNativeTestFiles({ rootDir: options.rootDir });
  const diagnostics: Diagnostic[] = [...discovered.diagnostics];
  const selectedFiles = filterByFileSelection(discovered.files, options.rootDir, options.files);
  const selectedArtifacts = selectArtifacts(selectedFiles, options.filter, diagnostics, options.rootDir);

  if (options.listOnly) {
    return { artifacts: selectedArtifacts.map((a) => ({ id: a.id, name: a.name, status: 'passed' })), diagnostics };
  }

  const artifactRoot = path.resolve(options.artifactRoot ?? path.join(options.rootDir, '.tspack', 'artifacts'));
  const artifacts: StandaloneArtifactResult[] = [];

  for (const file of selectedFiles) {
    const fileArtifacts = selectedArtifacts.filter((entry) => entry.filePath === file.filePath);
    if (fileArtifacts.length === 0) {
      continue;
    }
    try {
      const root = await loadRuntimeSuite(file.filePath);
      for (const declared of fileArtifacts) {
        artifacts.push(await runStandaloneArtifact(root, declared.id, declared.name, declared.path, declared.format, artifactRoot));
      }
    } catch (error) {
      diagnostics.push({ code: 'TSPACK_ARTIFACT_FAILED', message: `failed to load module ${file.filePath}: ${(error as Error).message}`, file: file.filePath, severity: 'error' });
    }
  }

  artifacts.sort((a, b) => a.id.localeCompare(b.id));
  return { artifacts, diagnostics };
}

async function runStandaloneArtifact(root: RuntimeNode, id: string, name: string, declaredPath: string, format: string | undefined, artifactRoot: string): Promise<StandaloneArtifactResult> {
  const started = performance.now();
  const suiteChildren = root.children.filter((entry) => isNode(entry) && entry.__tag === 'Artifact') as RuntimeNode[];
  const node = suiteChildren.find((entry) => String(entry.props.name ?? '') === name && String(entry.props.path ?? '') === declaredPath);
  if (!node) {
    return { id, name, status: 'failed', failure: { code: 'TSPACK_ARTIFACT_UNKNOWN', message: `standalone artifact not found: ${name}` }, durationMs: performance.now() - started };
  }

  const callback = node.children.find((entry) => typeof entry === 'function') as ((ctx: { artifact: any }) => unknown) | undefined;
  const artifact = createSingleArtifactState(id, name, declaredPath, format, artifactRoot);
  try {
    await callback?.({ artifact: artifact.writer });
    verifyNoPendingExpectations();
    if (!artifact.result.written) {
      return { id, name, status: 'failed', failure: { code: 'TSPACK_ARTIFACT_REQUIRED_NOT_WRITTEN', message: `required artifact not written: ${name}` }, artifact: artifact.result, durationMs: performance.now() - started };
    }
    return { id, name, status: 'passed', artifact: artifact.result, durationMs: performance.now() - started };
  } catch (error) {
    if (isSkipSignal(error)) {
      clearPendingExpectations();
      return { id, name, status: 'skipped', skipReason: error.skipReason, artifact: artifact.result, durationMs: performance.now() - started };
    }
    const e = error as Error & { code?: string; reason?: string };
    return { id, name, status: 'failed', failure: { code: e.code, message: e.message, reason: e.reason }, artifact: artifact.result, durationMs: performance.now() - started };
  }
}

function createSingleArtifactState(id: string, name: string, declaredPath: string, format: string | undefined, artifactRoot: string) {
  const outputPath = path.join(artifactRoot, sanitizeId(id), declaredPath);
  const result: TestArtifact = { name, declaredPath, outputPath, format, required: true, written: false };
  const writer = {
    writeText: async (artifactName: string, text: string, reason: string) => writeCommon(result, artifactName, reason, Buffer.from(text, 'utf8')),
    writeJson: async (artifactName: string, value: unknown, reason: string) => writeCommon(result, artifactName, reason, Buffer.from(`${JSON.stringify(value, null, 2)}\n`, 'utf8')),
    writeBytes: async (artifactName: string, bytes: Uint8Array | Buffer, reason: string) => writeCommon(result, artifactName, reason, Buffer.from(bytes)),
  };
  return { writer, result };
}

async function writeCommon(artifact: TestArtifact, name: string, reason: string, data: Buffer): Promise<void> {
  if (!reason || !reason.trim()) {
    const error = new Error('artifact reason is required') as Error & { code: string };
    error.code = 'TSPACK_ARTIFACT_REASON_REQUIRED';
    throw error;
  }
  if (artifact.name !== name) {
    const error = new Error(`unknown artifact: ${name}`) as Error & { code: string };
    error.code = 'TSPACK_ARTIFACT_UNKNOWN';
    throw error;
  }
  if (artifact.written) {
    const error = new Error(`artifact already written: ${name}`) as Error & { code: string };
    error.code = 'TSPACK_ARTIFACT_ALREADY_WRITTEN';
    throw error;
  }
  try {
    await fsp.mkdir(path.dirname(artifact.outputPath), { recursive: true });
    await fsp.writeFile(artifact.outputPath, data);
    artifact.written = true;
    artifact.size = data.byteLength;
    artifact.reason = reason;
    artifact.hash = `sha256:${crypto.createHash('sha256').update(data).digest('hex')}`;
  } catch (cause) {
    const error = new Error(`artifact write failed: ${(cause as Error).message}`) as Error & { code: string };
    error.code = 'TSPACK_ARTIFACT_WRITE_FAILED';
    throw error;
  }
}

function selectArtifacts(files: DiscoveredFile[], filter: string | undefined, diagnostics: Diagnostic[], rootDir: string) {
  const out = files.flatMap((file) => file.standaloneArtifacts);
  const selected = out.filter((entry) => matchesFilter(entry.id, entry.name, filter));
  if (filter && selected.length === 0) {
    diagnostics.push({ code: 'TSPACK_ARTIFACT_FILTER_NO_MATCH', message: `standalone artifact filter matched no artifacts: ${filter}`, file: path.resolve(rootDir), severity: 'error' });
  }
  return selected.sort((a, b) => a.id.localeCompare(b.id));
}

function filterByFileSelection(files: DiscoveredFile[], rootDir: string, requested?: string[]): DiscoveredFile[] {
  return files.filter((file) => {
    if (!requested || requested.length === 0) {
      return true;
    }
    return requested.some((candidate) => path.resolve(rootDir, candidate) === file.filePath || candidate === file.filePath);
  });
}

function filterByTestSelection(files: DiscoveredFile[], filter: string | undefined, diagnostics: Diagnostic[], rootDir: string): DiscoveredFile[] {
  if (!filter) return files;
  const matchedFiles = files.filter((file) => file.tests.some((test) => matchesFilter(test.id, test.name, filter)));
  if (matchedFiles.length === 0) {
    diagnostics.push({ code: 'TSPACK_TEST_FILTER_NO_MATCH', message: `native test filter matched no tests: ${filter}`, file: path.resolve(rootDir), severity: 'error' });
  }
  return matchedFiles;
}

function matchesFilter(id: string, name: string, filter: string | undefined): boolean {
  if (!filter) return true;
  return id.includes(filter) || name.includes(filter);
}

function sanitizeId(value: string): string {
  return value.replace(/[^a-zA-Z0-9._-]+/g, '__').replace(/^_+|_+$/g, '').toLowerCase();
}

async function loadRuntimeSuite(filePath: string): Promise<RuntimeNode> {
  const source = fs.readFileSync(filePath, 'utf8');
  const compiled = ts.transpileModule(source, { fileName: filePath, compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext, jsx: ts.JsxEmit.React, jsxFactory: '__tspackJsx' } });
  const prelude = `const __tspackJsx = (type, props, ...children) => { if (typeof type === 'function') return type(props ?? {}, ...children); return { __tag: String(type), props: props ?? {}, children }; };
const makeTag = (tag) => (props, ...children) => ({ __tag: tag, props: props ?? {}, children });
const Suite = makeTag('Suite'); const Fact = makeTag('Fact'); const Theory = makeTag('Theory'); const Case = makeTag('Case'); const Artifact = makeTag('Artifact');
const assert = globalThis.__tspackAssert; const expect = globalThis.__tspackExpect; const skip = globalThis.__tspackSkip;`;
  const tempFile = path.join(path.dirname(filePath), `${path.basename(filePath)}.tspack-temp.mjs`);
  (globalThis as Record<string, unknown>).__tspackAssert = assert;
  (globalThis as Record<string, unknown>).__tspackExpect = expect;
  (globalThis as Record<string, unknown>).__tspackSkip = skip;
  fs.writeFileSync(tempFile, `${prelude}\n${compiled.outputText}`);
  try {
    const mod = await import(pathToFileURL(tempFile).href);
    const root = mod.default as RuntimeNode;
    if (!root || root.__tag !== 'Suite') {
      throw new Error('default export must evaluate to Suite runtime node');
    }
    return root;
  } finally {
    fs.rmSync(tempFile, { force: true });
  }
}

function isNode(value: unknown): value is RuntimeNode {
  return !!value && typeof value === 'object' && typeof (value as RuntimeNode).__tag === 'string' && Array.isArray((value as RuntimeNode).children);
}
