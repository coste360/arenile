/**
 * Season release by range (spec 30.8): from-to, optionally only some
 * weekdays ("ogni lunedì di luglio"). ISO weekdays: 1 = Monday ... 7 = Sunday.
 */
export const ISO_WEEKDAYS = [1, 2, 3, 4, 5, 6, 7] as const;

/** ISO weekday (1 = Monday ... 7 = Sunday) of a "YYYY-MM-DD" date. */
export function isoWeekday(date: string): number {
  const [y, m, d] = date.split('-').map(Number) as [number, number, number];
  const day = new Date(Date.UTC(y, m - 1, d)).getUTCDay();
  return day === 0 ? 7 : day;
}

/**
 * The days a range release covers: every date from `from` to `to` (clamped
 * to the pass and to today) on the chosen weekdays (none = every day). The
 * server computes the same and skips days already released or closed.
 */
export function rangeReleaseDates(
  from: string,
  to: string,
  weekdays: readonly number[],
  window: { starts_on: string; ends_on: string },
  today: string,
): string[] {
  const start = [from, window.starts_on, today].sort()[2]!;
  const end = to < window.ends_on ? to : window.ends_on;
  const out: string[] = [];
  const days = new Set(weekdays);
  const [y, m, d] = start.split('-').map(Number) as [number, number, number];
  for (let i = 0; i < 400; i++) {
    const date = new Date(Date.UTC(y, m - 1, d + i)).toISOString().slice(0, 10);
    if (date > end) break;
    if (days.size === 0 || days.has(isoWeekday(date))) out.push(date);
  }
  return out;
}
