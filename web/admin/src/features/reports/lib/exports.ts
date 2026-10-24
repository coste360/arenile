import type { ResolvedPeriod } from './period';

export const EXPORT_LISTS = ['customers', 'bookings', 'season-passes', 'payments'] as const;
export type ExportList = (typeof EXPORT_LISTS)[number];
export type ExportFileFormat = 'csv' | 'xlsx';

/** Lists filtered by the period (the others are the whole registry). */
const PERIOD_LISTS: ReadonlySet<ExportList> = new Set(['bookings', 'payments']);

/** Download URL of a list export (GET /exports/{list}, session cookie). */
export function exportListUrl(
  establishmentId: string,
  list: ExportList,
  format: ExportFileFormat,
  period: ResolvedPeriod,
): string {
  const q = new URLSearchParams({ format });
  if (PERIOD_LISTS.has(list)) {
    q.set('from', period.from);
    q.set('to', period.to);
  }
  return `/api/admin/v1/establishments/${encodeURIComponent(establishmentId)}/exports/${list}?${q.toString()}`;
}

/** Suggested file name: "arenile_bookings_2026-06-01_2026-09-15.xlsx". */
export function exportListFileName(
  list: ExportList,
  format: ExportFileFormat,
  period: ResolvedPeriod,
): string {
  const span = PERIOD_LISTS.has(list) ? `_${period.from}_${period.to}` : '';
  return `arenile_${list}${span}.${format}`;
}

/** The same export as an .xlsx sheet (ADR 0019 F): only the format changes. */
export function toXlsx(href: string): string {
  return href.replace(/([?&])format=csv\b/, '$1format=xlsx');
}
