import { format, isValid, parse } from 'date-fns';
import { de, enGB, fr, it, type Locale as DateFnsLocale } from 'date-fns/locale';

/**
 * Calendar dates travel as "YYYY-MM-DD" (spec: calendar.Date, establishment
 * time zone). The UI shows them as dd/MM/yyyy. Conversions use local-midnight
 * Date objects, which is what react-day-picker works with.
 */

export const DISPLAY_DATE_FORMAT = 'dd/MM/yyyy';
const ISO_RE = /^\d{4}-\d{2}-\d{2}$/;

const LOCALES: Record<string, DateFnsLocale> = { it, en: enGB, de, fr };

/** date-fns locale for an i18n language (Italian fallback). */
export function dateFnsLocale(language: string | undefined): DateFnsLocale {
  return LOCALES[(language ?? 'it').slice(0, 2)] ?? it;
}

/** "2026-07-14" → Date at local midnight; undefined when malformed. */
export function isoToDate(value: string | null | undefined): Date | undefined {
  if (!value || !ISO_RE.test(value)) return undefined;
  const d = parse(value, 'yyyy-MM-dd', new Date());
  return isValid(d) ? d : undefined;
}

/** Date → "2026-07-14" (local calendar day). */
export function dateToIso(date: Date): string {
  return format(date, 'yyyy-MM-dd');
}

/** "2026-07-14" → "14/07/2026"; returns the input unchanged when malformed. */
export function formatIsoDate(value: string): string {
  const d = isoToDate(value);
  return d ? format(d, DISPLAY_DATE_FORMAT) : value;
}

/** "14/07/2026" → "2026-07-14"; undefined when malformed. */
export function parseDisplayDate(value: string): string | undefined {
  const d = parse(value.trim(), DISPLAY_DATE_FORMAT, new Date());
  return isValid(d) && format(d, DISPLAY_DATE_FORMAT) === value.trim() ? dateToIso(d) : undefined;
}
