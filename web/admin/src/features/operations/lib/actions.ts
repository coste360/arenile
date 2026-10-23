import type { OperationsPermissions } from './permissions';
import { isBookable, slotStatus, type OperationalSpot, type SlotChoice } from './status';

/** Actions a spot offers right now (pure: drives the buttons and the tests). */
export interface SpotActions {
  checkIn: boolean;
  newBooking: boolean;
  releaseDay: boolean;
  revokeRelease: boolean;
  move: boolean;
  collect: boolean;
  openDetail: boolean;
}

/**
 * Actions of a spot for the day shown. `day` (shown) and `today` are ISO
 * dates: check-in is offered only when the day shown is today, since the
 * server refuses it outside the stay (422 booking_outside_period); without
 * them the day is assumed to be today.
 */
export function spotActions(
  spot: OperationalSpot,
  slot: SlotChoice,
  perms: OperationsPermissions,
  day?: { date: string; today: string },
): SpotActions {
  const status = slotStatus(spot, slot);
  const b = spot.booking;
  const isToday = !day || day.date === day.today;
  return {
    checkIn: perms.canRun && isToday && status === 'booked' && b?.status === 'confirmed',
    newBooking: perms.canWriteBookings && isBookable(status),
    releaseDay: perms.canWriteBookings && status === 'pass_holder' && !!b?.is_season_pass,
    revokeRelease: perms.canWriteBookings && status === 'released' && !!spot.release_id && !!b,
    move:
      perms.canWriteBookings &&
      !!b?.line_id &&
      !b.is_season_pass &&
      (status === 'booked' || status === 'checked_in' || status === 'held'),
    collect: perms.canWriteBookings && !!b && b.balance_due > 0,
    openDetail: !!b,
  };
}
