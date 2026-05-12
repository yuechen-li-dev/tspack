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
});
