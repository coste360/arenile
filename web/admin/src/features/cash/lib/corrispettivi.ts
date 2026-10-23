/**
 * Pure helpers of the corrispettivi page (spec 14.2, driver "export"): the
 * period picked by the user mapped to the API query, and integer sums of the
 * report rows by payment method. VAT splitting (scorporo) is done by the
 * server from the price snapshot; the client only adds integers.
 */
import type { Schemas } from '@arenile/shared/api';
import { PAYMENT_METHODS, type PaymentMethod } from './totals';

export type CorrispettiviReport = Schemas['CorrispettiviReport'];
export type CorrispettiviRow = Schemas['CorrispettiviRow'];
export type GroupBy = 'day' | 'month';

export interface CorrispettiviQuery {
  from: string;
  to: string;
  group_by: GroupBy;
}

/** Period picked on the page: one day (YYYY-MM-DD) or one month (YYYY-MM). */
export type Period = { mode: 'day'; date: string } | { mode: 'month'; month: string };

const MONTH_RE = /^(\d{4})-(0[1-9]|1[0-2])$/;

/** Last day of a month, "2026-02" → 28 (Gregorian leap years). */
export function daysInMonth(month: string): number {
  const m = MONTH_RE.exec(month);
  if (!m) throw new Error(`invalid month ${month}`);
  const year = Number(m[1]);
  const mm = Number(m[2]);
  if (mm === 2) return (year % 4 === 0 && year % 100 !== 0) || year % 400 === 0 ? 29 : 28;
  return [4, 6, 9, 11].includes(mm) ? 30 : 31;
}

/** API query of a period: a day is grouped by day, a month by month. */
export function periodQuery(p: Period): CorrispettiviQuery {
  if (p.mode === 'day') return { from: p.date, to: p.date, group_by: 'day' };
  const last = String(daysInMonth(p.month)).padStart(2, '0');
  return { from: `${p.month}-01`, to: `${p.month}-${last}`, group_by: 'month' };
}

/**
 * Relative URL of the export (same-origin, session cookie): the browser
 * downloads it directly, so the file is exactly what the server produced.
 */
export function exportUrl(
  establishmentId: string,
  q: CorrispettiviQuery,
  format: 'csv' | 'json',
): string {
  const params = new URLSearchParams({ from: q.from, to: q.to, group_by: q.group_by, format });
  return `/api/admin/v1/establishments/${encodeURIComponent(establishmentId)}/fiscal/corrispettivi?${params.toString()}`;
}

/** Suggested file name of an export. */
export function exportFileName(q: CorrispettiviQuery, format: 'csv' | 'json'): string {
  const span = q.from === q.to ? q.from : `${q.from}_${q.to}`;
  return `corrispettivi_${span}.${format}`;
}

export interface MethodRow {
  method: PaymentMethod;
  gross: number;
  count: number;
}

/** Gross takings and number of payments per method, in display order (refunds are negative rows). */
export function totalsByMethod(rows: readonly CorrispettiviRow[]): MethodRow[] {
  const acc = new Map<PaymentMethod, MethodRow>();
  for (const r of rows) {
    const cur = acc.get(r.method) ?? { method: r.method, gross: 0, count: 0 };
    cur.gross += r.gross;
    cur.count += r.count;
    acc.set(r.method, cur);
  }
  return PAYMENT_METHODS.flatMap((m) => {
    const row = acc.get(m);
    return row ? [row] : [];
  });
}

export interface VatTotals {
  taxable: number;
  tax: number;
  gross: number;
}

/** Column totals of the VAT table. */
export function sumVat(
  totals: readonly { taxable: number; tax: number; gross: number }[],
): VatTotals {
  return totals.reduce<VatTotals>(
    (acc, t) => ({
      taxable: acc.taxable + t.taxable,
      tax: acc.tax + t.tax,
      gross: acc.gross + t.gross,
    }),
    { taxable: 0, tax: 0, gross: 0 },
  );
}
