import { afterEach, describe, expect, it } from 'vitest';
import { formatInstantDate } from './dates';
import { intlLocaleOf, uiIntlLocale } from './intl';
import { formatBasisPoints, formatCents } from './money';

const nbsp = (s: string) => s.replace(/[\u00a0\u202f]/g, ' ');

describe('intlLocaleOf', () => {
  it.each([
    ['it', 'it-IT'],
    ['en', 'en-GB'],
    ['de-AT', 'de-DE'],
    ['FR', 'fr-FR'],
    ['es', 'it-IT'],
    ['', 'it-IT'],
    [null, 'it-IT'],
  ])('%j -> %s', (lang, expected) => {
    expect(intlLocaleOf(lang)).toBe(expected);
  });
});

describe('shared formatters follow the UI language', () => {
  afterEach(() => {
    document.documentElement.lang = '';
  });

  it('default to Italian when no language is set', () => {
    expect(uiIntlLocale()).toBe('it-IT');
    expect(nbsp(formatCents(123450))).toBe('1.234,50 €');
  });

  it.each([
    ['en', '€1,234.50', '5.5%', '23/10/2026'],
    ['de', '1.234,50 €', '5,5 %', '23.10.2026'],
    ['fr', '1 234,50 €', '5,5 %', '23/10/2026'],
  ])('format money, percentages and dates in %s', (lang, money, percent, date) => {
    document.documentElement.lang = lang;
    expect(nbsp(formatCents(123450))).toBe(money);
    expect(nbsp(formatBasisPoints(550))).toBe(percent);
    expect(formatInstantDate('2026-10-23T10:00:00Z', { timeZone: 'Europe/Rome' })).toBe(date);
  });

  it('let an explicit locale win', () => {
    document.documentElement.lang = 'de';
    expect(nbsp(formatCents(2500, 'EUR', 'it-IT'))).toBe('25,00 €');
  });
});
