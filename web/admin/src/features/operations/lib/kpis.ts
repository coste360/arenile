import type { Schemas } from '@arenile/shared/api';
import type { SpotStatus } from '@arenile/shared/map';
import {
  OPERATIONAL_LEGEND,
  slotStatus,
  toSpotStatus,
  type OperationsDay,
  type SlotChoice,
} from './status';

type Payment = Schemas['Payment'];

/** Key figures of the "Oggi" page (spec 15.1), derived from the operations day. */
export interface DayKpis {
  /** Occupied spots / bookable spots, 0..100 (integer percent). */
  occupancyPercent: number;
  occupied: number;
  spots: number;
  free: number;
  arrivals: number;
  departures: number;
  checkedIn: number;
  /** Confirmed bookings on the map not yet checked in (distinct bookings). */
  pendingCheckIns: number;
  /** Season-pass spots in use today (not released). */
  passHolders: number;
  /** Season-pass days released for today (resellable or resold). */
  released: number;
  pendingAssignments: number;
  /** Open balances of today's bookings, cents. */
  balanceDue: number;
  /** Spots per colour status, for the distribution chart. */
  byStatus: Partial<Record<SpotStatus, number>>;
}

/** Pure computation of the KPIs from the day payload (no clock, no I/O). */
export function computeDayKpis(day: OperationsDay, slot: SlotChoice = day.slot): DayKpis {
  const pending = new Set<string>();
  const byStatus: Partial<Record<SpotStatus, number>> = {};
  let passHolders = 0;
  let released = 0;
  for (const s of day.spots) {
    const status = slotStatus(s, slot);
    const colour = toSpotStatus(status, s.booking);
    byStatus[colour] = (byStatus[colour] ?? 0) + 1;
    if (status === 'pass_holder') passHolders += 1;
    if (status === 'released') released += 1;
    if (status === 'booked' && s.booking?.status === 'confirmed' && !s.booking.is_season_pass) {
      pending.add(s.booking.booking_id);
    }
  }
  const { totals } = day;
  return {
    occupancyPercent: totals.spots > 0 ? Math.round((totals.occupied * 100) / totals.spots) : 0,
    occupied: totals.occupied,
    spots: totals.spots,
    free: totals.free,
    arrivals: totals.arrivals,
    departures: totals.departures,
    checkedIn: totals.checked_in,
    pendingCheckIns: pending.size,
    passHolders,
    released,
    pendingAssignments: totals.pending_assignments,
    balanceDue: totals.balance_due_amount,
    byStatus,
  };
}

/** Net takings in cents: succeeded incoming payments minus succeeded refunds. */
export function netTakings(payments: readonly Payment[]): number {
  let total = 0;
  for (const p of payments) {
    if (p.status !== 'succeeded') continue;
    total += p.direction === 'refund' ? -Math.abs(p.amount) : p.amount;
  }
  return total;
}

/** Rows of the chart: statuses in legend order, empty ones left out. */
export function chartRows(byStatus: Partial<Record<SpotStatus, number>>) {
  return OPERATIONAL_LEGEND.filter((s) => (byStatus[s] ?? 0) > 0).map((s) => ({
    status: s,
    count: byStatus[s] ?? 0,
  }));
}
