/**
 * Calendar dates travel as "YYYY-MM-DD" (spec: calendar.Date, establishment
 * time zone). The UI shows them as dd/MM/yyyy. Conversions use local-midnight
 * Date objects, which is what react-day-picker works with.
 *
 * Pure string/Date arithmetic on purpose: this module is in the first-paint
 * bundle of the booking page, date-fns stays in the lazily loaded calendar.
 */

export const DISPLAY_DATE_FORMAT = 'dd/MM/yyyy';
const ISO_RE = /^(\d{4})-(\d{2})-(\d{2})$/;
const DISPLAY_RE = /^(\d{2})\/(\d{2})\/(\d{4})$/;

const pad = (n: number) => String(n).padStart(2, '0');

/** Local-midnight Date for a real calendar day; undefined for 2026-02-30. */
function localDate(y: number, m: number, d: number): Date | undefined {
  const date = new Date(y, m - 1, d);
  return date.getFullYear() === y && date.getMonth() === m - 1 && date.getDate() === d
    ? date
    : undefined;
}

/** "2026-07-14" → Date at local midnight; undefined when malformed. */
export function isoToDate(value: string | null | undefined): Date | undefined {
  const m = value ? ISO_RE.exec(value) : null;
  return m ? localDate(Number(m[1]), Number(m[2]), Number(m[3])) : undefined;
}

/** Date → "2026-07-14" (local calendar day). */
export function dateToIso(date: Date): string {
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

/** "2026-07-14" → "14/07/2026"; returns the input unchanged when malformed. */
export function formatIsoDate(value: string): string {
  const m = ISO_RE.exec(value);
  return m && localDate(Number(m[1]), Number(m[2]), Number(m[3]))
    ? `${m[3]}/${m[2]}/${m[1]}`
    : value;
}

/** "14/07/2026" → "2026-07-14"; undefined when malformed. */
export function parseDisplayDate(value: string): string | undefined {
  const m = DISPLAY_RE.exec(value.trim());
  if (!m) return undefined;
  const d = localDate(Number(m[3]), Number(m[2]), Number(m[1]));
  return d ? dateToIso(d) : undefined;
}

/** Calendar days between two ISO dates, inclusive ("2026-07-14".."2026-07-16" → 3); 0 when malformed. */
export function daysInclusive(from: string, to: string): number {
  const a = isoToDate(from);
  const b = isoToDate(to);
  if (!a || !b) return 0;
  const utc = (d: Date) => Date.UTC(d.getFullYear(), d.getMonth(), d.getDate());
  return Math.round((utc(b) - utc(a)) / 86_400_000) + 1;
}

/** Later of two ISO dates (lexicographic order matches chronological order). */
export function maxIso(a: string, b: string): string {
  return a > b ? a : b;
}
