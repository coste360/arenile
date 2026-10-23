import type { TFunction } from 'i18next';
import { describe, expect, it } from 'vitest';
import { registerLabel } from './register';

const t = ((key: string) => `t:${key}`) as unknown as TFunction;

describe('registerLabel', () => {
  it.each([
    { code: 'main', want: 't:registers.main' },
    { code: 'bar', want: 'bar' },
  ])('$code -> $want', ({ code, want }) => {
    expect(registerLabel(t, code)).toBe(want);
  });
});
