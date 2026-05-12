import { isDeepStrictEqual } from 'node:util';
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
};
