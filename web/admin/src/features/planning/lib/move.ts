import type { Schemas } from '@arenile/shared/api';
import { addDays } from '../../operations/lib/dates';

type BookingDetail = Schemas['BookingDetail'];
export type BookingPatch = Schemas['BookingPatch'];

export interface MoveIntent {
  /** Whole-day shift of the booking (0 = same dates). */
  dayDelta: number;
  sourceSpotId: string;
  targetSpotId: string;
}

/**
 * PATCH body for a drag on the planning: shifts the whole booking by
 * `dayDelta` days and, when the spot changes, re-sends every line with the
 * dragged line's spot replaced (BookingPatch.lines replaces all lines; the
 * zone is deduced from the spot for seat zones). Null when nothing changes
 * or the dragged spot is not one of the booking's lines.
 */
export function buildMovePatch(detail: BookingDetail, intent: MoveIntent): BookingPatch | null {
  const patch: BookingPatch = {};
  if (intent.dayDelta !== 0) {
    patch.starts_on = addDays(detail.starts_on, intent.dayDelta);
    patch.ends_on = addDays(detail.ends_on, intent.dayDelta);
  }
  if (intent.targetSpotId !== intent.sourceSpotId) {
    if (!detail.lines.some((l) => l.spot_id === intent.sourceSpotId)) return null;
    patch.lines = detail.lines.map((l) => {
      const spot = l.spot_id === intent.sourceSpotId ? intent.targetSpotId : l.spot_id;
      return {
        pack_id: l.pack_id,
        ...(spot ? { spot_id: spot } : { zone_id: l.zone_id }),
        quantity: l.quantity,
        extras: l.extras.map((e) => ({ extra_id: e.extra_id, quantity: e.quantity })),
      };
    });
  }
  return patch.starts_on || patch.lines ? patch : null;
}
