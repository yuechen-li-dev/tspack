import { runInspect } from "../inspect/backend.js";
import { normalizeCdpEndpoint } from "../inspect/cdp.js";
import type { InspectOptions as BackendInspectOptions } from "../inspect/index.js";
import type {
  Bounds as InspectBounds,
  UIHitTest as InspectHitTest,
  UIInspectNode as InspectNode,
  UIInspectResult as InspectResult,
} from "../inspect/types.js";

export type {
  InspectBounds,
  InspectHitTest,
  InspectNode,
  InspectResult,
};

export type InspectPoint = {
  x: number;
  y: number;
};

export type InspectViewport =
  | string
  | {
      width: number;
      height: number;
    };

export type InspectUrlBrowser =
  | "chromium"
  | "webkit"
  | "playwright-chromium"
  | "playwright-webkit";

export type InspectUrlOptions = {
  browser?: InspectUrlBrowser;
  selector?: string;
  viewport?: InspectViewport;
  points?: InspectPoint[];
};

export type InspectOptions = InspectUrlOptions;

export type InspectCdpOptions = {
  target?: number | string;
  targetUrl?: string;
  selector?: string;
  viewport?: InspectViewport;
  points?: InspectPoint[];
};

type InspectBackend = (
  options: BackendInspectOptions,
) => Promise<InspectResult>;

type InspectHelper = {
  url: (url: string, options?: InspectUrlOptions) => Promise<InspectResult>;
  cdp: (
    endpoint: string,
    options?: InspectCdpOptions,
  ) => Promise<InspectResult>;
  target: (
    endpoint: string,
    options?: InspectCdpOptions,
  ) => Promise<InspectResult>;
  cdpTarget: (
    endpoint: string,
    options?: InspectCdpOptions,
  ) => Promise<InspectResult>;
  flatten: (root: InspectNode | null | undefined) => InspectNode[];
  findByRole: (
    root: InspectNode | null | undefined,
    role: string,
    name?: string | RegExp,
  ) => InspectNode | undefined;
  findByText: (
    root: InspectNode | null | undefined,
    text: string | RegExp,
  ) => InspectNode | undefined;
};

const defaultViewport = {
  width: 1440,
  height: 900,
};

export function createInspectHelper(
  backend: InspectBackend = runInspect,
): InspectHelper {
  async function inspectUrl(
    url: string,
    options: InspectUrlOptions = {},
  ): Promise<InspectResult> {
    return callBackend(backend, {
      url,
      browser: options.browser ?? "chromium",
      viewport: normalizeViewport(options.viewport),
      selector: options.selector,
      points: normalizePoints(options.points),
      json: true,
    });
  }

  async function inspectCdp(
    endpoint: string,
    options: InspectCdpOptions = {},
  ): Promise<InspectResult> {
    return callBackend(backend, {
      browser: "cdp",
      viewport: normalizeViewport(options.viewport),
      selector: options.selector,
      points: normalizePoints(options.points),
      json: true,
      cdpEndpoint: normalizeCdpEndpoint(endpoint),
      target: normalizeCdpTarget(options.target),
      targetUrl: options.targetUrl,
    });
  }

  return {
    url: inspectUrl,
    cdp: inspectCdp,
    target: inspectCdp,
    cdpTarget: inspectCdp,
    flatten,
    findByRole,
    findByText,
  };
}

export const inspect = createInspectHelper();

function normalizeViewport(
  viewport: InspectViewport | undefined,
): { width: number; height: number } {
  if (!viewport) {
    return { ...defaultViewport };
  }

  if (typeof viewport === "string") {
    const match = /^(\d+)x(\d+)$/i.exec(viewport);
    if (!match) {
      throw inspectError("TSPACK_INSPECT_INVALID_VIEWPORT");
    }

    return validateViewport({
      width: Number(match[1]),
      height: Number(match[2]),
    });
  }

  return validateViewport(viewport);
}

function validateViewport(viewport: {
  width: number;
  height: number;
}): { width: number; height: number } {
  if (
    !Number.isInteger(viewport.width) ||
    !Number.isInteger(viewport.height) ||
    viewport.width <= 0 ||
    viewport.height <= 0
  ) {
    throw inspectError("TSPACK_INSPECT_INVALID_VIEWPORT");
  }

  return {
    width: viewport.width,
    height: viewport.height,
  };
}

function normalizePoints(points: InspectPoint[] | undefined): InspectPoint[] {
  if (!points) {
    return [];
  }

  return points.map((point) => {
    if (
      !Number.isFinite(point.x) ||
      !Number.isFinite(point.y) ||
      point.x < 0 ||
      point.y < 0
    ) {
      throw inspectError("TSPACK_INSPECT_INVALID_POINT");
    }

    return {
      x: point.x,
      y: point.y,
    };
  });
}

function normalizeCdpTarget(
  target: number | string | undefined,
): string | undefined {
  if (target === undefined) {
    return undefined;
  }

  return String(target);
}

function inspectError(code: string): Error & { code: string } {
  const error = new Error(code) as Error & { code: string };
  error.code = code;
  return error;
}

async function callBackend(
  backend: InspectBackend,
  options: BackendInspectOptions,
): Promise<InspectResult> {
  try {
    return await backend(options);
  } catch (error: unknown) {
    if (error instanceof Error) {
      attachInspectErrorCode(error);
      throw error;
    }

    throw inspectError("TSPACK_TEST_INSPECT_FAILED");
  }
}

function attachInspectErrorCode(error: Error): void {
  const withCode = error as Error & { code?: string };
  if (withCode.code) {
    return;
  }

  const code = error.message.split(/\s|:/, 1)[0];
  if (code.startsWith("TSPACK_INSPECT_")) {
    withCode.code = code;
    return;
  }

  withCode.code = "TSPACK_TEST_INSPECT_FAILED";
}

export function flatten(root: InspectNode | null | undefined): InspectNode[] {
  if (!root) {
    return [];
  }

  const nodes: InspectNode[] = [];
  const pending: InspectNode[] = [root];

  while (pending.length > 0) {
    const node = pending.shift() as InspectNode;
    nodes.push(node);

    for (const child of node.children) {
      pending.push(child);
    }
  }

  return nodes;
}

export function findByRole(
  root: InspectNode | null | undefined,
  role: string,
  name?: string | RegExp,
): InspectNode | undefined {
  return flatten(root).find((node) => {
    if (node.role !== role) {
      return false;
    }

    if (name === undefined) {
      return true;
    }

    return matchesText(node.name, name);
  });
}

export function findByText(
  root: InspectNode | null | undefined,
  text: string | RegExp,
): InspectNode | undefined {
  return flatten(root).find((node) => matchesText(node.text, text));
}

function matchesText(
  value: string | undefined,
  expected: string | RegExp,
): boolean {
  if (value === undefined) {
    return false;
  }

  if (typeof expected === "string") {
    return value === expected;
  }

  return expected.test(value);
}
