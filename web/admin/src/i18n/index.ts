import { changeLocale, createI18n, createTestI18n, type Locale } from '@arenile/shared/i18n';
import type { i18n as I18n } from 'i18next';
import { FEATURE_NAMESPACES } from './namespaces';

export { FEATURE_NAMESPACES } from './namespaces';

/** Adds the feature namespaces of `locale` to `instance` (no-op when present). */
export async function loadFeatureNamespaces(instance: I18n, locale: Locale): Promise<void> {
  await Promise.all(
    FEATURE_NAMESPACES.map(async (f) => {
      if (instance.hasResourceBundle(locale, f.ns)) return;
      const messages = locale === 'it' ? f.it : (await f.load[locale]()).default;
      instance.addResourceBundle(locale, f.ns, messages, true, true);
    }),
  );
}

function addItalian(instance: I18n) {
  for (const f of FEATURE_NAMESPACES) instance.addResourceBundle('it', f.ns, f.it, true, true);
}

/**
 * Creates the admin i18next instance: shared `translation` namespace plus the
 * feature namespaces. Italian (the fallback) is always loaded.
 */
export async function createAdminI18n(locale: Locale): Promise<I18n> {
  const instance = await createI18n(locale);
  addItalian(instance);
  if (locale !== 'it') await loadFeatureNamespaces(instance, locale);
  return instance;
}

/** Switches the admin language, loading the feature namespaces first. */
export async function changeAdminLocale(instance: I18n, locale: Locale): Promise<void> {
  await loadFeatureNamespaces(instance, locale);
  await changeLocale(instance, locale);
}

/** Synchronous Italian instance with every admin namespace, for tests. */
export function createAdminTestI18n(): I18n {
  const instance = createTestI18n();
  addItalian(instance);
  return instance;
}
