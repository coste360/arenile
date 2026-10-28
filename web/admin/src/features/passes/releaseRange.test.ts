import { describe, expect, it } from 'vitest';
import { isoWeekday, rangeReleaseDates } from './releaseRange';

describe('rangeReleaseDates (spec 30.8)', () => {
  const pass = { starts_on: '2026-06-01', ends_on: '2026-09-15' };

  it('knows the ISO weekday', () => {
    expect(isoWeekday('2026-07-06')).toBe(1); // Monday
    expect(isoWeekday('2026-07-12')).toBe(7); // Sunday
  });

  it('covers every day of the range without weekdays', () => {
    expect(rangeReleaseDates('2026-07-10', '2026-07-12', [], pass, '2026-07-01')).toEqual([
      '2026-07-10',
      '2026-07-11',
      '2026-07-12',
    ]);
  });

  it('keeps only the chosen weekdays ("ogni lunedì di luglio")', () => {
    expect(rangeReleaseDates('2026-07-01', '2026-07-31', [1], pass, '2026-06-20')).toEqual([
      '2026-07-06',
      '2026-07-13',
      '2026-07-20',
      '2026-07-27',
    ]);
  });

  it('clamps to the pass and to today', () => {
    expect(rangeReleaseDates('2026-09-14', '2026-09-30', [], pass, '2026-07-01')).toEqual([
      '2026-09-14',
      '2026-09-15',
    ]);
    expect(rangeReleaseDates('2026-07-01', '2026-07-03', [], pass, '2026-07-03')).toEqual([
      '2026-07-03',
    ]);
  });
});
