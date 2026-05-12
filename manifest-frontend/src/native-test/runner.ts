import { performance } from 'node:perf_hooks';
import { clearPendingExpectations, verifyNoPendingExpectations } from './expect.js';
import { isSkipSignal } from './skip.js';
import type { TestResult } from './types.js';

type RuntimeNode = {
  __tag: 'Suite' | 'Fact' | 'Theory' | 'Case';
  props: Record<string, unknown>;
  children: unknown[];
};

export async function runSuite(root: RuntimeNode): Promise<TestResult[]> {
  if (!root || root.__tag !== 'Suite') {
    throw new Error('suite root must be Suite node');
  }

  const suiteName = String(root.props.name ?? '');
  const results: TestResult[] = [];

  for (const child of root.children) {
    if (!isNode(child)) {
      continue;
    }
    if (child.__tag === 'Fact') {
      const factName = String(child.props.name ?? '');
      const id = `${suiteName}/${factName}`;
      const callback = child.children.find((entry) => typeof entry === 'function') as (() => unknown) | undefined;
      results.push(await runSingle(id, factName, async () => callback?.()));
    }
    if (child.__tag === 'Theory') {
      const theoryName = String(child.props.name ?? '');
      const callback = child.children.find((entry) => typeof entry === 'function') as ((data: Record<string, unknown>) => unknown) | undefined;
      const cases = child.children.filter((entry) => isNode(entry) && entry.__tag === 'Case') as RuntimeNode[];
      for (let i = 0; i < cases.length; i += 1) {
        const id = `${suiteName}/${theoryName}[${i}]`;
        const caseData = { ...cases[i].props };
        results.push(await runSingle(id, theoryName, async () => callback?.(caseData)));
      }
    }
  }

  return results;
}

async function runSingle(id: string, name: string, fn: () => unknown): Promise<TestResult> {
  const started = performance.now();
  try {
    await fn();
    verifyNoPendingExpectations();
    return { id, name, status: 'passed', durationMs: performance.now() - started };
  } catch (error) {
    if (isSkipSignal(error)) {
      clearPendingExpectations();
      return {
        id,
        name,
        status: 'skipped',
        durationMs: performance.now() - started,
        skipReason: error.skipReason,
      };
    }
    return { id, name, status: 'failed', durationMs: performance.now() - started, error: error as Error };
  }
}

function isNode(value: unknown): value is RuntimeNode {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const candidate = value as RuntimeNode;
  return typeof candidate.__tag === 'string' && !!candidate.props && Array.isArray(candidate.children);
}
