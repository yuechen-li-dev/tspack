import { isDeepStrictEqual } from 'node:util';
import { markAssertActivity } from './activity.js';
import type { AssertionFailure } from './types.js';

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
};
