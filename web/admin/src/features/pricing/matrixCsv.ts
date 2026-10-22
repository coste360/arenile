import { centsToEuroInput } from '../configuration/lib/money';
import type { PriceMatrix } from './api';

/**
 * Client-side CSV of the price matrix: one row per pack × price category, one
 * column per rate period. Amounts are written as exact decimal strings from
 * integer cents (never via floating point) with the locale decimal separator;
 * the field separator is ";" when the decimal separator is "," (what
 * spreadsheet apps expect in it/de/fr), "," otherwise.
 */
export function matrixToCsv(
  matrix: PriceMatrix,
  opts: {
    packName: (id: string) => string;
    locale: string;
    headers: { pack: string; category: string };
    noPrice: string;
  },
): string {
  const decimal = centsToEuroInput(150, opts.locale).includes(',') ? ',' : '.';
  const sep = decimal === ',' ? ';' : ',';
  const cell = (v: string) =>
    v.includes(sep) || /["\n\r]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v;
  const index = new Map(
    matrix.cells.map((c) => [`${c.pack_id}\u0000${c.price_category}\u0000${c.rate_period}`, c]),
  );
  const rows: string[][] = [[opts.headers.pack, opts.headers.category, ...matrix.rate_periods]];
  for (const pack of matrix.pack_ids) {
    for (const cat of matrix.categories) {
      rows.push([
        opts.packName(pack),
        cat,
        ...matrix.rate_periods.map((period) => {
          const c = index.get(`${pack}\u0000${cat}\u0000${period}`);
          if (!c) return '';
          return c.priced ? centsToEuroInput(c.price, opts.locale) : opts.noPrice;
        }),
      ]);
    }
  }
  return rows.map((r) => r.map(cell).join(sep)).join('\r\n') + '\r\n';
}

/** Triggers a download of `content` as a UTF-8 CSV (with BOM so spreadsheets detect the encoding). */
export function downloadCsv(filename: string, content: string): void {
  const blob = new Blob(['﻿', content], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}
