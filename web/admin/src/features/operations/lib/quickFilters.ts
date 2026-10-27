import { slotStatus, type OperationalSpot, type SlotChoice } from './status';

/**
 * Quick filters of the operational map (spec 30.4): they highlight the
 * spots of today's arrivals, departures, open balances, guests present,
 * one agreement (or partner) or one group; the other spots are dimmed.
 */
export type QuickFilterKind =
  'arrivals' | 'departures' | 'to_collect' | 'present' | 'agreement' | 'group';

export const SIMPLE_FILTERS: readonly QuickFilterKind[] = [
  'arrivals',
  'departures',
  'to_collect',
  'present',
];

export interface QuickFilter {
  kind: QuickFilterKind;
  /** Agreement (or partner) id, group name. */
  value?: string;
}

const TAKEN = new Set(['booked', 'checked_in', 'pass_holder', 'held', 'renewal_offer']);

/** Spot ids matching `filter` for the slot shown (map order). */
export function filterSpots(
  spots: readonly OperationalSpot[],
  slot: SlotChoice,
  filter: QuickFilter | null,
): string[] {
  if (!filter) return [];
  const out: string[] = [];
  for (const s of spots) {
    const b = s.booking;
    const st = slotStatus(s, slot);
    if (!b || !TAKEN.has(st)) continue;
    let hit = false;
    switch (filter.kind) {
      case 'arrivals':
        hit = !!b.is_first_day;
        break;
      case 'departures':
        hit = !!b.is_last_day;
        break;
      case 'to_collect':
        hit = b.balance_due > 0;
        break;
      case 'present':
        hit = st === 'checked_in';
        break;
      case 'agreement':
        hit =
          !!filter.value && (b.agreement?.id === filter.value || b.partner?.id === filter.value);
        break;
      case 'group':
        hit = !!filter.value && b.group_name === filter.value;
        break;
    }
    if (hit) out.push(s.spot_id);
  }
  return out;
}

export interface FilterOption {
  value: string;
  label: string;
}

/** Agreements and partners present on the day, for the "Convenzione" filter. */
export function agreementOptions(spots: readonly OperationalSpot[]): FilterOption[] {
  const seen = new Map<string, string>();
  for (const s of spots) {
    const b = s.booking;
    if (b?.agreement && !seen.has(b.agreement.id)) seen.set(b.agreement.id, b.agreement.name);
    if (b?.partner && !b.agreement && !seen.has(b.partner.id))
      seen.set(b.partner.id, b.partner.name);
  }
  return [...seen].map(([value, label]) => ({ value, label })).sort(byLabel);
}

/** Groups present on the day, for the "Gruppo" filter. */
export function groupOptions(spots: readonly OperationalSpot[]): FilterOption[] {
  const seen = new Set<string>();
  for (const s of spots) if (s.booking?.group_name) seen.add(s.booking.group_name);
  return [...seen].map((g) => ({ value: g, label: g })).sort(byLabel);
}

function byLabel(a: FilterOption, b: FilterOption): number {
  return a.label.localeCompare(b.label, 'it');
}

/**
 * What the map highlights: the search matches, the quick filter matches, or
 * the spots matching both when both are active.
 */
export function combineHighlights(
  search: readonly string[] | null,
  filtered: readonly string[] | null,
): string[] {
  if (search && filtered) {
    const keep = new Set(filtered);
    return search.filter((id) => keep.has(id));
  }
  return [...(search ?? filtered ?? [])];
}
