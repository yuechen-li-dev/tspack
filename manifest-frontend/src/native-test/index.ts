export { assert } from './assert.js';
export { expect, verifyNoPendingExpectations } from './expect.js';
export { skip } from './skip.js';
export { discoverNativeTestFile, discoverNativeTestFiles } from './discover.js';
export { runSuite } from './runner.js';
export { runNativeTestFiles, runNativeArtifacts } from './file-runner.js';
export { listDiscoveredTests, listStandaloneArtifacts, listNativeTests, listNativeArtifacts, createNativeTestReport, createNativeArtifactReport, formatNativeTestTextReport, formatNativeArtifactTextReport, formatNativeTestJsonReport, formatNativeArtifactJsonReport, nativeTestExitCode, nativeArtifactExitCode } from './list-report.js';
export type { TestResult, DiscoveryResult, Diagnostic } from './types.js';

type NodeShape = {
  __tag: 'Suite' | 'Fact' | 'Theory' | 'Case' | 'Artifact' | 'Valid' | 'Invalid' | 'Project' | 'CycleTime';
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
