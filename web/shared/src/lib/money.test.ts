import { describe, expect, it } from 'vitest';
import {
  apiDecimalToInput,
  basisPointsToPercentText,
  centsToEuroText,
  decimalSeparator,
  formatBasisPoints,
  formatCents,
  parseEuroToCents,
  parsePercentToBasisPoints,
  toApiDecimal,
} from './money';

describe('parseEuroToCents', () => {
  it.each([
    ['12', 1200],
    ['12,5', 1250],
    ['12,50', 1250],
    ['19,99', 1999],
    ['0,1', 10],
    [',5', 50],
    ['1.234,50', 123450],
    ['1 234,50', 123450],
    ['1.234', 123400],
    ['1.234.567', 123456700],
    ['12.50', 1250],
    ['1.23', 123],
    ['€ 7,00', 700],
    ['  3  ', 300],
  ])('%j -> %j', (input, cents) => {
    expect(parseEuroToCents(input)).toBe(cents);
  });

  it.each(['', ' ', 'abc', '1,234', '1,2,3', '12,345', '1.2.3', '-5', '1e3', '12.345,6.7'])(
    'rejects %j',
    (input) => {
      expect(parseEuroToCents(input)).toBeNull();
    },
  );

  it('accepts negatives only when allowed', () => {
    expect(parseEuroToCents('-5,10', { allowNegative: true })).toBe(-510);
  });

  it('never goes through floating point', () => {
    for (let c = 0; c < 10_000; c += 7) {
      expect(parseEuroToCents(centsToEuroText(c))).toBe(c);
    }
  });
});

describe('formatting', () => {
  it('formats editable euro text with Italian grouping', () => {
    expect(centsToEuroText(123450)).toBe('1.234,50');
    expect(centsToEuroText(5)).toBe('0,05');
    expect(centsToEuroText(-1999)).toBe('-19,99');
  });

  it('formats currency for display with grouping, also below 10.000', () => {
    expect(formatCents(123450).replace(/\s/g, ' ')).toBe('1.234,50 €');
    expect(formatCents(2500).replace(/\s/g, ' ')).toBe('25,00 €');
    expect(formatCents(100000, 'EUR').replace(/\s/g, ' ')).toBe('1.000,00 €');
  });

  it('converts exact decimals to and from the API format', () => {
    expect(toApiDecimal('0,6')).toBe('0.6');
    expect(toApiDecimal('1.2', 6)).toBe('1.2');
    expect(toApiDecimal('12,5', 2)).toBe('12.5');
    expect(toApiDecimal('1', 2)).toBe('1');
    expect(toApiDecimal('12,345', 2)).toBeNull();
    expect(toApiDecimal('abc')).toBeNull();
    expect(apiDecimalToInput('0.6')).toBe('0,6');
    expect(decimalSeparator('it')).toBe(',');
    expect(decimalSeparator('en')).toBe('.');
  });

  it('round-trips basis points', () => {
    expect(parsePercentToBasisPoints('22')).toBe(2200);
    expect(parsePercentToBasisPoints('5,5')).toBe(550);
    expect(basisPointsToPercentText(550)).toBe('5,5');
    expect(basisPointsToPercentText(2200)).toBe('22');
    expect(basisPointsToPercentText(1)).toBe('0,01');
    expect(formatBasisPoints(550).replace(/\s/g, '')).toBe('5,5%');
  });
});
