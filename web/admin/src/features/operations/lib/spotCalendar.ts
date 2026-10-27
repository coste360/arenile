import type { Schemas } from '@arenile/shared/api';
import type { SpotStatus } from '@arenile/shared/map';

export type SpotCalendar = Schemas['SpotCalendar'];
export type SpotCalendarDay = Schemas['SpotCalendarDay'];
export type SpotCalendarCell = Schemas['SpotCalendarCell'];
export type SpotCalendarBooking = Schemas['SpotCalendarBooking'];

/**
 * Status of a half-day of the spot calendar on the shared palette (spec
 * 30.5: free, booked, season pass, released, blocked, to collect): a booking
 * with an open balance shows "da saldare", like the map.
 */
export function cellStatus(
  cell: SpotCalendarCell,
  bookings: ReadonlyMap<string, SpotCalendarBooking>,
): SpotStatus {
  const b = cell.booking_id ? bookings.get(cell.booking_id) : undefined;
  switch (cell.status) {
    case 'free':
      return 'free';
    case 'booked':
      return b && b.balance_due > 0 ? 'to_collect' : 'booked_desk';
    case 'season_pass':
      return b && b.balance_due > 0 ? 'to_collect' : 'season';
    case 'released':
      return 'season_released';
    case 'held':
      return 'hold';
    case 'blocked':
      return 'blocked';
    default:
      return 'unavailable';
  }
}

/** Statuses of the calendar legend, in order. */
export const CALENDAR_LEGEND: readonly SpotStatus[] = [
  'free',
  'booked_desk',
  'to_collect',
  'season',
  'season_released',
  'hold',
  'blocked',
  'unavailable',
];

export interface CalendarMonth {
  /** "2026-07" */
  key: string;
  /** Days of the month 1..31 → the season day, undefined outside the season. */
  days: (SpotCalendarDay | undefined)[];
}

/** Season days grouped by month, one entry per day of the month (31 columns). */
export function calendarMonths(days: readonly SpotCalendarDay[]): CalendarMonth[] {
  const months = new Map<string, CalendarMonth>();
  for (const d of days) {
    const key = d.date.slice(0, 7);
    let m = months.get(key);
    if (!m) {
      m = { key, days: Array.from({ length: 31 }, () => undefined) };
      months.set(key, m);
    }
    m.days[Number(d.date.slice(8, 10)) - 1] = d;
  }
  return [...months.values()].sort((a, b) => a.key.localeCompare(b.key));
}

/** A free half-day for the new booking: the whole day when both halves are free. */
export function isDayFree(d: SpotCalendarDay): boolean {
  return d.am.status === 'free' || d.pm.status === 'free';
}

/** Longest stay proposed from a free day of the calendar (spec 30.5 "date successive libere"). */
export const MAX_PROPOSED_DAYS = 14;

/**
 * The stay proposed when a free day is clicked: from that day to the last
 * of the following fully free days, at most MAX_PROPOSED_DAYS long; the
 * slot is the free half when only one half of the first day is free.
 */
export function proposedStay(
  days: readonly SpotCalendarDay[],
  date: string,
): { startsOn: string; endsOn: string; slots: 'FULL' | 'AM' | 'PM' } | null {
  const i = days.findIndex((d) => d.date === date);
  const first = days[i];
  if (!first || !isDayFree(first)) return null;
  const amFree = first.am.status === 'free';
  const pmFree = first.pm.status === 'free';
  if (!(amFree && pmFree)) {
    return { startsOn: date, endsOn: date, slots: amFree ? 'AM' : 'PM' };
  }
  let end = date;
  for (let j = i + 1; j < days.length && j - i < MAX_PROPOSED_DAYS; j++) {
    const d = days[j]!;
    if (d.am.status !== 'free' || d.pm.status !== 'free') break;
    end = d.date;
  }
  return { startsOn: date, endsOn: end, slots: 'FULL' };
}
