import { describe, expect, it } from 'vitest';
import { frontendVersion } from '../src/index';

describe('frontendVersion', () => {
  it('returns placeholder dev version', () => {
    expect(frontendVersion()).toBe('manifest-frontend 0.0.0-dev');
  });
});
