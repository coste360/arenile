import { describe, expect, it } from 'vitest';
import { opBooking, opDay, opSpot } from '../testing/fixtures';
import { chartRows, computeDayKpis, netTakings } from './kpis';
import { addDays, daysBetween, todayIn } from './dates';
import { bookingRows } from './worklist';

const day = opDay(
  [
    opSpot('s1', 'A1', 'free'),
    opSpot('s2', 'A2', 'booked', { booking: opBooking({ booking_id: 'b1' }) }),
    // Same booking on two spots counts once.
    opSpot('s3', 'A3', 'booked', { booking: opBooking({ booking_id: 'b1' }) }),
    opSpot('s4', 'A4', 'checked_in', { booking: opBooking({ booking_id: 'b2' }) }),
    opSpot('s5', 'A5', 'pass_holder', {
      booking: opBooking({ booking_id: 'p1', is_season_pass: true }),
    }),
    opSpot('s6', 'A6', 'released', { release_id: 'r1' }),
    opSpot('s7', 'A7', 'booked', {
      booking: opBooking({ booking_id: 'b3', status: 'held', balance_due: 1500 }),
    }),
  ],
  {
    totals: {
      spots: 7,
      free: 2,
      occupied: 5,
      arrivals: 3,
      departures: 1,
      checked_in: 1,
      balance_due_amount: 1500,
      pending_assignments: 2,
    },
  },
);

describe('computeDayKpis', () => {
  it('derives the figures of the day', () => {
    const k = computeDayKpis(day);
    expect(k).toMatchObject({
      occupancyPercent: 71,
      occupied: 5,
      spots: 7,
      arrivals: 3,
      departures: 1,
      checkedIn: 1,
      pendingCheckIns: 1,
      passHolders: 1,
      released: 1,
      pendingAssignments: 2,
      balanceDue: 1500,
    });
    expect(k.byStatus).toEqual({
      free: 1,
      booked_desk: 2,
      present: 1,
      season: 1,
      season_released: 1,
      to_collect: 1,
    });
  });

  it('is 0% on an empty map', () => {
    expect(computeDayKpis(opDay([])).occupancyPercent).toBe(0);
  });
});

describe('netTakings', () => {
  const p = (amount: number, direction: 'in' | 'refund', status = 'succeeded') =>
    ({
      id: `${amount}-${direction}`,
      provider: 'manual',
      method: 'cash',
      direction,
      amount,
      status,
      created_at: '2026-07-14T09:00:00Z',
    }) as Parameters<typeof netTakings>[0][number];
  it('sums succeeded payments minus refunds, in cents', () => {
    expect(
      netTakings([p(3000, 'in'), p(1999, 'in'), p(500, 'refund'), p(700, 'in', 'failed')]),
    ).toBe(4499);
  });
  it('leaves out payments taken in test mode (INV-74)', () => {
    expect(netTakings([p(3000, 'in'), { ...p(1000, 'in'), test: true }])).toBe(3000);
  });
});

describe('chartRows', () => {
  it('keeps legend order and drops empty statuses', () => {
    expect(chartRows({ season: 2, free: 3, hold: 0 })).toEqual([
      { status: 'free', count: 3 },
      { status: 'season', count: 2 },
    ]);
  });
});

describe('bookingRows', () => {
  it('groups spots by booking and sorts by code', () => {
    const rows = bookingRows(day, (status, b) => status === 'booked' && b.status === 'confirmed');
    expect(rows).toHaveLength(1);
    expect(rows[0]?.spots).toEqual(['A2', 'A3']);
  });
});

describe('dates', () => {
  it('computes today in the establishment time zone', () => {
    // 23:30 UTC on 14 July is already 15 July in Rome.
    const now = new Date('2026-07-14T23:30:00Z');
    expect(todayIn('Europe/Rome', now)).toBe('2026-07-15');
    expect(todayIn('UTC', now)).toBe('2026-07-14');
    expect(todayIn('Not/AZone', now)).toBe('2026-07-14');
  });
  it('adds days across months and counts them', () => {
    expect(addDays('2026-07-31', 1)).toBe('2026-08-01');
    expect(addDays('2026-03-01', -1)).toBe('2026-02-28');
    expect(daysBetween('2026-07-14', '2026-08-01')).toBe(18);
  });
});
