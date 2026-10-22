import { describe, expect, it } from 'vitest';
import type { PriceMatrix } from './api';
import { calendarMonths, eachDay, gapCountsByDay, heatLevel } from './coverage';
import { matrixToCsv } from './matrixCsv';

const cell = (pack: string, cat: string, period: string, price: number, priced = true) => ({
  pack_id: pack,
  price_category: cat,
  rate_period: period,
  zone_id: 'z1',
  date: '2027-07-07',
  priced,
  price,
});

const MATRIX: PriceMatrix = {
  season_id: 's1',
  formula: 'day',
  channel: 'desk',
  slots: 'FULL',
  pack_ids: ['p1', 'p2'],
  categories: ['row1', 'row2'],
  rate_periods: ['low', 'peak'],
  cells: [
    cell('p1', 'row1', 'low', 1999),
    cell('p1', 'row1', 'peak', 3010),
    cell('p1', 'row2', 'low', 1),
    cell('p1', 'row2', 'peak', 0, false),
    cell('p2', 'row1', 'low', 100000),
  ],
};

const opts = (locale: string) => ({
  packName: (id: string) => ({ p1: 'Ombrellone; 2 lettini', p2: 'Gazebo "Deluxe"' })[id] ?? id,
  locale,
  headers: { pack: 'Pack', category: 'Categoria' },
  noPrice: 'senza prezzo',
});

describe('matrixToCsv', () => {
  it('writes exact euro amounts with ";" and decimal comma for Italian', () => {
    expect(matrixToCsv(MATRIX, opts('it')).split('\r\n')).toEqual([
      'Pack;Categoria;low;peak',
      '"Ombrellone; 2 lettini";row1;19,99;30,10',
      '"Ombrellone; 2 lettini";row2;0,01;senza prezzo',
      '"Gazebo ""Deluxe""";row1;1000,00;',
      '"Gazebo ""Deluxe""";row2;;',
      '',
    ]);
  });

  it('uses "," and decimal point for English', () => {
    const lines = matrixToCsv(MATRIX, opts('en')).split('\r\n');
    expect(lines[0]).toBe('Pack,Categoria,low,peak');
    expect(lines[1]).toBe('Ombrellone; 2 lettini,row1,19.99,30.10');
    expect(lines[3]).toBe('"Gazebo ""Deluxe""",row1,1000.00,');
  });
});

describe('coverage calendar', () => {
  const report = {
    season_id: 's1',
    from: '2027-05-30',
    to: '2027-06-02',
    complete: false,
    gaps: [
      {
        pack_id: 'p1',
        price_category: 'row1',
        channel: 'desk' as const,
        from: '2027-05-31',
        to: '2027-06-01',
      },
      {
        pack_id: 'p1',
        price_category: 'row1',
        channel: 'online' as const,
        from: '2027-06-01',
        to: '2027-06-01',
      },
    ],
  };

  it('enumerates days across month ends and DST changes', () => {
    expect(eachDay('2027-03-27', '2027-03-29')).toEqual(['2027-03-27', '2027-03-28', '2027-03-29']);
    expect(eachDay('2027-02-27', '2027-03-01')).toEqual(['2027-02-27', '2027-02-28', '2027-03-01']);
  });

  it('counts uncovered combinations per day', () => {
    const counts = gapCountsByDay(report);
    expect(Object.fromEntries(counts)).toEqual({ '2027-05-31': 1, '2027-06-01': 2 });
  });

  it('splits the season into Monday-first months', () => {
    expect(calendarMonths(report.from, report.to)).toEqual([
      // 30 May 2027 is a Sunday → 6 empty cells before it.
      { key: '2027-05', offset: 6, days: ['2027-05-30', '2027-05-31'] },
      // 1 June 2027 is a Tuesday.
      { key: '2027-06', offset: 1, days: ['2027-06-01', '2027-06-02'] },
    ]);
  });

  it.each([
    [0, 5, 0],
    [1, 5, 1],
    [3, 5, 3],
    [5, 5, 4],
    [1, 1, 4],
  ])('heat level of %i over %i is %i', (count, max, level) => {
    expect(heatLevel(count, max)).toBe(level);
  });
});
