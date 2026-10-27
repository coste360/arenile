import type { Schemas } from '@arenile/shared/api';
import type { SpotStatus } from '@arenile/shared/map';

type AvailabilityResult = Schemas['AvailabilityResult'];
type SpotAvailability = Schemas['SpotAvailability'];

/**
 * State of a spot over a range (spec 30.4 "vista a intervallo"): free on
 * every day, free on some days ("libero in parte", N days out of M), taken
 * on every day, or not sold by the desk on that range.
 */
export type RangeState = 'free' | 'partial' | 'taken' | 'unavailable';

export const RANGE_STATES: readonly RangeState[] = ['free', 'partial', 'taken', 'unavailable'];

/**
 * Palette and glyph of each range state on the shared map: "libero in parte"
 * borrows the released-day colour and glyph (a day that can be sold), the
 * range legend names it.
 */
export const RANGE_STATUS: Record<RangeState, SpotStatus> = {
  free: 'free',
  partial: 'season_released',
  taken: 'booked_desk',
  unavailable: 'unavailable',
};

export interface RangeSpot {
  spotId: string;
  state: RangeState;
  freeDays: number;
  totalDays: number;
  freeDates: string[];
}

/** State of one spot from its availability over the range. */
export function rangeState(a: SpotAvailability): RangeSpot {
  const total = a.total_days ?? 0;
  const free = a.free_days ?? (a.status === 'available' ? total : 0);
  let state: RangeState;
  if (a.status === 'available' || (total > 0 && free >= total)) state = 'free';
  else if (free > 0) state = 'partial';
  else state = 'taken';
  return {
    spotId: a.spot_id,
    state,
    freeDays: state === 'free' ? total : free,
    totalDays: total,
    freeDates: a.free_dates ?? [],
  };
}

/** Range state per spot id; spots missing from the answer are unavailable. */
export function rangeSpots(
  result: AvailabilityResult | undefined,
  spotIds: readonly string[],
): Map<string, RangeSpot> {
  const out = new Map<string, RangeSpot>();
  for (const z of result?.zones ?? []) {
    for (const a of z.spots ?? []) out.set(a.spot_id, rangeState(a));
  }
  for (const id of spotIds) {
    if (!out.has(id)) {
      out.set(id, { spotId: id, state: 'unavailable', freeDays: 0, totalDays: 0, freeDates: [] });
    }
  }
  return out;
}

/** Consecutive runs of dates ("2026-07-10".."2026-07-12") in a sorted list of free dates. */
export function dateRuns(dates: readonly string[]): { from: string; to: string }[] {
  const sorted = [...dates].sort();
  const runs: { from: string; to: string }[] = [];
  for (const d of sorted) {
    const last = runs[runs.length - 1];
    if (last && nextDay(last.to) === d) last.to = d;
    else runs.push({ from: d, to: d });
  }
  return runs;
}

function nextDay(date: string): string {
  const [y, m, d] = date.split('-').map(Number) as [number, number, number];
  return new Date(Date.UTC(y, m - 1, d + 1)).toISOString().slice(0, 10);
}
