import type { Schemas } from '@arenile/shared/api';
import type { SpotIndicator, SpotStatus } from '@arenile/shared/map';

export type OperationalSpot = Schemas['OperationalSpot'];
export type OperationalSpotStatus = Schemas['OperationalSpotStatus'];
export type OperationalBooking = Schemas['OperationalBooking'];
export type OperationsDay = Schemas['OperationsDay'];
export type SlotChoice = Schemas['Slots'];

/** Customer tag that marks a VIP (spec 11.3 overlay indicator). */
export const VIP_TAG = 'vip';

/**
 * Colour status of a spot in the operational view (spec 11.3 table), from
 * the server status of the slot plus the booking summary:
 *
 *   free → free; held → hold; checked_in → present;
 *   pass_holder → to_collect when the pass has an open balance, else season;
 *   released → season_released; renewal_offer → renewal_hold;
 *   blocked → blocked; closed / inactive → unavailable;
 *   booked → to_collect when the balance is open, else booked_online
 *   (channel online/api) or booked_desk.
 *
 * "Da incassare" (spec 11.3) is defined on the money, paid_amount <
 * total_amount, so it takes precedence over the season colour: the map
 * then shows the same open balances the Oggi page counts.
 *
 * Pure function: the single place that maps API statuses onto the shared
 * palette (STATUS_CLASSES / --status-* tokens), so map, legend and lists agree.
 */
export function toSpotStatus(
  status: OperationalSpotStatus,
  booking?: Pick<OperationalBooking, 'channel' | 'balance_due'> | null,
): SpotStatus {
  switch (status) {
    case 'free':
      return 'free';
    case 'held':
      return 'hold';
    case 'checked_in':
      return 'present';
    case 'pass_holder':
      return booking && booking.balance_due > 0 ? 'to_collect' : 'season';
    case 'released':
      return 'season_released';
    case 'renewal_offer':
      return 'renewal_hold';
    case 'blocked':
      return 'blocked';
    case 'closed':
    case 'inactive':
      return 'unavailable';
    case 'booked':
      if (booking && booking.balance_due > 0) return 'to_collect';
      return booking?.channel === 'desk'
        ? 'booked_desk'
        : booking
          ? 'booked_online'
          : 'booked_desk';
    default:
      return 'unavailable';
  }
}

/** Server status of a spot for the selected slot (AM/PM fall back to the day status). */
export function slotStatus(spot: OperationalSpot, slot: SlotChoice): OperationalSpotStatus {
  if (slot === 'AM') return spot.am_status ?? spot.status;
  if (slot === 'PM') return spot.pm_status ?? spot.status;
  return spot.status;
}

/** Status per spot id for the map renderer. */
export function statusesBySpot(
  spots: readonly OperationalSpot[],
  slot: SlotChoice,
): Record<string, SpotStatus> {
  const out: Record<string, SpotStatus> = {};
  for (const s of spots) out[s.spot_id] = toSpotStatus(slotStatus(s, slot), s.booking);
  return out;
}

/** Overlay indicators (extra, note, VIP) per spot id; spots without any are omitted. */
export function indicatorsBySpot(
  spots: readonly OperationalSpot[],
): Record<string, readonly SpotIndicator[]> {
  const out: Record<string, readonly SpotIndicator[]> = {};
  for (const s of spots) {
    const b = s.booking;
    if (!b) continue;
    const list: SpotIndicator[] = [];
    if (b.has_extras) list.push('extra');
    if (b.has_notes) list.push('note');
    if (b.customer_tags.some((tag) => tag.toLowerCase() === VIP_TAG)) list.push('vip');
    if (list.length > 0) out[s.spot_id] = list;
  }
  return out;
}

/** Legend order of the operational map: always every status, never only those present. */
export const OPERATIONAL_LEGEND: readonly SpotStatus[] = [
  'free',
  'booked_desk',
  'booked_online',
  'to_collect',
  'present',
  'season',
  'season_released',
  'hold',
  'renewal_hold',
  'blocked',
  'unavailable',
];

/** True when a new booking can start on this spot for the slot (free or released pass day). */
export function isBookable(status: OperationalSpotStatus): boolean {
  return status === 'free' || status === 'released';
}

/**
 * Spot ids whose label, booking code or customer name contain `query`
 * (case and accent insensitive). Empty query → empty list (no highlight).
 * Phone numbers are not in the operational payload (no personal data on
 * the map): searching by phone goes through the customers page.
 */
export function searchSpots(spots: readonly OperationalSpot[], query: string): string[] {
  const q = normalize(query.trim());
  if (q.length === 0) return [];
  const out: string[] = [];
  for (const s of spots) {
    const hay = [s.label, s.booking?.code, s.booking?.customer_display_name]
      .filter((v): v is string => !!v)
      .map(normalize);
    if (hay.some((h) => h.includes(q))) out.push(s.spot_id);
  }
  return out;
}

function normalize(v: string): string {
  return v.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();
}
