import { describe, expect, it } from 'vitest';
import de from './locales/de.json';
import en from './locales/en.json';
import fr from './locales/fr.json';
import itMessages from './locales/it.json';
import { createI18n, matchLocale, resolveLocale } from './index';

function keys(obj: unknown, prefix = ''): string[] {
  if (typeof obj !== 'object' || obj === null) return [prefix];
  return Object.entries(obj).flatMap(([k, v]) => keys(v, prefix ? `${prefix}.${k}` : k));
}

describe('locale files', () => {
  const source = keys(itMessages).sort();
  it.each([
    ['en', en],
    ['de', de],
    ['fr', fr],
  ])('%s has exactly the Italian keys', (_name, messages) => {
    expect(keys(messages).sort()).toEqual(source);
  });
});

describe('matchLocale', () => {
  it.each([
    ['de-AT', 'de'],
    ['EN_us', 'en'],
    ['it', 'it'],
    ['es', null],
    ['', null],
    [null, null],
  ])('%j -> %j', (input, expected) => {
    expect(matchLocale(input)).toBe(expected);
  });
});

describe('resolveLocale', () => {
  it.each([
    ['param wins', { param: 'fr', browser: ['de'], establishmentDefault: 'it' }, 'fr'],
    [
      'browser next',
      { param: 'xx', browser: ['es-ES', 'de-DE'], establishmentDefault: 'it' },
      'de',
    ],
    ['establishment default', { browser: ['es'], establishmentDefault: 'en' }, 'en'],
    ['italian last', {}, 'it'],
    [
      'respects enabled locales',
      { param: 'fr', browser: ['de'], establishmentDefault: 'en', enabled: ['it', 'en'] },
      'en',
    ],
    ['first enabled when default disabled', { establishmentDefault: 'it', enabled: ['de'] }, 'de'],
  ] as const)('%s', (_name, inputs, expected) => {
    expect(resolveLocale(inputs)).toBe(expected);
  });
});

describe('createI18n', () => {
  it('translates in Italian and loads other locales lazily', async () => {
    const i18nIt = await createI18n('it');
    expect(i18nIt.t('map.fitView')).toBe('Adatta alla vista');
    const i18nEn = await createI18n('en');
    expect(i18nEn.t('map.fitView')).toBe('Fit to view');
    expect(i18nEn.t('common.comingSoon', { milestone: 'M3' })).toBe('Available from milestone M3');
  });
});
