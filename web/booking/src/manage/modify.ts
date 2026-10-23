import type { Schemas } from '@arenile/shared/api';
import type { ManageBooking } from '../api/public';

/**
 * Selection of a modification that keeps the booking's lines and changes
 * the dates (pure). Seat lines keep their zone; the server keeps the
 * assigned spot when it is free on the new dates (spec 10.3).
 */
export function modifySelection(
  booking: ManageBooking,
  dates: { from: string; to: string },
  formula: Schemas['Formula'],
): Schemas['ManageModifyRequest'] {
  const s = booking.summary;
  return {
    starts_on: dates.from,
    ends_on: dates.to,
    slots: s.slots,
    formula,
    lines: s.lines.map((l) => ({
      pack_id: l.pack_id,
      ...(l.zone_id ? { zone_id: l.zone_id } : {}),
      ...(l.quantity > 1 ? { quantity: l.quantity } : {}),
      ...(l.extras.length
        ? { extras: l.extras.map((e) => ({ extra_id: e.extra_id, quantity: e.quantity })) }
        : {}),
    })),
    ...(s.extras.length
      ? { extras: s.extras.map((e) => ({ extra_id: e.extra_id, quantity: e.quantity })) }
      : {}),
  };
}
