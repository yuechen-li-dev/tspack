import { describe, expect, it } from 'vitest';
import { assert, Case, expect as tExpect, Fact, runSuite, Suite, Theory } from '../../src/native-test/index';

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
});
