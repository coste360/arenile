import type { Schemas } from '@arenile/shared/api';
import type { SeasonRelease } from './api';

type Slot = Schemas['Slot'];
type Slots = Schemas['Slots'];

/** Half-days of a Slots value. */
export function halves(slots: Slots): Slot[] {
  return slots === 'FULL' ? ['AM', 'PM'] : [slots];
}

/** Half-days already released (and not revoked) per date: released or resold. */
export function releasedHalves(releases: readonly SeasonRelease[]): Map<string, Set<Slot>> {
  const out = new Map<string, Set<Slot>>();
  for (const r of releases) {
    if (r.status === 'revoked') continue;
    const set = out.get(r.date) ?? new Set<Slot>();
    set.add(r.slot);
    out.set(r.date, set);
  }
  return out;
}

/** Status of a date for the calendar colours: resold wins over released. */
export function dayStatus(
  date: string,
  releases: readonly SeasonRelease[],
): 'resold' | 'released' | undefined {
  let status: 'resold' | 'released' | undefined;
  for (const r of releases) {
    if (r.date !== date || r.status === 'revoked') continue;
    if (r.status === 'resold') return 'resold';
    status = 'released';
  }
  return status;
}

export interface PassWindow {
  starts_on: string;
  ends_on: string;
  /** Slots held by the pass: a PM-only pass cannot release the morning. */
  slots: Slots;
}

/**
 * Whether `date` can be released for `slots`: inside the pass, not in the
 * past, the requested half-days held by the pass and none of them already
 * released. The server checks again (409 / 422).
 */
export function isReleasable(
  date: string,
  slots: Slots,
  pass: PassWindow,
  released: Map<string, Set<Slot>>,
  today: string,
): boolean {
  if (date < pass.starts_on || date > pass.ends_on || date < today) return false;
  const held = new Set(halves(pass.slots));
  const wanted = halves(slots);
  if (!wanted.every((h) => held.has(h))) return false;
  const already = released.get(date);
  return !wanted.some((h) => already?.has(h));
}

/** Toggles a date in a sorted selection. */
export function toggleDate(selection: readonly string[], date: string): string[] {
  return selection.includes(date)
    ? selection.filter((d) => d !== date)
    : [...selection, date].sort();
}

/** Drops the selected dates that the new slot choice makes unreleasable. */
export function pruneSelection(
  selection: readonly string[],
  slots: Slots,
  pass: PassWindow,
  released: Map<string, Set<Slot>>,
  today: string,
): string[] {
  return selection.filter((d) => isReleasable(d, slots, pass, released, today));
}

/** Max dates per request (SeasonReleaseCreate.dates maxItems). */
export const MAX_RELEASE_DATES = 120;

/** Credit of the active releases of a pass (accrued when resold, spec 7.7). */
export function releasesCredit(releases: readonly SeasonRelease[]): number {
  return releases
    .filter((r) => r.status !== 'revoked')
    .reduce((sum, r) => sum + r.credit_amount, 0);
}
