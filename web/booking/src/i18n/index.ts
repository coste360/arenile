import type { Locale } from '@arenile/shared/i18n';
import type { i18n as I18n } from 'i18next';
import it from './it.json';

/**
 * i18next namespace of the public booking flow (steps, summary,
 * confirmation, manage area, problem messages). Italian is the source and
 * is bundled; en/de/fr are lazy chunks loaded with the locale. The shell
 * strings (`booking.*`, `common.*`) stay in the shared `translation`
 * namespace. Use `useTranslation(PUBLIC_NS)` or `t('public:who.pay')`.
 */
export const PUBLIC_NS = 'public';

export type PublicMessages = typeof it;

const loaders: Record<Exclude<Locale, 'it'>, () => Promise<{ default: PublicMessages }>> = {
  en: () => import('./en.json'),
  de: () => import('./de.json'),
  fr: () => import('./fr.json'),
};

/** Loads the public messages of `locale` (plus the Italian fallback) into `i18n`. */
export async function addPublicMessages(i18n: I18n, locale: Locale): Promise<void> {
  if (!i18n.hasResourceBundle('it', PUBLIC_NS)) i18n.addResourceBundle('it', PUBLIC_NS, it);
  if (locale === 'it' || i18n.hasResourceBundle(locale, PUBLIC_NS)) return;
  const mod = await loaders[locale]();
  i18n.addResourceBundle(locale, PUBLIC_NS, mod.default);
}

export const publicLocaleLoaders = loaders;
export const publicItalian = it;
