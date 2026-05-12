import { describe, expect as vexpect, it } from 'vitest';
import { expect, verifyNoPendingExpectations } from '../../src/native-test/expect';

describe('expect api', () => {
  it('toBe and toEqual with because', () => {
    expect(1).toBe(1).because('same value');
    expect({ a: 1 }).toEqual({ a: 1 }).because('same shape');
    vexpect(() => expect(1).toBe(2).because('must fail')).toThrow();
  });

  it('missing because is detected', () => {
    expect(1).toBe(1);
    vexpect(() => verifyNoPendingExpectations()).toThrowError(/TSPACK_EXPECT_BECAUSE_REQUIRED|missing because/);
  });

  it('empty reason fails', () => {
    vexpect(() => expect(1).toBe(1).because('')).toThrow();
  });

  it('supports not matcher', () => {
    expect(1).not.toBe(2).because('not equal');
  });
});
