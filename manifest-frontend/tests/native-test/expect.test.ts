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

  it('supports not matcher', () => {
    expect(1).not.toBe(2).because('not equal');
  });

  it('expect.error supports Diagnostic[] and diagnostics object shapes', () => {
    expect.error([{ code: 'E1' }], 'E1').because('array works');
    expect.error({ diagnostics: [{ code: 'E2' }] }, 'E2').because('object works');
    expect.error({ ok: false, diagnostics: [{ code: 'E3' }] }, 'E3').because('ok+diagnostics works');
  });

  it('expect.error failure includes actual codes', () => {
    let thrown: unknown;
    try {
      expect.error([{ code: 'A' }, { code: 'B' }], 'Z').because('must fail');
    } catch (error) {
      thrown = error;
    }
    const assertion = thrown as Error & { code?: string; actual?: unknown };
    vexpect(assertion.code).toBe('TSPACK_EXPECT_ERROR_NOT_FOUND');
    vexpect(assertion.actual).toEqual(['A', 'B']);
  });

  it('expect.noErrors semantics by severity', () => {
    expect.noErrors([]).because('empty passes');
    expect.noErrors([{ code: 'W1', severity: 'warning' }, { code: 'I1', severity: 'info' }]).because('warnings and infos pass');
    vexpect(() => expect.noErrors([{ code: 'E1', severity: 'error' }]).because('error fails')).toThrow();
    vexpect(() => expect.noErrors([{ code: 'X1' }]).because('no severity treated as error')).toThrow();
  });
});
