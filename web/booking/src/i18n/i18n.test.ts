import { createI18n } from '@arenile/shared/i18n';
import { describe, expect, it } from 'vitest';
import { PUBLIC_NS, addPublicMessages, publicItalian, publicLocaleLoaders } from './index';

function keys(obj: unknown, prefix = ''): string[] {
  if (typeof obj !== 'object' || obj === null) return [prefix];
  return Object.entries(obj).flatMap(([k, v]) => keys(v, prefix ? `${prefix}.${k}` : k));
}

describe('public namespace', () => {
  it.each(['en', 'de', 'fr'] as const)('%s has exactly the Italian keys', async (locale) => {
    const messages = (await publicLocaleLoaders[locale]()).default;
    expect(keys(messages).sort()).toEqual(keys(publicItalian).sort());
  });

  it('loads a locale with the Italian fallback', async () => {
    const i18n = await createI18n('en');
    await addPublicMessages(i18n, 'en');
    expect(i18n.t('who.confirm', { ns: PUBLIC_NS })).toBe('Confirm booking');
    expect(i18n.hasResourceBundle('it', PUBLIC_NS)).toBe(true);
  });
});
