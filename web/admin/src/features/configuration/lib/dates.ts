/** Calendar-date helpers on "YYYY-MM-DD" strings (no time zone involved). */

function toUtc(date: string): number {
  const [y, m, d] = date.split('-').map(Number) as [number, number, number];
  return Date.UTC(y, m - 1, d);
}

const DAY = 86_400_000;

/** Whole days from `a` to `b` (b - a). */
export function daysBetween(a: string, b: string): number {
  return Math.round((toUtc(b) - toUtc(a)) / DAY);
}

/** `date` plus `n` days. */
export function addDays(date: string, n: number): string {
  return new Date(toUtc(date) + n * DAY).toISOString().slice(0, 10);
}

export interface DateRange {
  from: string;
  to: string;
}

/** True when two inclusive ranges share at least one day. */
export function rangesOverlap(a: DateRange, b: DateRange): boolean {
  return a.from <= b.to && b.from <= a.to;
}

/** Localised short date for display. */
export function formatDate(date: string, locale: string): string {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) return date;
  return new Intl.DateTimeFormat(locale, {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    timeZone: 'UTC',
  }).format(new Date(toUtc(date)));
}

/** Localised day + month (no year), e.g. "15 ago". */
export function formatDayMonth(date: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, {
    day: 'numeric',
    month: 'short',
    timeZone: 'UTC',
  }).format(new Date(toUtc(date)));
}

/** First day of every month between from and to (inclusive), for timeline ticks. */
export function monthStarts(from: string, to: string): string[] {
  const out: string[] = [];
  const [y, m] = from.split('-').map(Number) as [number, number];
  let year = y;
  let month = m;
  for (let i = 0; i < 36; i++) {
    const d = `${year}-${String(month).padStart(2, '0')}-01`;
    if (d > to) break;
    if (d >= from) out.push(d);
    month += 1;
    if (month > 12) {
      month = 1;
      year += 1;
    }
  }
  return out;
}
