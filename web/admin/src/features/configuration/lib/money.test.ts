import { describe, expect, it } from 'vitest';
import {
  apiDecimalToInput,
  basisPointsToPercentInput,
  centsToEuroInput,
  euroToCents,
  formatCents,
  parseScaled,
  percentToBasisPoints,
  toApiDecimal,
} from './money';

describe('euroToCents', () => {
  it.each([
    ['12', 1200],
    ['12,5', 1250],
    ['12.50', 1250],
    ['19,99', 1999],
    ['0,01', 1],
    [',5', 50],
    ['  7 ', 700],
    ['1 000', 100000],
    ['0', 0],
    ['12,', 1200],
  ])('%j → %d', (input, cents) => {
    expect(euroToCents(input)).toBe(cents);
  });

  it.each(['', 'abc', '-5', '1,234', '1.2.3', '12,5€', '1e3', '99999999999999999999'])(
    'rejects %j',
    (input) => {
      expect(euroToCents(input)).toBeNull();
    },
  );

  it('round-trips without floating point drift', () => {
    for (let c = 0; c < 10000; c += 7) {
      expect(euroToCents(centsToEuroInput(c))).toBe(c);
    }
  });
});

describe('centsToEuroInput', () => {
  it.each([
    [1250, 'it', '12,50'],
    [1250, 'en', '12.50'],
    [5, 'it', '0,05'],
    [100000, 'de', '1000,00'],
  ])('%d (%s) → %j', (cents, locale, out) => {
    expect(centsToEuroInput(cents, locale)).toBe(out);
  });
});

describe('formatCents', () => {
  it('formats for display with the currency', () => {
    expect(formatCents(1250, 'en', 'EUR')).toBe('€12.50');
  });
});

describe('percent and basis points', () => {
  it.each([
    ['22', 2200],
    ['10', 1000],
    ['5,5', 550],
    ['4.25', 425],
  ])('%j → %d bp', (input, bp) => {
    expect(percentToBasisPoints(input)).toBe(bp);
  });
  it.each([
    [2200, '22'],
    [550, '5,5'],
    [425, '4,25'],
    [0, '0'],
  ])('%d bp → %j', (bp, out) => {
    expect(basisPointsToPercentInput(bp)).toBe(out);
  });
});

describe('decimals', () => {
  it.each([
    ['0,6', '0.6'],
    ['0.50', '0.5'],
    ['1', '1'],
    ['0,1234', '0.1234'],
  ])('%j → %j', (input, out) => {
    expect(toApiDecimal(input)).toBe(out);
  });
  it('rejects too many decimals', () => {
    expect(toApiDecimal('0,12345')).toBeNull();
    expect(parseScaled('1,234', 2)).toBeNull();
  });
  it('localises API decimals', () => {
    expect(apiDecimalToInput('0.6', 'it')).toBe('0,6');
    expect(apiDecimalToInput('0.6', 'en')).toBe('0.6');
  });
});
