import crypto from 'node:crypto';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { performance } from 'node:perf_hooks';
import { clearPendingExpectations, verifyNoPendingExpectations } from './expect.js';
import { isSkipSignal } from './skip.js';
import type { DiscoveredArtifact, TestArtifact, TestResult } from './types.js';

type RuntimeNode = {
  __tag: 'Suite' | 'Fact' | 'Theory' | 'Case' | 'Artifact' | 'Valid' | 'Invalid';
  props: Record<string, unknown>;
  children: unknown[];
};

type RunSuiteOptions = { artifactRoot?: string };
type TestContext = { artifact: ArtifactWriter };

type ArtifactWriter = {
  writeText: (name: string, text: string, reason: string) => Promise<void>;
  writeJson: (name: string, value: unknown, reason: string) => Promise<void>;
  writeBytes: (name: string, bytes: Uint8Array | Buffer, reason: string) => Promise<void>;
};

export async function runSuite(root: RuntimeNode, options: RunSuiteOptions = {}): Promise<TestResult[]> {
  if (!root || root.__tag !== 'Suite') {
    throw new Error('suite root must be Suite node');
  }

  const artifactRoot = options.artifactRoot ?? await fs.mkdtemp(path.join(os.tmpdir(), 'tspack-artifacts-'));
  const suiteName = String(root.props.name ?? '');
  const results: TestResult[] = [];

  for (const child of root.children) {
    if (!isNode(child)) continue;
    if (child.__tag === 'Fact') {
      const factName = String(child.props.name ?? '');
      const id = `${suiteName}/${factName}`;
      const callback = child.children.find((entry) => typeof entry === 'function') as ((ctx?: TestContext) => unknown) | undefined;
      const declarations = collectDeclarations(child);
      results.push(await runSingle(id, factName, declarations, artifactRoot, async (ctx) => callback?.(ctx)));
      continue;
    }

    if (child.__tag === 'Valid' || child.__tag === 'Invalid') {
      const invariantName = String(child.props.name ?? '');
      const prefix = child.__tag === 'Valid' ? 'valid' : 'invalid';
      const id = `${suiteName}/${prefix}/${invariantName}`;
      const callback = child.children.find((entry) => typeof entry === 'function') as ((ctx?: TestContext) => unknown) | undefined;
      results.push(await runSingle(id, invariantName, [], artifactRoot, async (ctx) => callback?.(ctx)));
      continue;
    }

    if (child.__tag === 'Theory') {
      const theoryName = String(child.props.name ?? '');
      const callback = child.children.find((entry) => typeof entry === 'function') as ((data: Record<string, unknown>, ctx?: TestContext) => unknown) | undefined;
      const cases = child.children.filter((entry) => isNode(entry) && entry.__tag === 'Case') as RuntimeNode[];
      const declarations = collectDeclarations(child);
      for (let i = 0; i < cases.length; i += 1) {
        const id = `${suiteName}/${theoryName}[${i}]`;
        const caseData = { ...cases[i].props };
        results.push(await runSingle(id, theoryName, declarations, artifactRoot, async (ctx) => callback?.(caseData, ctx)));
      }
    }
  }

  return results;
}

async function runSingle(id: string, name: string, declarations: DiscoveredArtifact[], artifactRoot: string, fn: (ctx: TestContext) => unknown): Promise<TestResult> {
  const started = performance.now();
  const state = createArtifactState(id, declarations, artifactRoot);
  const ctx: TestContext = { artifact: state.writer };
  try {
    await fn(ctx);
    verifyNoPendingExpectations();
    for (const entry of state.results) {
      if (entry.required && !entry.written) {
        const error = new Error(`required artifact not written: ${entry.name}`) as Error & { code: string };
        error.code = 'TSPACK_ARTIFACT_REQUIRED_NOT_WRITTEN';
        throw error;
      }
    }
    return { id, name, status: 'passed', durationMs: performance.now() - started, artifacts: state.results };
  } catch (error) {
    if (isSkipSignal(error)) {
      clearPendingExpectations();
      return { id, name, status: 'skipped', durationMs: performance.now() - started, skipReason: error.skipReason, artifacts: state.results };
    }
    return { id, name, status: 'failed', durationMs: performance.now() - started, error: error as Error, artifacts: state.results };
  }
}

function createArtifactState(id: string, declarations: DiscoveredArtifact[], artifactRoot: string) {
  const baseDir = path.join(artifactRoot, sanitizeId(id));
  const results: TestArtifact[] = declarations.map((item) => ({
    name: item.name,
    declaredPath: item.path,
    outputPath: path.join(baseDir, item.path),
    format: item.format,
    required: item.required,
    written: false,
  }));

  const writeCommon = async (name: string, reason: string, data: Buffer): Promise<void> => {
    if (!reason || !reason.trim()) {
      const error = new Error('artifact reason is required') as Error & { code: string };
      error.code = 'TSPACK_ARTIFACT_REASON_REQUIRED';
      throw error;
    }
    const artifact = results.find((entry) => entry.name === name);
    if (!artifact) {
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
      await fs.mkdir(path.dirname(artifact.outputPath), { recursive: true });
      await fs.writeFile(artifact.outputPath, data);
      artifact.written = true;
      artifact.size = data.byteLength;
      artifact.reason = reason;
      artifact.hash = `sha256:${crypto.createHash('sha256').update(data).digest('hex')}`;
    } catch (cause) {
      const error = new Error(`artifact write failed: ${(cause as Error).message}`) as Error & { code: string };
      error.code = 'TSPACK_ARTIFACT_WRITE_FAILED';
      throw error;
    }
  };

  const writer: ArtifactWriter = {
    writeText: async (name, text, reason) => writeCommon(name, reason, Buffer.from(text, 'utf8')),
    writeJson: async (name, value, reason) => writeCommon(name, reason, Buffer.from(`${stableStringify(value, 0)}\n`, 'utf8')),
    writeBytes: async (name, bytes, reason) => writeCommon(name, reason, Buffer.from(bytes)),
  };

  return { writer, results };
}

function stableStringify(value: unknown, indent: number): string {
  if (value === null || typeof value !== 'object') {
    return JSON.stringify(value);
  }
  if (Array.isArray(value)) {
    const items = value.map((entry) => stableStringify(entry, indent + 2));
    return `[${items.join(',')}]`;
  }
  const obj = value as Record<string, unknown>;
  const keys = Object.keys(obj).sort();
  const inner = keys.map((key) => `${JSON.stringify(key)}: ${stableStringify(obj[key], indent + 2)}`).join(', ');
  return `{${inner}}`;
}

function collectDeclarations(node: RuntimeNode): DiscoveredArtifact[] {
  const declarations: DiscoveredArtifact[] = [];
  for (const child of node.children) {
    if (!isNode(child) || child.__tag !== 'Artifact') {
      continue;
    }
    declarations.push({
      name: String(child.props.name ?? ''),
      path: String(child.props.path ?? ''),
      format: typeof child.props.format === 'string' ? child.props.format : undefined,
      required: child.props.optional === true ? false : true,
    });
  }
  return declarations;
}

function sanitizeId(value: string): string {
  return value.replace(/[^a-zA-Z0-9._-]+/g, '__').replace(/^_+|_+$/g, '').toLowerCase();
}

function isNode(value: unknown): value is RuntimeNode {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const candidate = value as RuntimeNode;
  return typeof candidate.__tag === 'string' && !!candidate.props && Array.isArray(candidate.children);
}
