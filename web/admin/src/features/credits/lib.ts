import type { Schemas } from '@arenile/shared/api';

type BookingSummary = Schemas['BookingSummary'];

/** Bookings of the customer that can still take a payment. */
export function payableBookings(bookings: readonly BookingSummary[]): BookingSummary[] {
  return bookings.filter(
    (b) =>
      b.balance_due > 0 &&
      (b.status === 'confirmed' ||
        b.status === 'checked_in' ||
        b.status === 'pending_payment' ||
        b.status === 'awaiting_approval'),
  );
}
