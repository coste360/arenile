import { describe, expect, it } from 'vitest';
import { opBooking, opDay, opSpot } from '../features/operations/testing/fixtures';
import { applyToDay } from './optimistic';
import type { OfflineOperation } from './types';

const base = {
  id: 'o1',
  idempotencyKey: 'k1',
  createdAt: '2026-07-14T08:00:00Z',
  seq: 1,
  status: 'pending' as const,
  establishmentId: 'e1',
  bookingId: 'b1',
  label: 'A2 · AR-0001',
};
const day = opDay([
  opSpot('s1', 'A1', 'free'),
  opSpot('s2', 'A2', 'booked', {
    am_status: 'booked',
    booking: opBooking({ total_amount: 3000, paid_amount: 1000, balance_due: 2000 }),
  }),
  opSpot('s3', 'A3', 'booked', { booking: opBooking({ booking_id: 'b2' }) }),
]);

describe('applyToDay', () => {
  it('check-in turns the booked spots of the booking into checked in', () => {
    const op: OfflineOperation = { ...base, kind: 'check_in' };
    const next = applyToDay(day, op);
    expect(next.spots[1]).toMatchObject({
      status: 'checked_in',
      am_status: 'checked_in',
      booking: { status: 'checked_in' },
    });
    expect(next.spots[2]?.status).toBe('booked');
    expect(next.totals.checked_in).toBe(1);
    expect(day.spots[1]?.status).toBe('booked'); // pure
  });

  it('raises the cached booking version by one for every kind (server bumps it once)', () => {
    const before = day.spots[1]?.booking?.version ?? 0;
    const ops: OfflineOperation[] = [
      { ...base, kind: 'check_in' },
      { ...base, kind: 'note', text: 'x' },
      { ...base, kind: 'cash_payment', amount: 100 },
    ];
    for (const op of ops) {
      expect(applyToDay(day, op).spots[1]?.booking?.version, op.kind).toBe(before + 1);
    }
    // Two queued operations: the second sees the version left by the first.
    const after = applyToDay(applyToDay(day, ops[2]!), ops[0]!);
    expect(after.spots[1]?.booking).toMatchObject({ version: before + 2, status: 'checked_in' });
  });

  it('check-in of a booking that is not confirmed changes nothing', () => {
    const op: OfflineOperation = { ...base, kind: 'check_in', bookingId: 'b2' };
    const cancelled = opDay([
      opSpot('s3', 'A3', 'booked', {
        booking: opBooking({ booking_id: 'b2', status: 'pending_payment' }),
      }),
    ]);
    expect(applyToDay(cancelled, op).spots[0]?.status).toBe('booked');
  });

  it('cash payment lowers the balance, never below zero', () => {
    const op: OfflineOperation = { ...base, kind: 'cash_payment', amount: 1500 };
    expect(applyToDay(day, op).spots[1]?.booking).toMatchObject({
      paid_amount: 2500,
      balance_due: 500,
    });
    const over: OfflineOperation = { ...base, kind: 'cash_payment', amount: 9000 };
    expect(applyToDay(day, over).spots[1]?.booking?.balance_due).toBe(0);
  });

  it('note marks the booking as having notes', () => {
    const op: OfflineOperation = { ...base, kind: 'note', text: 'Ombrellone rotto' };
    expect(applyToDay(day, op).spots[1]?.booking?.has_notes).toBe(true);
  });
});
