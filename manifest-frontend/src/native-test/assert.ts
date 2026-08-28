import { isDeepStrictEqual } from "node:util";
import { markAssertActivity } from "./activity.js";
import type { AssertionFailure, DoomResult } from "./types.js";
import type { CommandResult } from "./command.js";
import type { InspectBounds, InspectHitTest, InspectNode } from "./inspect.js";
type DiagnosticLike = { code?: unknown; severity?: unknown };

export type InspectBoundsConstraints = {
  minWidth?: number;
  minHeight?: number;
  maxWidth?: number;
  maxHeight?: number;
  minX?: number;
  minY?: number;
  maxX?: number;
  maxY?: number;
};

export type InspectHitExpected = {
  role?: string;
  name?: string;
  tag?: string;
};

export type InspectSourceExpected = {
  file?: string;
  component?: string;
  symbol?: string;
};

type InspectNodeLike = InspectNode | null | undefined;

function createInspectFailure(
  code: string,
  assertion: string,
  reason: string,
  expected: unknown,
  actual: unknown,
  details: Record<string, unknown> = {},
): AssertionFailure {
  const failure = createFailure(assertion, reason, expected, actual);
  failure.code = code;
  failure.details = {
    ...details,
    assertionKind: assertion,
  };
  return failure;
}

function summarizeInspectNode(
  node: InspectNodeLike,
): Record<string, unknown> | null {
  if (!node) {
    return null;
  }

  return {
    tag: node.tag,
    role: node.role,
    name: node.name,
    text: snippet(node.text ?? node.name),
    bounds: node.bounds,
    visible: node.visible,
    focusable: node.focusable,
    source: node.source,
  };
}

function snippet(value: string | undefined): string | undefined {
  if (!value) {
    return undefined;
  }

  const normalized = value.replace(/\s+/g, " ").trim();
  if (normalized.length <= 120) {
    return normalized;
  }

  return `${normalized.slice(0, 117)}...`;
}

function failMissingInspectNode(assertion: string, reason: string): never {
  throw createInspectFailure(
    inspectFailureCode(assertion),
    assertion,
    reason,
    "inspect node",
    null,
    { node: null },
  );
}

function inspectFailureCode(assertion: string): string {
  switch (assertion) {
    case "inspect.exists":
      return "TSPACK_ASSERT_INSPECT_EXISTS_FAILED";
    case "inspect.visible":
      return "TSPACK_ASSERT_INSPECT_VISIBLE_FAILED";
    case "inspect.hidden":
      return "TSPACK_ASSERT_INSPECT_HIDDEN_FAILED";
    case "inspect.role":
      return "TSPACK_ASSERT_INSPECT_ROLE_FAILED";
    case "inspect.name":
      return "TSPACK_ASSERT_INSPECT_NAME_FAILED";
    case "inspect.focusable":
      return "TSPACK_ASSERT_INSPECT_FOCUSABLE_FAILED";
    case "inspect.boundsWithin":
      return "TSPACK_ASSERT_INSPECT_BOUNDS_FAILED";
    case "inspect.hitIncludes":
      return "TSPACK_ASSERT_INSPECT_HIT_FAILED";
    case "inspect.source":
      return "TSPACK_ASSERT_INSPECT_SOURCE_FAILED";
    default:
      return "TSPACK_ASSERT_INSPECT_FAILED";
  }
}

function isInspectNode(value: InspectNodeLike): value is InspectNode {
  return value !== null && value !== undefined;
}

function boundsFailures(
  bounds: InspectBounds | undefined,
  constraints: InspectBoundsConstraints,
): string[] {
  if (!bounds) {
    return ["bounds must exist"];
  }

  const failures: string[] = [];
  if (
    constraints.minWidth !== undefined &&
    bounds.width < constraints.minWidth
  ) {
    failures.push(`width ${bounds.width} < minWidth ${constraints.minWidth}`);
  }
  if (
    constraints.minHeight !== undefined &&
    bounds.height < constraints.minHeight
  ) {
    failures.push(
      `height ${bounds.height} < minHeight ${constraints.minHeight}`,
    );
  }
  if (
    constraints.maxWidth !== undefined &&
    bounds.width > constraints.maxWidth
  ) {
    failures.push(`width ${bounds.width} > maxWidth ${constraints.maxWidth}`);
  }
  if (
    constraints.maxHeight !== undefined &&
    bounds.height > constraints.maxHeight
  ) {
    failures.push(
      `height ${bounds.height} > maxHeight ${constraints.maxHeight}`,
    );
  }
  if (constraints.minX !== undefined && bounds.x < constraints.minX) {
    failures.push(`x ${bounds.x} < minX ${constraints.minX}`);
  }
  if (constraints.minY !== undefined && bounds.y < constraints.minY) {
    failures.push(`y ${bounds.y} < minY ${constraints.minY}`);
  }
  if (constraints.maxX !== undefined && bounds.x > constraints.maxX) {
    failures.push(`x ${bounds.x} > maxX ${constraints.maxX}`);
  }
  if (constraints.maxY !== undefined && bounds.y > constraints.maxY) {
    failures.push(`y ${bounds.y} > maxY ${constraints.maxY}`);
  }

  return failures;
}

function hitElementMatches(
  node: InspectNode,
  expected: InspectHitExpected,
): boolean {
  if (expected.role !== undefined && node.role !== expected.role) {
    return false;
  }
  if (expected.name !== undefined && node.name !== expected.name) {
    return false;
  }
  if (expected.tag !== undefined && node.tag !== expected.tag) {
    return false;
  }

  return true;
}

function summarizeHitElements(
  hitTest: InspectHitTest | null | undefined,
): Record<string, unknown>[] {
  if (!hitTest) {
    return [];
  }

  return hitTest.elements.map((element) => ({
    tag: element.tag,
    role: element.role,
    name: element.name,
    bounds: element.bounds,
    visible: element.visible,
  }));
}

function sourceFailures(
  node: InspectNode,
  expected: InspectSourceExpected,
): string[] {
  const failures: string[] = [];
  const source = node.source;
  if (!source) {
    return ["source must exist"];
  }

  if (expected.file !== undefined && source.file !== expected.file) {
    failures.push(`file ${String(source.file)} !== ${expected.file}`);
  }
  if (
    expected.component !== undefined &&
    source.component !== expected.component
  ) {
    failures.push(
      `component ${String(source.component)} !== ${expected.component}`,
    );
  }
  if (expected.symbol !== undefined && source.symbol !== expected.symbol) {
    failures.push(`symbol ${String(source.symbol)} !== ${expected.symbol}`);
  }

  return failures;
}

function createFailure(
  assertion: string,
  reason: string,
  expected?: unknown,
  actual?: unknown,
): AssertionFailure {
  const error = new Error(`${assertion} failed: ${reason}`) as AssertionFailure;
  error.code = "TSPACK_ASSERT_FAILURE";
  error.assertion = assertion;
  error.reason = reason;
  error.expected = expected;
  error.actual = actual;
  return error;
}

function validateReason(reason: string): void {
  markAssertActivity();
  if (typeof reason !== "string" || reason.trim().length === 0) {
    const error = new Error("assertion reason is required") as AssertionFailure;
    error.code = "TSPACK_ASSERT_REASON_REQUIRED";
    error.assertion = "reason";
    error.reason = "";
    throw error;
  }
}

export const assert = {
  is(actual: unknown, expected: unknown, reason: string): void {
    validateReason(reason);
    if (!Object.is(actual, expected)) {
      throw createFailure("is", reason, expected, actual);
    }
  },
  equal(actual: unknown, expected: unknown, reason: string): void {
    validateReason(reason);
    if (!isDeepStrictEqual(actual, expected)) {
      throw createFailure("equal", reason, expected, actual);
    }
  },
  notEqual(actual: unknown, expected: unknown, reason: string): void {
    validateReason(reason);
    if (isDeepStrictEqual(actual, expected)) {
      throw createFailure("notEqual", reason, expected, actual);
    }
  },
  true(value: unknown, reason: string): void {
    validateReason(reason);
    if (value !== true) {
      throw createFailure("true", reason, true, value);
    }
  },
  false(value: unknown, reason: string): void {
    validateReason(reason);
    if (value !== false) {
      throw createFailure("false", reason, false, value);
    }
  },
  ok(value: unknown, reason: string): void {
    validateReason(reason);
    if (!value) {
      throw createFailure("ok", reason, "truthy", value);
    }
  },
  fail(reason: string): void {
    validateReason(reason);
    throw createFailure("fail", reason);
  },
  type<TExpected>(value: TExpected, reason: string): void {
    void value;
    validateReason(reason);
  },
  near(
    actual: number,
    expected: number,
    tolerance: number,
    reason: string,
  ): void {
    validateReason(reason);
    if (
      !Number.isFinite(actual) ||
      !Number.isFinite(expected) ||
      !Number.isFinite(tolerance) ||
      tolerance < 0
    ) {
      const failure = createFailure("near", reason, expected, actual);
      failure.code = "TSPACK_ASSERT_NEAR_FAILED";
      (
        failure as AssertionFailure & { tolerance: number; difference: number }
      ).tolerance = tolerance;
      (
        failure as AssertionFailure & { tolerance: number; difference: number }
      ).difference = Number.NaN;
      throw failure;
    }

    const difference = Math.abs(actual - expected);
    if (difference > tolerance) {
      const failure = createFailure("near", reason, expected, actual);
      failure.code = "TSPACK_ASSERT_NEAR_FAILED";
      (
        failure as AssertionFailure & { tolerance: number; difference: number }
      ).tolerance = tolerance;
      (
        failure as AssertionFailure & { tolerance: number; difference: number }
      ).difference = difference;
      throw failure;
    }
  },

  exitCode(result: CommandResult, expected: number, reason: string): void {
    validateReason(reason);
    if (result.timedOut || result.exitCode !== expected) {
      const failure = createFailure(
        "exitCode",
        reason,
        expected,
        result.exitCode,
      );
      failure.code = "TSPACK_ASSERT_EXIT_CODE_FAILED";
      failure.details = {
        exitCode: result.exitCode,
        timedOut: result.timedOut,
        signal: result.signal,
        diagnostics: result.diagnostics.map((entry) => entry.code),
      };
      throw failure;
    }
  },
  LGTM(subject: unknown, reason: string): void {
    validateReason(reason);
    const diagnostics = extractDiagnostics(subject);
    const errorCodes = diagnostics
      .filter((entry) => isErrorSeverity(entry.severity))
      .map((entry) => String(entry.code ?? ""));
    if (errorCodes.length > 0) {
      const failure = createFailure("LGTM", reason, [], errorCodes);
      failure.code = "TSPACK_ASSERT_LGTM_FAILED";
      throw failure;
    }
  },
  doom(
    result: DoomResult,
    expected: { reason?: string; abnormal?: boolean },
    reason: string,
  ): void {
    validateReason(reason);
    const abnormalExpected = expected.abnormal ?? true;
    const isAbnormal = result.status === "passed";
    if (abnormalExpected && !isAbnormal) {
      const failure = createFailure("doom", reason, true, false);
      failure.code = "TSPACK_ASSERT_DOOM_FAILED";
      throw failure;
    }
    if (
      expected.reason &&
      result.envelope?.foretell.reason !== expected.reason
    ) {
      const failure = createFailure(
        "doom",
        reason,
        expected.reason,
        result.envelope?.foretell.reason,
      );
      failure.code = "TSPACK_ASSERT_DOOM_FAILED";
      throw failure;
    }
  },
  inspect: {
    exists(node: InspectNodeLike, reason: string): void {
      validateReason(reason);
      if (!isInspectNode(node)) {
        failMissingInspectNode("inspect.exists", reason);
      }
    },
    visible(node: InspectNodeLike, reason: string): void {
      validateReason(reason);
      if (!isInspectNode(node)) {
        failMissingInspectNode("inspect.visible", reason);
      }
      if (node.visible !== true) {
        throw createInspectFailure(
          "TSPACK_ASSERT_INSPECT_VISIBLE_FAILED",
          "inspect.visible",
          reason,
          { visible: true },
          summarizeInspectNode(node),
          { node: summarizeInspectNode(node) },
        );
      }
    },
    hidden(node: InspectNodeLike, reason: string): void {
      validateReason(reason);
      if (!isInspectNode(node)) {
        failMissingInspectNode("inspect.hidden", reason);
      }
      if (node.visible !== false) {
        throw createInspectFailure(
          "TSPACK_ASSERT_INSPECT_HIDDEN_FAILED",
          "inspect.hidden",
          reason,
          { visible: false },
          summarizeInspectNode(node),
          { node: summarizeInspectNode(node) },
        );
      }
    },
    role(node: InspectNodeLike, role: string, reason: string): void {
      validateReason(reason);
      if (!isInspectNode(node)) {
        failMissingInspectNode("inspect.role", reason);
      }
      if (node.role !== role) {
        throw createInspectFailure(
          "TSPACK_ASSERT_INSPECT_ROLE_FAILED",
          "inspect.role",
          reason,
          role,
          node.role,
          { node: summarizeInspectNode(node) },
        );
      }
    },
    name(node: InspectNodeLike, name: string, reason: string): void {
      validateReason(reason);
      if (!isInspectNode(node)) {
        failMissingInspectNode("inspect.name", reason);
      }
      if (node.name !== name) {
        throw createInspectFailure(
          "TSPACK_ASSERT_INSPECT_NAME_FAILED",
          "inspect.name",
          reason,
          name,
          node.name,
          { node: summarizeInspectNode(node) },
        );
      }
    },
    focusable(
      node: InspectNodeLike,
      expected: boolean,
      reason: string,
    ): void {
      validateReason(reason);
      if (!isInspectNode(node)) {
        failMissingInspectNode("inspect.focusable", reason);
      }
      if (node.focusable !== expected) {
        throw createInspectFailure(
          "TSPACK_ASSERT_INSPECT_FOCUSABLE_FAILED",
          "inspect.focusable",
          reason,
          expected,
          node.focusable,
          { node: summarizeInspectNode(node) },
        );
      }
    },
    boundsWithin(
      node: InspectNodeLike,
      constraints: InspectBoundsConstraints,
      reason: string,
    ): void {
      validateReason(reason);
      if (!isInspectNode(node)) {
        failMissingInspectNode("inspect.boundsWithin", reason);
      }

      const failures = boundsFailures(node.bounds, constraints);
      if (failures.length > 0) {
        throw createInspectFailure(
          "TSPACK_ASSERT_INSPECT_BOUNDS_FAILED",
          "inspect.boundsWithin",
          reason,
          constraints,
          node.bounds ?? null,
          {
            failedConstraints: failures,
            node: summarizeInspectNode(node),
          },
        );
      }
    },
    hitIncludes(
      hitTest: InspectHitTest | null | undefined,
      expected: InspectHitExpected,
      reason: string,
    ): void {
      validateReason(reason);
      const elements = hitTest?.elements ?? [];
      const found = elements.some((element) =>
        hitElementMatches(element, expected),
      );
      if (!found) {
        throw createInspectFailure(
          "TSPACK_ASSERT_INSPECT_HIT_FAILED",
          "inspect.hitIncludes",
          reason,
          expected,
          summarizeHitElements(hitTest),
          {
            point: hitTest?.point,
            elements: summarizeHitElements(hitTest),
          },
        );
      }
    },
    source(
      node: InspectNodeLike,
      expected: InspectSourceExpected,
      reason: string,
    ): void {
      validateReason(reason);
      if (!isInspectNode(node)) {
        failMissingInspectNode("inspect.source", reason);
      }

      const failures = sourceFailures(node, expected);
      if (failures.length > 0) {
        throw createInspectFailure(
          "TSPACK_ASSERT_INSPECT_SOURCE_FAILED",
          "inspect.source",
          reason,
          expected,
          node.source ?? null,
          {
            failedFields: failures,
            node: summarizeInspectNode(node),
          },
        );
      }
    },
  },
};

function extractDiagnostics(subject: unknown): DiagnosticLike[] {
  if (Array.isArray(subject)) {
    return subject;
  }

  if (subject && typeof subject === "object") {
    const diagnostics = (subject as { diagnostics?: unknown }).diagnostics;
    if (Array.isArray(diagnostics)) {
      return diagnostics;
    }
  }

  return [];
}

function isErrorSeverity(value: unknown): boolean {
  if (typeof value !== "string") {
    return true;
  }

  const normalized = value.toLowerCase();
  return normalized !== "warning" && normalized !== "info";
}
