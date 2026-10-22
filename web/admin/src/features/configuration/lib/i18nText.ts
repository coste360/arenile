/** Localised text as stored by the API: locale → text (OpenAPI `I18nText`). */
export type I18nText = Record<string, string>;

/**
 * Text in `locale`, falling back to the establishment default locale, then to
 * any non-empty value (spec 20: fallback to the default locale).
 */
export function localize(
  text: I18nText | null | undefined,
  locale: string,
  defaultLocale: string,
): string {
  if (!text) return '';
  const direct = text[locale]?.trim();
  if (direct) return direct;
  const fallback = text[defaultLocale]?.trim();
  if (fallback) return fallback;
  return Object.values(text).find((v) => v.trim() !== '') ?? '';
}

/** Drops empty translations: the API stores only the languages actually filled in. */
export function compactI18n(text: I18nText): I18nText {
  const out: I18nText = {};
  for (const [k, v] of Object.entries(text)) {
    const s = v.trim();
    if (s) out[k] = s;
  }
  return out;
}

/** Form value for an I18nText: one (possibly empty) entry per enabled locale. */
export function i18nFormValue(text: I18nText | null | undefined, locales: readonly string[]) {
  const out: I18nText = {};
  for (const l of locales) out[l] = text?.[l] ?? '';
  // Keep translations of languages no longer enabled so a save does not erase them.
  for (const [k, v] of Object.entries(text ?? {})) if (!(k in out)) out[k] = v;
  return out;
}

/** Enabled locales with the default one first. */
export function orderedLocales(defaultLocale: string, enabled: readonly string[]): string[] {
  return [defaultLocale, ...enabled.filter((l) => l !== defaultLocale)];
}
