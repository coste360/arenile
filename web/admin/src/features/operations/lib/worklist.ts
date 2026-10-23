import { slotStatus, type OperationalBooking, type OperationsDay } from './status';

export interface BookingRow {
  booking: OperationalBooking;
  spots: string[];
}

/** Bookings on the map (one row per booking, with its spot labels) matching `pick`. */
export function bookingRows(
  day: OperationsDay,
  pick: (status: string, b: OperationalBooking) => boolean,
): BookingRow[] {
  const rows = new Map<string, BookingRow>();
  for (const s of day.spots) {
    const b = s.booking;
    if (!b || !pick(slotStatus(s, day.slot), b)) continue;
    const row = rows.get(b.booking_id) ?? { booking: b, spots: [] };
    row.spots.push(s.label);
    rows.set(b.booking_id, row);
  }
  return [...rows.values()].sort((a, b) => a.booking.code.localeCompare(b.booking.code));
}
