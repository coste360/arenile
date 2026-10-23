import type { Locale } from '@arenile/shared/i18n';
import type { i18n as I18n } from 'i18next';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

export type NamespaceLoaders = Record<
  Exclude<Locale, 'it'>,
  () => Promise<{ default: Record<string, unknown> }>
>;

/** Adds the Italian bundle (source and fallback) of `ns` once. */
export function addItalian(i18n: I18n, ns: string, it: Record<string, unknown>): void {
  if (!i18n.hasResourceBundle('it', ns)) i18n.addResourceBundle('it', ns, it);
}

/**
 * i18next namespace of a lazily loaded part of the public page (M5B manage
 * additions): the Italian messages ship with the chunk that uses them,
 * en/de/fr are separate chunks loaded for the current language, so none of
 * them weighs on the booking first screen. Renders with the Italian
 * fallback until the translation arrives, then re-renders.
 */
export function useLazyNamespace(
  ns: string,
  it: Record<string, unknown>,
  loaders: NamespaceLoaders,
): void {
  const { i18n } = useTranslation();
  addItalian(i18n, ns, it);
  const language = i18n.language as Locale;
  const [, setLoaded] = useState('');
  useEffect(() => {
    if (language === 'it' || !(language in loaders)) return;
    if (i18n.hasResourceBundle(language, ns)) return;
    let alive = true;
    void loaders[language as Exclude<Locale, 'it'>]().then((mod) => {
      if (!i18n.hasResourceBundle(language, ns)) {
        i18n.addResourceBundle(language, ns, mod.default);
      }
      if (alive) setLoaded(language);
    });
    return () => {
      alive = false;
    };
  }, [i18n, language, ns, loaders]);
}
