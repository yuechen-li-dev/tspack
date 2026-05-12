import { isDeepStrictEqual } from 'node:util';
import type { AssertionFailure } from './types.js';

type PendingExpectation = {
  because: (reason: string) => void;
};

const pending = new Set<{ finalize: (reason: string) => void }>();

function requiredReason(reason: string): void {
  if (typeof reason !== 'string' || reason.trim().length === 0) {
    const error = new Error('expect reason is required') as AssertionFailure;
    error.code = 'TSPACK_ASSERT_REASON_REQUIRED';
    error.assertion = 'because';
    error.reason = '';
    throw error;
  }
}

function expectationFailure(assertion: string, reason: string, expected: unknown, actual: unknown): AssertionFailure {
  const error = new Error(`${assertion} failed: ${reason}`) as AssertionFailure;
  error.code = 'TSPACK_ASSERT_FAILURE';
  error.assertion = assertion;
  error.reason = reason;
  error.expected = expected;
  error.actual = actual;
  return error;
}

function buildPending(actual: unknown, expected: unknown, useDeepEqual: boolean, negate: boolean): PendingExpectation {
  const token = {
    finalize(reason: string): void {
      requiredReason(reason);
      const matched = useDeepEqual ? isDeepStrictEqual(actual, expected) : Object.is(actual, expected);
      const ok = negate ? !matched : matched;
      if (!ok) {
        const name = `${negate ? 'not.' : ''}${useDeepEqual ? 'toEqual' : 'toBe'}`;
        throw expectationFailure(name, reason, expected, actual);
      }
    },
  };

  pending.add(token);

  return {
    because(reason: string): void {
      pending.delete(token);
      token.finalize(reason);
    },
  };
}

export function expect(actual: unknown) {
  const mk = (negate: boolean) => ({
    toBe(expected: unknown): PendingExpectation {
      return buildPending(actual, expected, false, negate);
    },
    toEqual(expected: unknown): PendingExpectation {
      return buildPending(actual, expected, true, negate);
    },
  });

  return {
    ...mk(false),
    not: mk(true),
  };
}

export function verifyNoPendingExpectations(): void {
  if (pending.size > 0) {
    pending.clear();
    const error = new Error('missing because(reason) in expectation chain');
    (error as AssertionFailure).code = 'TSPACK_EXPECT_BECAUSE_REQUIRED';
    throw error;
  }
}

export function clearPendingExpectations(): void {
  pending.clear();
}
