import i18next, { type i18n as I18n, type Resource } from 'i18next';
import { initReactI18next } from 'react-i18next';
import it from './locales/it.json';

/** Languages bundled with Arenile. Italian is the source language. */
export const SUPPORTED_LOCALES = ['it', 'en', 'de', 'fr'] as const;
export type Locale = (typeof SUPPORTED_LOCALES)[number];
export const SOURCE_LOCALE: Locale = 'it';

/** Namespace used for every UI string. */
export const NAMESPACE = 'translation';

export type Messages = typeof it;

const loaders: Record<Exclude<Locale, 'it'>, () => Promise<{ default: Messages }>> = {
  en: () => import('./locales/en.json'),
  de: () => import('./locales/de.json'),
  fr: () => import('./locales/fr.json'),
};

export function isLocale(value: string | null | undefined): value is Locale {
  return !!value && (SUPPORTED_LOCALES as readonly string[]).includes(value);
}

/** Normalises a BCP 47 tag ("de-AT") to a supported locale ("de"), if any. */
export function matchLocale(tag: string | null | undefined): Locale | null {
  if (!tag) return null;
  const base = tag.toLowerCase().split(/[-_]/)[0];
  return isLocale(base) ? base : null;
}

export interface LocaleInputs {
  /** Explicit `?lang=` parameter. */
  param?: string | null;
  /** `navigator.languages`, most preferred first. */
  browser?: readonly string[];
  /** Default locale of the establishment. */
  establishmentDefault?: string | null;
  /** Locales enabled by the establishment; empty means all supported. */
  enabled?: readonly string[] | null;
}

/**
 * Picks the UI language following spec 20: query parameter, then browser
 * language, then the establishment default, then Italian.
 */
export function resolveLocale(inputs: LocaleInputs): Locale {
  const enabled = (inputs.enabled ?? []).map(matchLocale).filter((l): l is Locale => l !== null);
  const allowed = (l: Locale | null): l is Locale =>
    l !== null && (enabled.length === 0 || enabled.includes(l));

  const fromParam = matchLocale(inputs.param);
  if (allowed(fromParam)) return fromParam;
  for (const tag of inputs.browser ?? []) {
    const l = matchLocale(tag);
    if (allowed(l)) return l;
  }
  const fallback = matchLocale(inputs.establishmentDefault);
  if (allowed(fallback)) return fallback;
  return enabled[0] ?? SOURCE_LOCALE;
}

/** Loads the messages for `locale` (Italian is bundled, others are split chunks). */
export async function loadMessages(locale: Locale): Promise<Messages> {
  if (locale === 'it') return it;
  const mod = await loaders[locale]();
  return mod.default;
}

/**
 * Creates an isolated i18next instance (no global singleton) with Italian
 * bundled and `locale` loaded on demand.
 */
export async function createI18n(locale: Locale = SOURCE_LOCALE): Promise<I18n> {
  const resources: Resource = { it: { [NAMESPACE]: it } };
  if (locale !== 'it') {
    resources[locale] = { [NAMESPACE]: await loadMessages(locale) };
  }
  const instance = i18next.createInstance();
  await instance.use(initReactI18next).init({
    lng: locale,
    fallbackLng: SOURCE_LOCALE,
    supportedLngs: [...SUPPORTED_LOCALES],
    defaultNS: NAMESPACE,
    ns: [NAMESPACE],
    resources,
    interpolation: { escapeValue: false },
    returnNull: false,
  });
  return instance;
}

/** Switches language, loading its messages first if needed. */
export async function changeLocale(instance: I18n, locale: Locale): Promise<void> {
  if (!instance.hasResourceBundle(locale, NAMESPACE)) {
    instance.addResourceBundle(locale, NAMESPACE, await loadMessages(locale));
  }
  await instance.changeLanguage(locale);
  if (typeof document !== 'undefined') document.documentElement.lang = locale;
}

/** Synchronous Italian-only instance, handy for tests. */
export function createTestI18n(): I18n {
  const instance = i18next.createInstance();
  void instance.use(initReactI18next).init({
    lng: 'it',
    fallbackLng: 'it',
    defaultNS: NAMESPACE,
    resources: { it: { [NAMESPACE]: it } },
    interpolation: { escapeValue: false },
    initAsync: false,
  });
  return instance;
}

export { it as italianMessages };
