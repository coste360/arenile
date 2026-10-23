/**
 * i18n for the live components: the REAL translations of web/shared (only
 * the `map` subtree is needed, imported as a named JSON export so the rest of
 * the catalogue is tree-shaken), in a synchronous isolated i18next instance
 * so the islands render on the server too (no flash, no layout shift).
 */
import i18next, { type i18n as I18n } from 'i18next';
import { initReactI18next } from 'react-i18next';
import type { NAMESPACE as SHARED_NAMESPACE } from '@arenile/shared/i18n';
import { map as enMap } from '@arenile/shared/i18n/locales/en.json';
import { map as itMap } from '@arenile/shared/i18n/locales/it.json';

export type DocsLocale = 'it' | 'en';

/** The shared namespace (type-checked against web/shared, without importing its loaders). */
const NAMESPACE: typeof SHARED_NAMESPACE = 'translation';

const cache = new Map<DocsLocale, I18n>();

/** Docs page language ("it" for the root locale) to a supported one. */
export function docsLocale(lang: string | undefined): DocsLocale {
  return lang?.toLowerCase().startsWith('en') ? 'en' : 'it';
}

/**
 * Shared label of a spot status. Read from the catalogue directly: in the
 * docs, i18next's key types are Starlight's (it augments CustomTypeOptions),
 * so typed `t()` calls with Arenile keys would not type-check.
 */
export function statusLabel(locale: DocsLocale, status: string): string {
  const names = (locale === 'en' ? enMap : itMap).status as Record<string, string>;
  return names[status] ?? itMap.status[status as keyof typeof itMap.status] ?? status;
}

export function docsI18n(locale: DocsLocale): I18n {
  const hit = cache.get(locale);
  if (hit) return hit;
  const instance = i18next.createInstance();
  void instance.use(initReactI18next).init({
    lng: locale,
    fallbackLng: 'it',
    defaultNS: NAMESPACE,
    ns: [NAMESPACE],
    resources: {
      it: { [NAMESPACE]: { map: itMap } },
      en: { [NAMESPACE]: { map: enMap } },
    },
    interpolation: { escapeValue: false },
    initAsync: false,
  });
  cache.set(locale, instance);
  return instance;
}
