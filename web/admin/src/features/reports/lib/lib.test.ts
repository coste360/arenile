import { describe, expect, it } from 'vitest';
import {
  deltaLabel,
  deltaTone,
  formatPointsDelta,
  formatRatioBP,
  formatSignedBP,
  metricsByKey,
  type ComparisonMetric,
} from './delta';
import {
  comparisonGranularity,
  compareYear,
  csvFileName,
  defaultSeason,
  monthRange,
  resolvePeriod,
} from './period';
import { revenueGroupAllowed } from './revenue';

const seasons = [
  { id: 's25', year: 2025, opens_on: '2025-05-15', closes_on: '2025-09-30' },
  { id: 's26', year: 2026, opens_on: '2026-05-15', closes_on: '2026-09-30' },
  { id: 's27', year: 2027, opens_on: '2027-05-15', closes_on: '2027-09-30' },
];

describe('monthRange', () => {
  it.each([
    ['2026-02', '2026-02-01', '2026-02-28'],
    ['2028-02', '2028-02-01', '2028-02-29'],
    ['2026-12', '2026-12-01', '2026-12-31'],
  ])('%s', (month, from, to) => {
    expect(monthRange(month)).toEqual({ from, to });
  });
});

describe('defaultSeason', () => {
  it.each([
    ['2026-07-01', 's26'],
    ['2026-11-01', 's26'],
    ['2024-01-01', 's25'],
  ])('on %s picks %s', (today, id) => {
    expect(defaultSeason(seasons, today)?.id).toBe(id);
  });
  it('is undefined without seasons', () => {
    expect(defaultSeason([], '2026-07-01')).toBeUndefined();
  });
});

describe('resolvePeriod', () => {
  it('resolves a season, a month and a custom range', () => {
    expect(resolvePeriod({ mode: 'season', seasonId: 's26' }, seasons)).toEqual({
      period: { from: '2026-05-15', to: '2026-09-30' },
    });
    expect(resolvePeriod({ mode: 'month', month: '2026-07' }, seasons)).toEqual({
      period: { from: '2026-07-01', to: '2026-07-31' },
    });
    expect(
      resolvePeriod({ mode: 'custom', from: '2026-07-01', to: '2026-07-10' }, seasons),
    ).toEqual({ period: { from: '2026-07-01', to: '2026-07-10' } });
  });
  it.each([
    [{ mode: 'season', seasonId: 'nope' } as const, 'noSeason'],
    [{ mode: 'custom', from: '2026-07-01' } as const, 'incomplete'],
    [{ mode: 'custom', from: '2026-07-10', to: '2026-07-01' } as const, 'reversed'],
    [{ mode: 'custom', from: '2026-01-01', to: '2027-02-05' } as const, 'tooLong'],
  ])('rejects %o with %s', (sel, error) => {
    expect(resolvePeriod(sel, seasons)).toEqual({ error });
  });
  it('accepts exactly 400 days', () => {
    expect(
      resolvePeriod({ mode: 'custom', from: '2026-01-01', to: '2027-02-04' }, seasons),
    ).toEqual({ period: { from: '2026-01-01', to: '2027-02-04' } });
  });
});

describe('comparison helpers', () => {
  it.each([
    ['2026-07-01', '2026-07-31', 'day'],
    ['2026-06-01', '2026-08-31', 'week'],
    ['2026-05-15', '2026-09-30', 'week'],
    ['2026-01-01', '2026-12-31', 'month'],
  ])('%s..%s uses %s', (from, to, g) => {
    expect(comparisonGranularity({ from, to })).toBe(g);
  });
  it('compares with the year before from', () => {
    expect(compareYear({ from: '2026-05-15', to: '2026-09-30' })).toBe(2025);
  });
  it('names the CSV file after the report and period', () => {
    expect(csvFileName('revenue', { from: '2026-06-01', to: '2026-06-30' })).toBe(
      'arenile_revenue_2026-06-01_2026-06-30.csv',
    );
  });
});

const metric = (m: Partial<ComparisonMetric>): ComparisonMetric => ({
  metric: 'bookings',
  unit: 'count',
  current: 0,
  previous: 0,
  delta: 0,
  ...m,
});

describe('deltas', () => {
  it.each([
    [metric({ delta: 5 }), 'positive'],
    [metric({ delta: -5 }), 'negative'],
    [metric({ delta: 0 }), 'neutral'],
    [metric({ metric: 'cancellations', delta: 3 }), 'negative'],
    [metric({ metric: 'no_shows', delta: -3 }), 'positive'],
  ])('tone of %o is %s', (m, tone) => {
    expect(deltaTone(m)).toBe(tone);
  });
  it('indexes metrics by key', () => {
    const byKey = metricsByKey([metric({ metric: 'no_shows', current: 2 })]);
    expect(byKey.no_shows?.current).toBe(2);
    expect(byKey.bookings).toBeUndefined();
  });
  it.each([
    [1250, '+12,5%'],
    [-33, '−0,3%'],
    [0, '0,0%'],
    [1234567, '+12.345,6%'],
  ])('formatSignedBP(%i) = %s', (bp, text) => {
    expect(formatSignedBP(bp)).toBe(text);
  });
  it('formats ratios and percentage points', () => {
    expect(formatRatioBP(6234)).toBe('62,3%');
    expect(formatRatioBP(0)).toBe('0,0%');
    expect(formatPointsDelta(450)).toBe('+4,5 p.p.');
    expect(formatPointsDelta(-120)).toBe('−1,2 p.p.');
  });
  it('labels a change from 0 as new, but 0 → 0 as no change', () => {
    expect(deltaLabel(metric({ current: 4, previous: 0, delta: 4 }), 'Nuovo')).toBe('Nuovo');
    expect(deltaLabel(metric({ current: 0, previous: 0, delta: 0 }), 'Nuovo')).toBe('0,0%');
    expect(deltaLabel(metric({ current: 5, previous: 4, delta: 1, delta_bp: 2500 }), 'Nuovo')).toBe(
      '+25,0%',
    );
  });
});

describe('revenueGroupAllowed', () => {
  it.each([
    ['payment_method', 'collected', true],
    ['payment_method', 'booked', false],
    ['pack', 'booked', true],
    ['extra', 'collected', false],
    ['vat_rate', 'booked', true],
    ['day', 'collected', true],
  ] as const)('%s with %s: %s', (g, b, ok) => {
    expect(revenueGroupAllowed(g, b)).toBe(ok);
  });
});
