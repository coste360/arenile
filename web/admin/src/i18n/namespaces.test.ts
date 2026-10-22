import { describe, expect, it } from 'vitest';
import { FEATURE_NAMESPACES } from './namespaces';
import { createAdminI18n, createAdminTestI18n } from './index';

function keys(obj: unknown, prefix = ''): string[] {
  if (typeof obj !== 'object' || obj === null) return [prefix];
  return Object.entries(obj).flatMap(([k, v]) => keys(v, prefix ? `${prefix}.${k}` : k));
}

describe('feature namespaces', () => {
  it('have unique names', () => {
    const names = FEATURE_NAMESPACES.map((f) => f.ns);
    expect(new Set(names).size).toBe(names.length);
  });

  describe.each(FEATURE_NAMESPACES.map((f) => [f.ns, f] as const))('%s', (_ns, f) => {
    it.each(['en', 'de', 'fr'] as const)('%s has exactly the Italian keys', async (locale) => {
      const messages = (await f.load[locale]()).default;
      expect(keys(messages).sort()).toEqual(keys(f.it).sort());
    });
  });

  it('are available in the test instance', () => {
    const i18n = createAdminTestI18n();
    expect(i18n.t('pricing:simulator.title')).toBe('Simulatore prezzi');
    expect(i18n.t('nav.settings')).toBe('Configurazione');
  });

  it('are loaded for other languages with Italian fallback', async () => {
    const i18n = await createAdminI18n('en');
    expect(i18n.t('mapEditor:editor.title')).toBe('Map editor');
    expect(i18n.hasResourceBundle('it', 'mapEditor')).toBe(true);
  });
});
