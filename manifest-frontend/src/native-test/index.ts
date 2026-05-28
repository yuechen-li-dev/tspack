export { assert } from './assert.js';
export { expect, verifyNoPendingExpectations } from './expect.js';
export { skip } from './skip.js';
export { discoverNativeTestFile, discoverNativeTestFiles } from './discover.js';
export { runSuite } from './runner.js';
export { runNativeTestFiles, runNativeArtifacts } from './file-runner.js';
export { listNativeBenchmarks, runNativeBenchmarks } from './benchmark.js';
export { listNativeProphecies, runNativeProphecies, createNativeDoomReport, formatNativeDoomTextReport, formatNativeDoomJsonReport, nativeDoomExitCode } from './doom.js';
export { listDiscoveredTests, listStandaloneArtifacts, listNativeTests, listNativeArtifacts, createNativeTestReport, createNativeArtifactReport, createNativeBenchmarkReport, formatNativeTestTextReport, formatNativeTestCompactTextReport, formatNativeArtifactTextReport, formatNativeBenchmarkTextReport, formatNativeTestJsonReport, formatNativeArtifactJsonReport, formatNativeBenchmarkJsonReport, nativeTestExitCode, nativeArtifactExitCode, nativeBenchmarkExitCode } from './list-report.js';
export type { TestResult, DiscoveryResult, Diagnostic } from './types.js';

type NodeShape = {
  __tag: 'Suite' | 'Fact' | 'Theory' | 'Case' | 'Artifact' | 'Valid' | 'Invalid' | 'Project' | 'CycleTime' | 'Benchmark' | 'Iterations' | 'Warmup' | 'Prophecy' | 'Foretell';
  props: Record<string, unknown>;
  children: unknown[];
};

function makeTag(tag: NodeShape['__tag']) {
  return function tagFactory(props: Record<string, unknown>, ...children: unknown[]): NodeShape {
    return { __tag: tag, props: props ?? {}, children };
  };
}

export const Suite = makeTag('Suite');
export const Fact = makeTag('Fact');
export const Theory = makeTag('Theory');
export const Case = makeTag('Case');
export const Artifact = makeTag('Artifact');
export const Valid = makeTag('Valid');
export const Invalid = makeTag('Invalid');

export const Project = makeTag('Project');
export const CycleTime = makeTag('CycleTime');
export const Benchmark = makeTag('Benchmark');
export const Iterations = makeTag('Iterations');
export const Warmup = makeTag('Warmup');
export const Prophecy = makeTag('Prophecy');
export const Foretell = makeTag('Foretell');
