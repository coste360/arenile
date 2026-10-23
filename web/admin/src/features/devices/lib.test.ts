import { describe, expect, it } from 'vitest';
import type { Device } from './api';
import {
  byLastActivity,
  formatCountdown,
  isExpired,
  localDeadline,
  pairedWith,
  pairingBlock,
} from './lib';

const dev = (id: string, extra: Partial<Device> = {}): Device => ({
  id,
  establishment_id: 'e',
  user_id: 'u1',
  user_name: 'Mario',
  role: 'owner',
  name: id,
  platform: 'ios',
  kind: 'mobile',
  created_at: '2027-06-01T08:00:00Z',
  current: false,
  fiscal_bridge: false,
  ...extra,
});

describe('countdown', () => {
  it.each([
    [300_000, '5:00'],
    [299_001, '5:00'],
    [61_000, '1:01'],
    [9_000, '0:09'],
    [0, '0:00'],
    [-5_000, '0:00'],
  ])('%d ms left -> %s', (left, want) => {
    expect(formatCountdown(1_000_000 + left, 1_000_000)).toBe(want);
  });

  it('counts the lifetime from the response, not from expires_at', () => {
    expect(localDeadline(1_000)).toBe(301_000);
    expect(isExpired(301_000, 300_999)).toBe(false);
    expect(isExpired(301_000, 301_000)).toBe(true);
  });
});

describe('pairingBlock', () => {
  it.each([
    ['server', null],
    ['lan', null],
    ['remote', null],
    ['local', 'lan_mode_required'],
  ] as const)('%s -> %s', (mode, want) => {
    expect(pairingBlock(mode)).toBe(want);
  });
});

describe('pairedWith', () => {
  const known = new Set(['old']);
  it('finds the new device of the code user only', () => {
    expect(pairedWith([dev('old')], { user_id: 'u1' }, known)).toBeUndefined();
    expect(pairedWith([dev('x', { user_id: 'u2' })], { user_id: 'u1' }, known)).toBeUndefined();
    expect(
      pairedWith([dev('r', { revoked_at: '2027-06-01T09:00:00Z' })], { user_id: 'u1' }, known),
    ).toBeUndefined();
    expect(pairedWith([dev('old'), dev('new')], { user_id: 'u1' }, known)?.id).toBe('new');
  });
});

describe('byLastActivity', () => {
  it('orders by last use, else pairing, most recent first', () => {
    const rows = [
      dev('a'),
      dev('b', { last_used_at: '2027-06-03T08:00:00Z' }),
      dev('c', { created_at: '2027-06-02T08:00:00Z' }),
    ];
    expect([...rows].sort(byLastActivity).map((d) => d.id)).toEqual(['b', 'c', 'a']);
  });
});
