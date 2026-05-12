export { assert } from './assert.js';
export { expect, verifyNoPendingExpectations } from './expect.js';
export { discoverNativeTestFile, discoverNativeTestFiles } from './discover.js';
export { runSuite } from './runner.js';
export { runNativeTestFiles } from './file-runner.js';
export type { TestResult, DiscoveryResult, Diagnostic } from './types.js';

type NodeShape = {
  __tag: 'Suite' | 'Fact' | 'Theory' | 'Case';
  props: Record<string, unknown>;
  children: unknown[];
};

function makeTag(tag: NodeShape['__tag']) {
  return function tagFactory(props: Record<string, unknown>, ...children: unknown[]): NodeShape {
    return {
      __tag: tag,
      props: props ?? {},
      children,
    };
  };
}

export const Suite = makeTag('Suite');
export const Fact = makeTag('Fact');
export const Theory = makeTag('Theory');
export const Case = makeTag('Case');
