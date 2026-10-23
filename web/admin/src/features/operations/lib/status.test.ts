import { SPOT_STATUSES } from '@arenile/shared/map';
import { describe, expect, it } from 'vitest';
import { opBooking, opSpot } from '../testing/fixtures';
import {
  OPERATIONAL_LEGEND,
  indicatorsBySpot,
  isBookable,
  searchSpots,
  slotStatus,
  statusesBySpot,
  toSpotStatus,
  type OperationalSpotStatus,
} from './status';

describe('toSpotStatus', () => {
  it.each<[OperationalSpotStatus, Parameters<typeof toSpotStatus>[1], string]>([
    ['free', undefined, 'free'],
    ['held', undefined, 'hold'],
    ['checked_in', opBooking(), 'present'],
    ['pass_holder', opBooking({ is_season_pass: true }), 'season'],
    ['released', undefined, 'season_released'],
    ['renewal_offer', undefined, 'renewal_hold'],
    ['blocked', undefined, 'blocked'],
    ['closed', undefined, 'unavailable'],
    ['inactive', undefined, 'unavailable'],
    ['booked', opBooking({ channel: 'desk' }), 'booked_desk'],
    ['booked', opBooking({ channel: 'online' }), 'booked_online'],
    ['booked', opBooking({ channel: 'api' }), 'booked_online'],
    ['booked', opBooking({ channel: 'online', balance_due: 500 }), 'to_collect'],
    ['booked', undefined, 'booked_desk'],
  ])('%s (%o) → %s', (status, booking, expected) => {
    expect(toSpotStatus(status, booking)).toBe(expected);
  });

  it('legend lists every palette status, so the legend never depends on the day', () => {
    expect([...OPERATIONAL_LEGEND].sort()).toEqual([...SPOT_STATUSES].sort());
  });
});

describe('slotStatus', () => {
  const spot = opSpot('s1', 'A1', 'booked', { am_status: 'free', pm_status: 'booked' });
  it('uses the half-day status for AM and PM, the day status for FULL', () => {
    expect(slotStatus(spot, 'AM')).toBe('free');
    expect(slotStatus(spot, 'PM')).toBe('booked');
    expect(slotStatus(spot, 'FULL')).toBe('booked');
  });
  it('falls back to the day status without half-day detail', () => {
    expect(slotStatus(opSpot('s2', 'A2', 'blocked'), 'AM')).toBe('blocked');
  });
});

describe('statusesBySpot / indicatorsBySpot', () => {
  const spots = [
    opSpot('s1', 'A1', 'free'),
    opSpot('s2', 'A2', 'booked', {
      booking: opBooking({ has_extras: true, has_notes: true, customer_tags: ['VIP'] }),
    }),
    opSpot('s3', 'A3', 'checked_in', { booking: opBooking({ booking_id: 'b2' }) }),
  ];
  it('maps every spot', () => {
    expect(statusesBySpot(spots, 'FULL')).toEqual({ s1: 'free', s2: 'booked_desk', s3: 'present' });
  });
  it('lists extra, note and VIP (case-insensitive tag) and omits spots without any', () => {
    expect(indicatorsBySpot(spots)).toEqual({ s2: ['extra', 'note', 'vip'] });
  });
});

describe('searchSpots', () => {
  const spots = [
    opSpot('s1', 'A1', 'free'),
    opSpot('s2', 'B7', 'booked', {
      booking: opBooking({ code: 'AR-7K3Q9', customer_display_name: 'Niccolò Bianchi' }),
    }),
  ];
  it.each([
    ['', []],
    ['   ', []],
    ['b7', ['s2']],
    ['7k3', ['s2']],
    ['niccolo', ['s2']],
    ['BIANCHI', ['s2']],
    ['a1', ['s1']],
    ['zzz', []],
  ])('%j → %j', (q, expected) => {
    expect(searchSpots(spots, q)).toEqual(expected);
  });
});

describe('isBookable', () => {
  it('only free spots and released pass days', () => {
    expect(isBookable('free')).toBe(true);
    expect(isBookable('released')).toBe(true);
    expect(isBookable('booked')).toBe(false);
    expect(isBookable('held')).toBe(false);
  });
});
