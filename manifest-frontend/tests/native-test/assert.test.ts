import { describe, expect, it } from 'vitest';
import { assert } from '../../src/native-test/assert';

describe('assert api', () => {
  it('equal and is pass and fail', () => {
    assert.equal({ a: 1 }, { a: 1 }, 'deep equal works');
    assert.is(2, 2, 'is uses object is');
    expect(() => assert.equal(1, 2, 'not equal')).toThrow();
    expect(() => assert.is(1, 2, 'not same')).toThrow();
  });

  it('boolean and fail', () => {
    assert.true(true, 'must be true');
    assert.false(false, 'must be false');
    assert.ok(1, 'must be truthy');
    expect(() => assert.fail('forced failure')).toThrow();
  });

  it('missing or empty reason throws required code', () => {
    try {
      assert.ok(true, '');
    } catch (error) {
      expect((error as { code: string }).code).toBe('TSPACK_ASSERT_REASON_REQUIRED');
    }
  });

  it('near uses explicit tolerance with diagnostics', () => {
    assert.near(10.5, 10, 0.5, 'passes exactly on tolerance boundary');
    assert.near(10.4, 10, 0.5, 'passes within tolerance');

    try {
      assert.near(11, 10, 0.5, 'fails outside tolerance');
    } catch (error) {
      const failure = error as { code: string; reason: string; tolerance: number; difference: number };
      expect(failure.code).toBe('TSPACK_ASSERT_NEAR_FAILED');
      expect(failure.reason).toBe('fails outside tolerance');
      expect(failure.tolerance).toBe(0.5);
      expect(failure.difference).toBe(1);
    }
  });

  it('near rejects invalid numbers and negative tolerance', () => {
    expect(() => assert.near(1, 1, -1, 'negative tolerance is invalid')).toThrow();
    expect(() => assert.near(Number.NaN, 1, 0.1, 'actual must be finite')).toThrow();
    expect(() => assert.near(1, Number.POSITIVE_INFINITY, 0.1, 'expected must be finite')).toThrow();
    expect(() => assert.near(1, 1, Number.NaN, 'tolerance must be finite')).toThrow();
  });

  it('LGTM passes for clean diagnostics and ignores warning/info severities', () => {
    assert.LGTM([], 'empty array is clean');
    assert.LGTM({ diagnostics: [] }, 'diagnostics object is clean');
    assert.LGTM({ ok: true, diagnostics: [] }, 'ok+diagnostics object is clean');
    assert.LGTM([{ code: 'W1', severity: 'warning' }, { code: 'I1', severity: 'info' }], 'warning/info are non-fatal');
  });

  it('LGTM fails for error/no-severity diagnostics and includes codes', () => {
    let thrownError: unknown;
    try {
      assert.LGTM([{ code: 'E1', severity: 'error' }, { code: 'E2' }], 'must fail');
    } catch (error) {
      thrownError = error;
    }
    const failure = thrownError as { code: string; assertion: string; actual: unknown };
    expect(failure.code).toBe('TSPACK_ASSERT_LGTM_FAILED');
    expect(failure.assertion).toBe('LGTM');
    expect(failure.actual).toEqual(['E1', 'E2']);
  });

  it('LGTM requires a non-empty reason', () => {
    expect(() => assert.LGTM([], '')).toThrow();
    try {
      assert.LGTM([], '');
    } catch (error) {
      expect((error as { code?: string }).code).toBe('TSPACK_ASSERT_REASON_REQUIRED');
    }
  });
});
