/**
 * Period selection of the reports page (spec 15.3): a season, a calendar
 * month or a custom inclusive range, always resolved to `{from, to}` ISO
 * dates in the establishment timezone. Pure functions, no clock: "today" is
 * passed in by the caller.
 */
import { daysInclusive } from '@arenile/shared/lib/dates';

/** Longest period the report endpoints accept (report.NewPeriod). */
export const MAX_REPORT_DAYS = 400;

export type PeriodMode = 'season' | 'month' | 'custom';

export interface ResolvedPeriod {
  from: string;
  to: string;
}

/** Season fields the picker needs (subset of Schemas['Season']). */
export interface SeasonLike {
  id: string;
  year: number;
  opens_on: string;
  closes_on: string;
}

export type PeriodSelection =
  | { mode: 'season'; seasonId: string }
  | { mode: 'month'; month: string }
  | { mode: 'custom'; from: string; to?: string };

export type PeriodError = 'incomplete' | 'reversed' | 'tooLong' | 'noSeason';

const pad = (n: number) => String(n).padStart(2, '0');

/** "2026-02" → { from: "2026-02-01", to: "2026-02-28" }. */
export function monthRange(month: string): ResolvedPeriod {
  const [y, m] = month.split('-').map(Number) as [number, number];
  const last = new Date(Date.UTC(y, m, 0)).getUTCDate();
  return { from: `${month}-01`, to: `${y}-${pad(m)}-${pad(last)}` };
}

/**
 * Season to show first: the one containing today, else the latest that has
 * already opened, else the earliest upcoming one.
 */
export function defaultSeason<T extends SeasonLike>(
  seasons: readonly T[],
  today: string,
): T | undefined {
  const current = seasons.find((s) => s.opens_on <= today && today <= s.closes_on);
  if (current) return current;
  const past = seasons
    .filter((s) => s.opens_on <= today)
    .sort((a, b) => b.opens_on.localeCompare(a.opens_on));
  if (past[0]) return past[0];
  return [...seasons].sort((a, b) => a.opens_on.localeCompare(b.opens_on))[0];
}

/** Resolves a selection to an API period, or the reason it cannot be queried. */
export function resolvePeriod(
  sel: PeriodSelection,
  seasons: readonly SeasonLike[],
): { period: ResolvedPeriod } | { error: PeriodError } {
  let period: ResolvedPeriod;
  if (sel.mode === 'season') {
    const s = seasons.find((x) => x.id === sel.seasonId);
    if (!s) return { error: 'noSeason' };
    period = { from: s.opens_on, to: s.closes_on };
  } else if (sel.mode === 'month') {
    period = monthRange(sel.month);
  } else {
    if (!sel.to) return { error: 'incomplete' };
    period = { from: sel.from, to: sel.to };
  }
  if (period.to < period.from) return { error: 'reversed' };
  if (daysInclusive(period.from, period.to) > MAX_REPORT_DAYS) return { error: 'tooLong' };
  return { period };
}

/** Series granularity of the year comparison that keeps the chart readable. */
export function comparisonGranularity(period: ResolvedPeriod): 'day' | 'week' | 'month' {
  const days = daysInclusive(period.from, period.to);
  if (days <= 31) return 'day';
  if (days <= 150) return 'week';
  return 'month';
}

/** Year the comparison is made against (server default: year of `from` − 1). */
export function compareYear(period: ResolvedPeriod): number {
  return Number(period.from.slice(0, 4)) - 1;
}

/** File name of a CSV export: "arenile_revenue_2026-06-01_2026-09-15.csv". */
export function csvFileName(name: string, period: ResolvedPeriod): string {
  return `arenile_${name}_${period.from}_${period.to}.csv`;
}
