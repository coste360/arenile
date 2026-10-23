import { describe, expect, it } from 'vitest';
import { createPersistPolicy, offlineWindow } from './cache';

const NOW = new Date('2026-07-14T21:30:00Z'); // 23:30 in Rome, still the 14th
const policy = createPersistPolicy({
  currentEstablishmentId: () => 'e1',
  timezoneOf: () => 'Europe/Rome',
  now: () => NOW,
});
const keep = (queryKey: unknown[], data: unknown = {}) => policy({ queryKey, state: { data } });

describe('offlineWindow', () => {
  it('is today and tomorrow in the establishment time zone', () => {
    expect(offlineWindow('Europe/Rome', NOW)).toEqual(['2026-07-14', '2026-07-15']);
    expect(offlineWindow('Pacific/Kiritimati', NOW)).toEqual(['2026-07-15', '2026-07-16']);
  });
});

describe('createPersistPolicy_Allowlist', () => {
  it.each([
    ['signed-in user', ['auth', 'me'], true],
    ['establishments list', ['establishments', { limit: 200 }], true],
    ['map document', ['config', 'e1', 'maps', 'document', 'm1'], true],
    ['packs catalog', ['config', 'e1', 'packs', 'list', {}], true],
    ['team (staff e-mails)', ['config', 'e1', 'team'], false],
    ['pricing', ['config', 'e1', 'pricing', 'matrix', {}], false],
    ['day today', ['est', 'e1', 'operations', 'day', { date: '2026-07-14', slot: 'FULL' }], true],
    ['day tomorrow', ['est', 'e1', 'operations', 'day', { date: '2026-07-15', slot: 'AM' }], true],
    ['day yesterday', ['est', 'e1', 'operations', 'day', { date: '2026-07-13' }], false],
    ['day in two days', ['est', 'e1', 'operations', 'day', { date: '2026-07-16' }], false],
    ['other establishment', ['est', 'e2', 'operations', 'day', { date: '2026-07-14' }], false],
    ['other establishment config', ['config', 'e2', 'maps'], false],
    ['customers', ['est', 'e1', 'customers', 'list', {}], false],
    ['payments', ['est', 'e1', 'payments', 'takings', '2026-07-14'], false],
    ['reports', ['est', 'e1', 'reports', 'summary'], false],
    ['bookings list', ['est', 'e1', 'bookings', 'list', {}], false],
    ['auth other', ['auth', 'totp'], false],
  ])('%s', (_name, key, expected) => {
    expect(keep(key)).toBe(expected);
  });

  it.each([
    ['stay over today', { starts_on: '2026-07-10', ends_on: '2026-07-14' }, true],
    ['starts tomorrow', { starts_on: '2026-07-15', ends_on: '2026-07-20' }, true],
    ['ended yesterday', { starts_on: '2026-07-01', ends_on: '2026-07-13' }, false],
    ['starts later', { starts_on: '2026-07-16', ends_on: '2026-07-16' }, false],
    ['no dates', {}, false],
  ])('booking detail: %s', (_name, data, expected) => {
    expect(keep(['est', 'e1', 'bookings', 'detail', 'b1'], data)).toBe(expected);
  });

  it("accepts the establishment of the snapshot at restore time ('*')", () => {
    const any = createPersistPolicy({
      currentEstablishmentId: () => '*',
      timezoneOf: () => undefined,
      now: () => NOW,
    });
    expect(any({ queryKey: ['config', 'e2', 'maps'], state: {} })).toBe(true);
    expect(
      any({ queryKey: ['est', 'e2', 'operations', 'day', { date: '2026-07-14' }], state: {} }),
    ).toBe(true);
    expect(any({ queryKey: ['est', 'e2', 'customers', 'list', {}], state: {} })).toBe(false);
    expect(any({ queryKey: ['config', 'e2', 'team'], state: {} })).toBe(false);
  });

  it('keeps nothing establishment-scoped when no establishment is shown', () => {
    const none = createPersistPolicy({
      currentEstablishmentId: () => undefined,
      timezoneOf: () => undefined,
      now: () => NOW,
    });
    expect(none({ queryKey: ['config', 'e1', 'maps'], state: {} })).toBe(false);
    expect(none({ queryKey: ['auth', 'me'], state: {} })).toBe(true);
  });
});
