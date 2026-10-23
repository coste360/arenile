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

export function spotActions(
  spot: OperationalSpot,
  slot: SlotChoice,
  perms: OperationsPermissions,
): SpotActions {
  const status = slotStatus(spot, slot);
  const b = spot.booking;
  return {
    checkIn: perms.canRun && status === 'booked' && b?.status === 'confirmed',
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
