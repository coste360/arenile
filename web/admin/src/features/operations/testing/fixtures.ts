import type { OperationalBooking, OperationalSpot, OperationsDay } from '../lib/status';

/** Test fixtures of the operations feature (tests only, never shipped). */

export function opBooking(overrides: Partial<OperationalBooking> = {}): OperationalBooking {
  return {
    booking_id: 'b1',
    line_id: 'l1',
    code: 'AR-0001',
    status: 'confirmed',
    channel: 'desk',
    customer_id: 'c1',
    customer_display_name: 'Rossi M.',
    customer_tags: [],
    starts_on: '2026-07-14',
    ends_on: '2026-07-14',
    slots: 'FULL',
    is_season_pass: false,
    pack_codes: ['STD'],
    total_amount: 3000,
    paid_amount: 3000,
    balance_due: 0,
    has_notes: false,
    has_extras: false,
    ...overrides,
  };
}

export function opSpot(
  id: string,
  label: string,
  status: OperationalSpot['status'],
  overrides: Partial<OperationalSpot> = {},
): OperationalSpot {
  return { spot_id: id, map_id: 'm1', zone_id: 'z1', label, status, ...overrides };
}

export function opDay(
  spots: OperationalSpot[],
  overrides: Partial<OperationsDay> = {},
): OperationsDay {
  const occupied = spots.filter((s) => s.status !== 'free').length;
  return {
    date: '2026-07-14',
    slot: 'FULL',
    closed: false,
    spots,
    pending_assignments: [],
    pools: [],
    totals: {
      spots: spots.length,
      free: spots.length - occupied,
      occupied,
      arrivals: 1,
      departures: 0,
      checked_in: 0,
      balance_due_amount: 0,
      pending_assignments: 0,
    },
    ...overrides,
  };
}
