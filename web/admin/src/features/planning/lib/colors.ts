import type { SpotStatus } from '@arenile/shared/map';
import type { PlanningEntry } from './grid';

/**
 * Palette status of a planning bar, consistent with the operational map
 * (spec 11.3 colours): holds and bookings waiting for payment or approval
 * are "hold", checked-in bookings "present", open balances "to_collect",
 * confirmed bookings by channel; season passes, released days and manual
 * blocks keep their own colours.
 */
export function entryStatus(entry: PlanningEntry): SpotStatus {
  switch (entry.kind) {
    case 'hold':
      return 'hold';
    case 'season':
      return 'season';
    case 'release':
      return 'season_released';
    case 'block':
      return 'blocked';
    case 'booking':
      break;
    default:
      return 'unavailable';
  }
  switch (entry.status) {
    case 'held':
    case 'pending_payment':
    case 'awaiting_approval':
    case 'draft':
      return 'hold';
    case 'checked_in':
      return 'present';
    default:
      if (entry.balance_due) return 'to_collect';
      return entry.channel === 'desk' || !entry.channel ? 'booked_desk' : 'booked_online';
  }
}

/** Legend of the planning: the statuses a bar can take. */
export const PLANNING_LEGEND: readonly SpotStatus[] = [
  'booked_desk',
  'booked_online',
  'to_collect',
  'present',
  'hold',
  'season',
  'season_released',
  'blocked',
];
