export const materializedXTestGlobals = String.raw`/**
 * Authoring-time globals for TSPack native xTest TSX files.
 * Keep this aligned with the native test runtime surface.
 * This file is editor/type support only; it is not runtime implementation.
 */

type TspackNativeChild =
  | JSX.Element
  | string
  | number
  | boolean
  | null
  | undefined
  | TspackNativeChild[];

type TspackInspectBounds = {
  x: number;
  y: number;
  width: number;
  height: number;
};

type TspackInspectSourceHint = {
  raw?: string;
  file?: string;
  line?: number;
  column?: number;
  component?: string;
  symbol?: string;
  parseError?: string;
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
  source?: TspackInspectSourceHint;
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

type TspackInspectPoint = {
  x: number;
  y: number;
};

type TspackInspectViewport =
  | string
  | {
      width: number;
      height: number;
    };

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

type TspackInspectBoundsConstraints = {
  minWidth?: number;
  minHeight?: number;
  maxWidth?: number;
  maxHeight?: number;
  minX?: number;
  minY?: number;
  maxX?: number;
  maxY?: number;
};

type TspackInspectHitExpected = {
  role?: string;
  name?: string;
  tag?: string;
};

type TspackInspectSourceExpected = {
  file?: string;
  component?: string;
  symbol?: string;
};

type TspackLifecycleProbePolicy = {
  denyNetwork?: boolean;
  denyChildProcess?: boolean;
  denyEnv?: string[];
  allowRead?: string[];
  allowWrite?: string[];
};

type TspackLifecycleRunScriptRequest = {
  packageDir: string;
  command: string;
  script?: string;
  policy?: TspackLifecycleProbePolicy;
  env?: Record<string, string>;
  timeoutSeconds?: number;
};

type TspackLifecycleViolation = {
  code: string;
  kind: string;
  detail: string;
  path?: string;
  module?: string;
  envKey?: string;
};

type TspackLifecycleRunScriptResult = {
  exitCode: number | null;
  signal?: string | null;
  timedOut: boolean;
  stdout: string;
  stderr: string;
  violations: TspackLifecycleViolation[];
  reads: string[];
  writes: string[];
};

type TspackDiagnosticLike = {
  code?: unknown;
  severity?: unknown;
};

type TspackCommandResult = {
  exitCode: number | null;
  signal?: string | null;
  stdout: string;
  stderr: string;
  timedOut: boolean;
  diagnostics: TspackDiagnosticLike[];
};

type TspackDoomResult = {
  status: "passed" | "failed" | "skipped";
  envelope?: {
    foretell: {
      reason: string;
    };
  };
};

type TspackPendingExpectation = {
  because(reason: string): void;
};

type TspackExpectChain<TActual> = {
  toBe(expected: TActual): TspackPendingExpectation;
  toEqual(expected: unknown): TspackPendingExpectation;
  not: {
    toBe(expected: TActual): TspackPendingExpectation;
    toEqual(expected: unknown): TspackPendingExpectation;
  };
};

type TspackExpect = {
  <TActual>(actual: TActual): TspackExpectChain<TActual>;
  error(subject: unknown, code: string): TspackPendingExpectation;
  noErrors(subject: unknown): TspackPendingExpectation;
  noError(subject: unknown): TspackPendingExpectation;
  snapshotText(value: unknown, name: string): TspackPendingExpectation;
  snapshotJson(value: unknown, name: string): TspackPendingExpectation;
};

type TspackNativeProps = Record<string, unknown> | null | undefined;
type TspackNativeTag = (
  props?: TspackNativeProps,
  ...children: TspackNativeChild[]
) => JSX.Element;

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
  is(actual: unknown, expected: unknown, reason: string): void;
  equal(actual: unknown, expected: unknown, reason: string): void;
  notEqual(actual: unknown, expected: unknown, reason: string): void;
  true(value: unknown, reason: string): void;
  false(value: unknown, reason: string): void;
  ok(value: unknown, reason: string): void;
  fail(reason: string): void;
  type<TExpected>(value: TExpected, reason: string): void;
  near(actual: number, expected: number, tolerance: number, reason: string): void;
  exitCode(result: TspackCommandResult, expected: number, reason: string): void;
  LGTM(subject: unknown, reason: string): void;
  doom(
    result: TspackDoomResult,
    expected: { reason?: string; abnormal?: boolean },
    reason: string,
  ): void;
  inspect: {
    exists(node: TspackInspectNode | null | undefined, reason: string): void;
    visible(node: TspackInspectNode | null | undefined, reason: string): void;
    hidden(node: TspackInspectNode | null | undefined, reason: string): void;
    role(
      node: TspackInspectNode | null | undefined,
      role: string,
      reason: string,
    ): void;
    name(
      node: TspackInspectNode | null | undefined,
      name: string,
      reason: string,
    ): void;
    boundsWithin(
      node: TspackInspectNode | null | undefined,
      constraints: TspackInspectBoundsConstraints,
      reason: string,
    ): void;
    hitIncludes(
      hitTest: TspackInspectHitTest | null | undefined,
      expected: TspackInspectHitExpected,
      reason: string,
    ): void;
    source(
      node: TspackInspectNode | null | undefined,
      expected: TspackInspectSourceExpected,
      reason: string,
    ): void;
  };
};

declare const expect: TspackExpect;
declare const skip: (reason: string) => never;
declare const runLifecycleScript: (
  request: TspackLifecycleRunScriptRequest,
) => Promise<TspackLifecycleRunScriptResult>;
declare const inspect: {
  url(url: string, options?: TspackInspectUrlOptions): Promise<TspackInspectResult>;
  cdp(
    endpoint: string,
    options?: TspackInspectCdpOptions,
  ): Promise<TspackInspectResult>;
  target(
    endpoint: string,
    options?: TspackInspectCdpOptions,
  ): Promise<TspackInspectResult>;
  cdpTarget(
    endpoint: string,
    options?: TspackInspectCdpOptions,
  ): Promise<TspackInspectResult>;
  flatten(root: TspackInspectNode | null | undefined): TspackInspectNode[];
  findByRole(
    root: TspackInspectNode | null | undefined,
    role: string,
    name?: string | RegExp,
  ): TspackInspectNode | undefined;
  findByText(
    root: TspackInspectNode | null | undefined,
    text: string | RegExp,
  ): TspackInspectNode | undefined;
};

declare namespace JSX {
  interface Element {
    readonly __jsxElement?: true;
  }

  interface ElementChildrenAttribute {
    children: {};
  }
}
`;

const internalOnlyIntrinsicElements = `
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

export const nativeTypecheckGlobals =
  materializedXTestGlobals + internalOnlyIntrinsicElements;
