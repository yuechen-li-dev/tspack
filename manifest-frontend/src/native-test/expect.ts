import { isDeepStrictEqual } from 'node:util';
import { markExpectationActivity } from './activity.js';
import { assertJsonSnapshot, assertTextSnapshot } from './snapshot.js';
import type { AssertionFailure } from './types.js';

type PendingExpectation = { because: (reason: string) => void };
type ExpectFunction = {
  (actual: unknown): {
    toBe: (expected: unknown) => PendingExpectation;
    toEqual: (expected: unknown) => PendingExpectation;
    not: {
      toBe: (expected: unknown) => PendingExpectation;
      toEqual: (expected: unknown) => PendingExpectation;
    };
  };
  error: (subject: unknown, code: string) => PendingExpectation;
  noErrors: (subject: unknown) => PendingExpectation;
  noError: (subject: unknown) => PendingExpectation;
  snapshotText: (value: unknown, name: string) => PendingExpectation;
  snapshotJson: (value: unknown, name: string) => PendingExpectation;
};
const pending = new Set<{ finalize: (reason: string) => void }>();

type DiagnosticLike = { code?: unknown; severity?: unknown };

function requiredReason(reason: string): void {
  if (typeof reason !== 'string' || reason.trim().length === 0) {
    const error = new Error('expect reason is required') as AssertionFailure;
    error.code = 'TSPACK_ASSERT_REASON_REQUIRED';
    error.assertion = 'because';
    error.reason = '';
    throw error;
  }
}

function extractDiagnostics(subject: unknown): DiagnosticLike[] {
  if (Array.isArray(subject)) return subject;
  if (subject && typeof subject === 'object' && Array.isArray((subject as { diagnostics?: unknown }).diagnostics)) {
    return (subject as { diagnostics: DiagnosticLike[] }).diagnostics;
  }
  return [];
}

function expectationFailure(code: string, assertion: string, reason: string, expected: unknown, actual: unknown): AssertionFailure {
  const error = new Error(`${assertion} failed: ${reason}`) as AssertionFailure;
  error.code = code;
  error.assertion = assertion;
  error.reason = reason;
  error.expected = expected;
  error.actual = actual;
  return error;
}

function buildPending(actual: unknown, expected: unknown, useDeepEqual: boolean, negate: boolean): PendingExpectation {
  const token = { finalize(reason: string): void {
    requiredReason(reason);
    const matched = useDeepEqual ? isDeepStrictEqual(actual, expected) : Object.is(actual, expected);
    const ok = negate ? !matched : matched;
    if (!ok) throw expectationFailure('TSPACK_ASSERT_FAILURE', `${negate ? 'not.' : ''}${useDeepEqual ? 'toEqual' : 'toBe'}`, reason, expected, actual);
  } };
  pending.add(token);
  return { because(reason: string): void { markExpectationActivity(); pending.delete(token); token.finalize(reason); } };
}

function buildErrorExpectation(subject: unknown, expectedCode: string): PendingExpectation {
  const token = { finalize(reason: string): void {
    requiredReason(reason);
    const diagnostics = extractDiagnostics(subject);
    const codes = diagnostics.map((entry) => String((entry as { code?: string }).code ?? ''));
    if (!codes.includes(expectedCode)) {
      throw expectationFailure('TSPACK_EXPECT_ERROR_NOT_FOUND', 'error', reason, expectedCode, codes);
    }
  } };
  pending.add(token);
  return { because(reason: string): void { markExpectationActivity(); pending.delete(token); token.finalize(reason); } };
}

function isErrorSeverity(value: unknown): boolean {
  if (typeof value !== 'string') return true;
  return value.toLowerCase() !== 'warning' && value.toLowerCase() !== 'info';
}

function buildNoErrorsExpectation(subject: unknown): PendingExpectation {
  const token = { finalize(reason: string): void {
    requiredReason(reason);
    const diagnostics = extractDiagnostics(subject);
    const errorCodes = diagnostics.filter((entry) => isErrorSeverity(entry.severity)).map((entry) => String(entry.code ?? ''));
    if (errorCodes.length > 0) {
      throw expectationFailure('TSPACK_EXPECT_UNEXPECTED_ERRORS', 'noErrors', reason, [], errorCodes);
    }
  } };
  pending.add(token);
  return { because(reason: string): void { markExpectationActivity(); pending.delete(token); token.finalize(reason); } };
}


function buildSnapshotExpectation(
  value: unknown,
  name: string,
  kind: 'text' | 'json',
): PendingExpectation {
  const token = { finalize(reason: string): void {
    requiredReason(reason);
    if (kind === 'text') {
      assertTextSnapshot(value, name, reason);
      return;
    }
    assertJsonSnapshot(value, name, reason);
  } };
  pending.add(token);
  return { because(reason: string): void { markExpectationActivity(); pending.delete(token); token.finalize(reason); } };
}

function baseExpect(actual: unknown) {
  const mk = (negate: boolean) => ({ toBe: (expected: unknown) => buildPending(actual, expected, false, negate), toEqual: (expected: unknown) => buildPending(actual, expected, true, negate) });
  return { ...mk(false), not: mk(true) };
}

export const expect = baseExpect as ExpectFunction;

expect.error = (subject: unknown, code: string): PendingExpectation => buildErrorExpectation(subject, code);
expect.noErrors = (subject: unknown): PendingExpectation => buildNoErrorsExpectation(subject);
expect.noError = (subject: unknown): PendingExpectation => buildNoErrorsExpectation(subject);
expect.snapshotText = (value: unknown, name: string): PendingExpectation => buildSnapshotExpectation(value, name, 'text');
expect.snapshotJson = (value: unknown, name: string): PendingExpectation => buildSnapshotExpectation(value, name, 'json');

export function verifyNoPendingExpectations(): void {
  if (pending.size > 0) {
    pending.clear();
    const error = new Error('missing because(reason) in expectation chain');
    (error as AssertionFailure).code = 'TSPACK_EXPECT_BECAUSE_REQUIRED';
    throw error;
  }
}
export function clearPendingExpectations(): void { pending.clear(); }
