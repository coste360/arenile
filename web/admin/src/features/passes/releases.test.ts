import { describe, expect, it } from 'vitest';
import type { SeasonRelease } from './api';
import { createPassSchema, toPassCreate, toPassQuote } from './pass.schema';
import {
  dayStatus,
  isReleasable,
  pruneSelection,
  releasedHalves,
  releasesCredit,
  toggleDate,
} from './releases';

function release(
  date: string,
  slot: 'AM' | 'PM',
  status: SeasonRelease['status'],
  credit = 0,
): SeasonRelease {
  return {
    id: `${date}-${slot}`,
    pass_id: 'p',
    date,
    slot,
    status,
    credit_amount: credit,
    created_at: '2026-06-01T00:00:00Z',
  };
}

const pass = { starts_on: '2026-06-01', ends_on: '2026-09-15', slots: 'FULL' as const };
const releases = [
  release('2026-07-10', 'AM', 'released'),
  release('2026-07-11', 'AM', 'resold', 1200),
  release('2026-07-11', 'PM', 'released'),
  release('2026-07-12', 'AM', 'revoked'),
];
const released = releasedHalves(releases);
const today = '2026-07-01';

describe('release selection', () => {
  it('ignores revoked releases', () => {
    expect([...(released.get('2026-07-11') ?? [])].sort()).toEqual(['AM', 'PM']);
    expect(released.has('2026-07-12')).toBe(false);
  });

  it('allows only future days inside the pass with free halves', () => {
    expect(isReleasable('2026-07-20', 'FULL', pass, released, today)).toBe(true);
    expect(isReleasable('2026-06-20', 'FULL', pass, released, today)).toBe(false); // past
    expect(isReleasable('2026-09-16', 'FULL', pass, released, today)).toBe(false); // after the pass
    expect(isReleasable('2026-07-10', 'FULL', pass, released, today)).toBe(false); // AM already released
    expect(isReleasable('2026-07-10', 'PM', pass, released, today)).toBe(true); // PM still held
    expect(isReleasable('2026-07-12', 'FULL', pass, released, today)).toBe(true); // revoked = held again
  });

  it('never releases a half the pass does not hold', () => {
    const pmPass = { ...pass, slots: 'PM' as const };
    expect(isReleasable('2026-07-20', 'AM', pmPass, released, today)).toBe(false);
    expect(isReleasable('2026-07-20', 'FULL', pmPass, released, today)).toBe(false);
    expect(isReleasable('2026-07-20', 'PM', pmPass, released, today)).toBe(true);
  });

  it('toggles and prunes the selection when the half changes', () => {
    let sel = toggleDate([], '2026-07-20');
    sel = toggleDate(sel, '2026-07-10');
    expect(sel).toEqual(['2026-07-10', '2026-07-20']);
    expect(toggleDate(sel, '2026-07-20')).toEqual(['2026-07-10']);
    expect(pruneSelection(['2026-07-10', '2026-07-20'], 'FULL', pass, released, today)).toEqual([
      '2026-07-20',
    ]);
  });

  it('colours resold over released and sums the credit', () => {
    expect(dayStatus('2026-07-11', releases)).toBe('resold');
    expect(dayStatus('2026-07-10', releases)).toBe('released');
    expect(dayStatus('2026-07-12', releases)).toBeUndefined();
    expect(releasesCredit(releases)).toBe(1200);
  });
});

describe('create pass schema', () => {
  const v = {
    customer_id: 'c1',
    season_id: 's1',
    spot_id: 'sp1',
    pack_id: 'p1',
    slots: 'FULL' as const,
    starts_on: '',
    ends_on: '',
    notes_internal: '',
  };

  it('requires customer, season, spot and pack', () => {
    const r = createPassSchema.safeParse({ ...v, customer_id: '', spot_id: '' });
    expect(r.success).toBe(false);
    expect(r.error?.issues.map((i) => i.message).sort()).toEqual([
      'passes:validation.customer',
      'passes:validation.spot',
    ]);
  });

  it('sends only what differs from the season defaults', () => {
    expect(toPassCreate(v)).toEqual({
      customer_id: 'c1',
      season_id: 's1',
      spot_id: 'sp1',
      pack_id: 'p1',
    });
    expect(
      toPassCreate({ ...v, slots: 'AM', starts_on: '2026-07-01', notes_internal: 'rinnovo' }),
    ).toMatchObject({
      slots: 'AM',
      starts_on: '2026-07-01',
      notes_internal: 'rinnovo',
    });
  });

  it('quotes the engine price over the season window with formula season', () => {
    const season = { opens_on: '2026-05-15', closes_on: '2026-09-20' };
    expect(toPassQuote(v, season, 'z1')).toEqual({
      channel: 'desk',
      starts_on: '2026-05-15',
      ends_on: '2026-09-20',
      slots: 'FULL',
      formula: 'season',
      lines: [{ pack_id: 'p1', spot_id: 'sp1', zone_id: 'z1' }],
    });
    expect(toPassQuote({ ...v, pack_id: '' }, season, 'z1')).toBeNull();
    expect(toPassQuote(v, undefined, 'z1')).toBeNull();
  });
});
