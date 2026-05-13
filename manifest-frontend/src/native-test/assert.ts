import { isDeepStrictEqual } from 'node:util';
import { markAssertActivity } from './activity.js';
import type { AssertionFailure, DoomResult } from './types.js';
import type { CommandResult } from './command.js';
type DiagnosticLike = { code?: unknown; severity?: unknown };

function createFailure(assertion: string, reason: string, expected?: unknown, actual?: unknown): AssertionFailure {
  const error = new Error(`${assertion} failed: ${reason}`) as AssertionFailure;
  error.code = 'TSPACK_ASSERT_FAILURE';
  error.assertion = assertion;
  error.reason = reason;
  error.expected = expected;
  error.actual = actual;
  return error;
}

function validateReason(reason: string): void {
  markAssertActivity();
  if (typeof reason !== 'string' || reason.trim().length === 0) {
    const error = new Error('assertion reason is required') as AssertionFailure;
    error.code = 'TSPACK_ASSERT_REASON_REQUIRED';
    error.assertion = 'reason';
    error.reason = '';
    throw error;
  }
}

export const assert = {
  is(actual: unknown, expected: unknown, reason: string): void {
    validateReason(reason);
    if (!Object.is(actual, expected)) {
      throw createFailure('is', reason, expected, actual);
    }
  },
  equal(actual: unknown, expected: unknown, reason: string): void {
    validateReason(reason);
    if (!isDeepStrictEqual(actual, expected)) {
      throw createFailure('equal', reason, expected, actual);
    }
  },
  notEqual(actual: unknown, expected: unknown, reason: string): void {
    validateReason(reason);
    if (isDeepStrictEqual(actual, expected)) {
      throw createFailure('notEqual', reason, expected, actual);
    }
  },
  true(value: unknown, reason: string): void {
    validateReason(reason);
    if (value !== true) {
      throw createFailure('true', reason, true, value);
    }
  },
  false(value: unknown, reason: string): void {
    validateReason(reason);
    if (value !== false) {
      throw createFailure('false', reason, false, value);
    }
  },
  ok(value: unknown, reason: string): void {
    validateReason(reason);
    if (!value) {
      throw createFailure('ok', reason, 'truthy', value);
    }
  },
  fail(reason: string): void {
    validateReason(reason);
    throw createFailure('fail', reason);
  },
  near(actual: number, expected: number, tolerance: number, reason: string): void {
    validateReason(reason);
    if (!Number.isFinite(actual) || !Number.isFinite(expected) || !Number.isFinite(tolerance) || tolerance < 0) {
      const failure = createFailure('near', reason, expected, actual);
      failure.code = 'TSPACK_ASSERT_NEAR_FAILED';
      (failure as AssertionFailure & { tolerance: number; difference: number }).tolerance = tolerance;
      (failure as AssertionFailure & { tolerance: number; difference: number }).difference = Number.NaN;
      throw failure;
    }

    const difference = Math.abs(actual - expected);
    if (difference > tolerance) {
      const failure = createFailure('near', reason, expected, actual);
      failure.code = 'TSPACK_ASSERT_NEAR_FAILED';
      (failure as AssertionFailure & { tolerance: number; difference: number }).tolerance = tolerance;
      (failure as AssertionFailure & { tolerance: number; difference: number }).difference = difference;
      throw failure;
    }
  },

  exitCode(result: CommandResult, expected: number, reason: string): void {
    validateReason(reason);
    if (result.timedOut || result.exitCode !== expected) {
      const failure = createFailure('exitCode', reason, expected, result.exitCode);
      failure.code = 'TSPACK_ASSERT_EXIT_CODE_FAILED';
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
    const errorCodes = diagnostics.filter((entry) => isErrorSeverity(entry.severity)).map((entry) => String(entry.code ?? ''));
    if (errorCodes.length > 0) {
      const failure = createFailure('LGTM', reason, [], errorCodes);
      failure.code = 'TSPACK_ASSERT_LGTM_FAILED';
      throw failure;
    }
  },
  doom(result: DoomResult, expected: { reason?: string; abnormal?: boolean }, reason: string): void {
    validateReason(reason);
    const abnormalExpected = expected.abnormal ?? true;
    const isAbnormal = result.status === 'passed';
    if (abnormalExpected && !isAbnormal) {
      const failure = createFailure('doom', reason, true, false);
      failure.code = 'TSPACK_ASSERT_DOOM_FAILED';
      throw failure;
    }
    if (expected.reason && result.envelope?.foretell.reason !== expected.reason) {
      const failure = createFailure('doom', reason, expected.reason, result.envelope?.foretell.reason);
      failure.code = 'TSPACK_ASSERT_DOOM_FAILED';
      throw failure;
    }
  },
};

function extractDiagnostics(subject: unknown): DiagnosticLike[] {
  if (Array.isArray(subject)) {
    return subject;
  }

  if (subject && typeof subject === 'object') {
    const diagnostics = (subject as { diagnostics?: unknown }).diagnostics;
    if (Array.isArray(diagnostics)) {
      return diagnostics;
    }
  }

  return [];
}

function isErrorSeverity(value: unknown): boolean {
  if (typeof value !== 'string') {
    return true;
  }

  const normalized = value.toLowerCase();
  return normalized !== 'warning' && normalized !== 'info';
}
