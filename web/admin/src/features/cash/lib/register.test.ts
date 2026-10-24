import type { TFunction } from 'i18next';
import { describe, expect, it, vi } from 'vitest';
import {
  pickRegister,
  readPreferredRegister,
  registerLabel,
  sessionRegisterLabel,
  writePreferredRegister,
} from './register';

const t = ((key: string) => `t:${key}`) as unknown as TFunction;

describe('registerLabel', () => {
  it.each([
    { code: 'main', want: 't:registers.main' },
    { code: 'bar', want: 'bar' },
  ])('$code -> $want', ({ code, want }) => {
    expect(registerLabel(t, code)).toBe(want);
  });
});

describe('sessionRegisterLabel', () => {
  it('prefers the configured name', () => {
    expect(sessionRegisterLabel(t, { register: 'bar', register_name: 'Bar spiaggia' })).toBe(
      'Bar spiaggia',
    );
    expect(sessionRegisterLabel(t, { register: 'main' })).toBe('t:registers.main');
  });
});

describe('pickRegister', () => {
  const regs = [
    { id: 'm', is_default: true },
    { id: 'b', is_default: false },
  ];
  it.each([
    { preferred: 'b', want: 'b' },
    { preferred: 'gone', want: 'm' },
    { preferred: '', want: 'm' },
  ])('preferred $preferred -> $want', ({ preferred, want }) => {
    expect(pickRegister(regs, preferred)).toBe(want);
  });
  it('falls back to the first, then empty', () => {
    expect(pickRegister([{ id: 'x', is_default: false }], '')).toBe('x');
    expect(pickRegister([], 'x')).toBe('');
  });
});

describe('preferred register storage', () => {
  it('round-trips and survives a throwing storage', () => {
    writePreferredRegister('e1', 'b');
    expect(readPreferredRegister('e1')).toBe('b');
    const spy = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked');
    });
    expect(readPreferredRegister('e1')).toBe('');
    spy.mockRestore();
  });
});
