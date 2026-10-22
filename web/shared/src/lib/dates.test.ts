import { describe, expect, it } from 'vitest';
import { dateToIso, formatIsoDate, isoToDate, parseDisplayDate } from './dates';

describe('calendar dates', () => {
  it('converts between ISO and dd/MM/yyyy', () => {
    expect(formatIsoDate('2026-07-04')).toBe('04/07/2026');
    expect(parseDisplayDate('04/07/2026')).toBe('2026-07-04');
    expect(parseDisplayDate('31/02/2026')).toBeUndefined();
    expect(formatIsoDate('nope')).toBe('nope');
  });

  it('round-trips through local Date objects', () => {
    const d = isoToDate('2026-03-29'); // DST change in Europe/Rome
    expect(d && dateToIso(d)).toBe('2026-03-29');
    expect(isoToDate('2026-13-01')).toBeUndefined();
  });
});
