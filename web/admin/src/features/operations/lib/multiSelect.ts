import type { OperationsPermissions } from './permissions';
import { isBookable, slotStatus, type OperationalSpot, type SlotChoice } from './status';

/**
 * Multi-selection on the operational map (ADR 0019 C): the operator selects
 * several spots (tap in selection mode, Shift/Ctrl-click, lasso) and books
 * them as one group booking or blocks them together. Only spots that are
 * bookable for the slot shown can be selected; the server checks again
 * (DB constraints, all or nothing).
 */

/** Ids of the spots that can be selected for the slot shown. */
export function selectableSpotIds(
  spots: readonly OperationalSpot[],
  slot: SlotChoice,
): ReadonlySet<string> {
  const out = new Set<string>();
  for (const s of spots) if (isBookable(slotStatus(s, slot))) out.add(s.spot_id);
  return out;
}

/** Actions of the selection bar. */
export interface SelectionActions {
  /** One booking with a line per spot (bookings.write). */
  groupBooking: boolean;
  /** Block every selected spot (operations.run; only free spots, not released pass days). */
  block: boolean;
}

export function selectionActions(
  selected: readonly OperationalSpot[],
  slot: SlotChoice,
  perms: OperationsPermissions,
): SelectionActions {
  const n = selected.length;
  return {
    groupBooking: perms.canWriteBookings && n > 0,
    block: perms.canRun && n > 0 && selected.every((s) => slotStatus(s, slot) === 'free'),
  };
}

/** Whether the map offers the selection mode at all. */
export function canMultiSelect(perms: OperationsPermissions): boolean {
  return perms.canWriteBookings || perms.canRun;
}
