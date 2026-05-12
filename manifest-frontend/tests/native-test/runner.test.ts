import { describe, expect, it } from 'vitest';
import { assert, Case, expect as tExpect, Fact, runSuite, skip, Suite, Theory } from '../../src/native-test/index';

describe('native runner', () => {
  it('runs fact and theory in deterministic order', async () => {
    const root = Suite({ name: 'math' },
      Fact({ name: 'pass' }, () => {
        assert.equal(1 + 1, 2, 'fact should pass');
      }),
      Theory({ name: 'len' },
        Case({ input: 'a', expected: 1 }),
        Case({ input: 'abc', expected: 3 }),
        ({ input, expected }: { input: string; expected: number }) => {
          assert.equal(input.length, expected, 'case input length should match expected value');
        },
      ),
    );

    const results = await runSuite(root);
    expect(results.map((r) => r.id)).toEqual(['math/pass', 'math/len[0]', 'math/len[1]']);
    expect(results.every((r) => r.status === 'passed')).toBe(true);
  });

  it('captures sync and async failures', async () => {
    const root = Suite({ name: 's' },
      Fact({ name: 'sync-fail' }, () => {
        assert.equal(1, 2, 'sync failure reason');
      }),
      Fact({ name: 'async-fail' }, async () => {
        await Promise.resolve();
        assert.true(false, 'async failure reason');
      }),
      Fact({ name: 'expect-missing-because' }, () => {
        tExpect(1).toBe(1);
      }),
    );

    const results = await runSuite(root);
    expect(results.map((r) => r.status)).toEqual(['failed', 'failed', 'failed']);
  });

  it('supports skip in fact/theory and async flows', async () => {
    const touched: string[] = [];
    const root = Suite({ name: 'skip' },
      Fact({ name: 'fact-skip' }, () => {
        skip('fact intentionally skipped');
        touched.push('fact-after-skip');
      }),
      Fact({ name: 'async-skip' }, async () => {
        await Promise.resolve();
        skip('async fact skipped');
      }),
      Theory({ name: 'theory-case-skip' },
        Case({ value: 1 }),
        Case({ value: 2 }),
        ({ value }: { value: number }) => {
          if (value === 1) {
            skip('case 1 intentionally skipped');
          }
          assert.equal(value, 2, 'case 2 still executes');
          touched.push(`case-${value}`);
        },
      ),
    );

    const results = await runSuite(root);
    expect(touched).toEqual(['case-2']);
    expect(results.map((r) => r.status)).toEqual(['skipped', 'skipped', 'skipped', 'passed']);
    expect(results[0].skipReason).toBe('fact intentionally skipped');
    expect(results[1].skipReason).toBe('async fact skipped');
    expect(results[2].skipReason).toBe('case 1 intentionally skipped');
  });

  it('skip requires a reason', async () => {
    const root = Suite({ name: 'skip' },
      Fact({ name: 'missing reason' }, () => {
        skip('');
      }),
    );
    const results = await runSuite(root);
    expect(results[0].status).toBe('failed');
    expect((results[0].error as { code?: string }).code).toBe('TSPACK_SKIP_REASON_REQUIRED');
  });
});
