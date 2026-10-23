import { establishmentKeys, type Schemas } from '@arenile/shared/api';
import type { QueryClient } from '@tanstack/react-query';
import type { OfflineOperation } from './types';

type OperationsDay = Schemas['OperationsDay'];
type OperationalSpot = Schemas['OperationalSpot'];

function isOperationsDay(v: unknown): v is OperationsDay {
  return typeof v === 'object' && v !== null && Array.isArray((v as OperationsDay).spots);
}

/**
 * Pure: the operations day as it will look once `op` is applied, so that a
 * queued action is visible on the map and in the Oggi lists while offline.
 * The server answer replaces it after the replay (queries are invalidated).
 *
 * Every queued operation makes the server raise bookings.version by exactly
 * one (check-in and note through booking.Service.mutate, the cash payment
 * through store.ApplyBookingPayment), so the cached version is raised too:
 * a check-in queued after a cash payment on the same booking then carries
 * the version the server will have once the payment is replayed.
 */
export function applyToDay(day: OperationsDay, op: OfflineOperation): OperationsDay {
  let checkedIn = 0;
  const spots = day.spots.map((spot): OperationalSpot => {
    const b = spot.booking;
    if (!b || b.booking_id !== op.bookingId) return spot;
    const version = b.version + 1;
    switch (op.kind) {
      case 'check_in': {
        if (b.status !== 'confirmed') return spot;
        checkedIn++;
        const swap = (s: OperationalSpot['status'] | undefined) =>
          s === 'booked' ? 'checked_in' : s;
        return {
          ...spot,
          status: swap(spot.status) ?? spot.status,
          am_status: swap(spot.am_status),
          pm_status: swap(spot.pm_status),
          booking: { ...b, status: 'checked_in', version },
        };
      }
      case 'note':
        return { ...spot, booking: { ...b, has_notes: true, version } };
      case 'cash_payment': {
        const paid = b.paid_amount + op.amount;
        return {
          ...spot,
          booking: {
            ...b,
            paid_amount: paid,
            balance_due: Math.max(0, b.total_amount - paid),
            version,
          },
        };
      }
    }
  });
  const totals =
    op.kind === 'check_in' && checkedIn > 0
      ? { ...day.totals, checked_in: day.totals.checked_in + checkedIn }
      : day.totals;
  return { ...day, spots, totals };
}

/** Applies `op` to every cached operations day of its establishment. */
export function applyOptimistic(qc: QueryClient, op: OfflineOperation): void {
  qc.setQueriesData<unknown>(
    { queryKey: establishmentKeys.area(op.establishmentId, 'operations') },
    (old: unknown) => (isOperationsDay(old) ? applyToDay(old, op) : old),
  );
}
