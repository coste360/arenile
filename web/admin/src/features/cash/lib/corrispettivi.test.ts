import { describe, expect, it } from 'vitest';
import {
  daysInMonth,
  exportFileName,
  exportUrl,
  periodQuery,
  sumVat,
  totalsByMethod,
  type CorrispettiviRow,
} from './corrispettivi';

describe('periodQuery', () => {
  it('maps a day to one-day range grouped by day', () => {
    expect(periodQuery({ mode: 'day', date: '2026-08-15' })).toEqual({
      from: '2026-08-15',
      to: '2026-08-15',
      group_by: 'day',
    });
  });

  it.each([
    ['2026-02', '2026-02-28'],
    ['2028-02', '2028-02-29'],
    ['2100-02', '2100-02-28'],
    ['2000-02', '2000-02-29'],
    ['2026-04', '2026-04-30'],
    ['2026-12', '2026-12-31'],
  ])('maps month %s to its last day %s', (month, to) => {
    expect(periodQuery({ mode: 'month', month })).toEqual({
      from: `${month}-01`,
      to,
      group_by: 'month',
    });
  });

  it('rejects malformed months', () => {
    expect(() => daysInMonth('2026-13')).toThrow();
  });
});

describe('exports', () => {
  const q = { from: '2026-08-01', to: '2026-08-31', group_by: 'month' as const };
  it('builds the same-origin API URL with the format', () => {
    expect(exportUrl('est-1', q, 'csv')).toBe(
      '/api/admin/v1/establishments/est-1/fiscal/corrispettivi?from=2026-08-01&to=2026-08-31&group_by=month&format=csv',
    );
    expect(exportFileName(q, 'json')).toBe('corrispettivi_2026-08-01_2026-08-31.json');
    expect(exportFileName({ ...q, to: q.from }, 'csv')).toBe('corrispettivi_2026-08-01.csv');
  });
});

describe('totals', () => {
  const row = (
    method: CorrispettiviRow['method'],
    gross: number,
    count: number,
    vat = 2200,
  ): CorrispettiviRow => ({
    period: '2026-08-15',
    vat_rate_bp: vat,
    method,
    gross,
    taxable: 0,
    tax: 0,
    count,
  });

  it('sums rows per method in display order, refunds included', () => {
    expect(
      totalsByMethod([
        row('card', 3000, 1),
        row('cash', 12200, 2),
        row('cash', -1000, 1, 1000),
        row('cash', 1, 1),
      ]),
    ).toEqual([
      { method: 'cash', gross: 11201, count: 4 },
      { method: 'card', gross: 3000, count: 1 },
    ]);
  });

  it('sums the VAT columns', () => {
    expect(
      sumVat([
        { taxable: 10000, tax: 2200, gross: 12200 },
        { taxable: 909, tax: 91, gross: 1000 },
      ]),
    ).toEqual({ taxable: 10909, tax: 2291, gross: 13200 });
  });
});
