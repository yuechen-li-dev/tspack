import type { AssertionFailure } from './types.js';

export type SkipSignal = Error & {
  code: 'TSPACK_TEST_SKIPPED';
  skipReason: string;
};

export function skip(reason: string): never {
  if (typeof reason !== 'string' || reason.trim().length === 0) {
    const error = new Error('skip reason is required') as AssertionFailure;
    error.code = 'TSPACK_SKIP_REASON_REQUIRED';
    error.assertion = 'skip';
    error.reason = '';
    throw error;
  }

  const signal = new Error(`test skipped: ${reason}`) as SkipSignal;
  signal.code = 'TSPACK_TEST_SKIPPED';
  signal.skipReason = reason;
  throw signal;
}

export function isSkipSignal(error: unknown): error is SkipSignal {
  if (!error || typeof error !== 'object') {
    return false;
  }
  const candidate = error as Partial<SkipSignal>;
  return candidate.code === 'TSPACK_TEST_SKIPPED' && typeof candidate.skipReason === 'string';
}
