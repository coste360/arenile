import { DEFAULT_ACCENT, DEFAULT_PRIMARY } from '@arenile/shared/theme';
import type { Schemas } from '@arenile/shared/api';
import { z } from 'zod';
import { normalizeOrigin, splitLines } from '../../configuration/lib/publicPage';
import { HEX_COLOR_RE, V, hexColor } from '../../configuration/lib/validation';
import type { OnlineSettings, OnlineSettingsInput } from '../api';

export const FORMULAS = [
  'half_day',
  'day',
  'multi_day',
  'week',
  'fortnight',
  'month',
  'season',
] as const satisfies readonly Schemas['Formula'][];
export const SLOTS = ['FULL', 'AM', 'PM'] as const satisfies readonly Schemas['Slots'][];
export const FONT_FAMILIES = ['Inter', 'system', 'serif'] as const;
export const RADII = ['none', 'sm', 'md', 'lg'] as const;

/**
 * Form of "Pagina pubblica" (spec 12, 15.1): booking switch, embed origins,
 * online flow options and the theme. Messages are namespaced i18n keys.
 */
export const onlineSettingsSchema = z.object({
  booking_enabled: z.boolean(),
  allowed_embed_origins: z
    .string()
    .refine((text) => splitLines(text).every((o) => normalizeOrigin(o) !== null), V.origin),
  formulas: z.array(z.enum(FORMULAS)),
  slots: z.array(z.enum(SLOTS)).min(1, V.slotRequired),
  show_rate_periods: z.boolean(),
  max_days_ahead: z
    .string()
    .trim()
    .refine((v) => v === '' || (/^\d+$/.test(v) && Number(v) >= 1 && Number(v) <= 730), V.integer),
  promo_codes_enabled: z.boolean(),
  require_phone: z.boolean(),
  primary_color: hexColor,
  accent_color: hexColor,
  font_family: z.enum(FONT_FAMILIES),
  radius: z.enum(RADII),
  logo_url: z
    .string()
    .trim()
    .refine((v) => v === '' || /^https:\/\/\S+$/i.test(v), V.url),
  show_prices_on_map: z.boolean(),
  hide_header_in_embed: z.boolean(),
  /** custom_texts.<locale>.intro */
  intro: z.record(z.string(), z.string().max(1000)),
});

export type OnlineSettingsValues = z.infer<typeof onlineSettingsSchema>;

export function toOnlineValues(
  s: OnlineSettings,
  locales: readonly string[],
): OnlineSettingsValues {
  const th = s.theme ?? {};
  const intro: Record<string, string> = {};
  for (const l of locales) intro[l] = th.custom_texts?.[l]?.intro ?? '';
  return {
    booking_enabled: s.booking_enabled,
    allowed_embed_origins: s.allowed_embed_origins.join('\n'),
    formulas: FORMULAS.filter((f) => s.flow.formulas.includes(f)),
    slots: SLOTS.filter((x) => (s.flow.slots.length ? s.flow.slots : SLOTS).includes(x)),
    show_rate_periods: s.flow.show_rate_periods,
    max_days_ahead: s.flow.max_days_ahead ? String(s.flow.max_days_ahead) : '',
    promo_codes_enabled: s.flow.promo_codes_enabled,
    require_phone: s.flow.require_phone,
    primary_color:
      th.primary_color && HEX_COLOR_RE.test(th.primary_color) ? th.primary_color : DEFAULT_PRIMARY,
    accent_color:
      th.accent_color && HEX_COLOR_RE.test(th.accent_color) ? th.accent_color : DEFAULT_ACCENT,
    font_family: th.font_family ?? 'system',
    radius: th.radius ?? 'md',
    logo_url: th.logo_url ?? '',
    show_prices_on_map: th.show_prices_on_map ?? true,
    hide_header_in_embed: th.hide_header_in_embed ?? true,
    intro,
  };
}

/** Theme of the form values (also posted to the live preview before saving). */
export function themeOf(
  v: Pick<
    OnlineSettingsValues,
    | 'primary_color'
    | 'accent_color'
    | 'font_family'
    | 'radius'
    | 'logo_url'
    | 'show_prices_on_map'
    | 'hide_header_in_embed'
    | 'intro'
  >,
  current?: Schemas['Theme'],
): Schemas['Theme'] {
  // Other custom texts of the current theme (e.g. a legacy "terms") are preserved.
  const customTexts: Record<string, Record<string, string>> = {};
  for (const [locale, texts] of Object.entries(current?.custom_texts ?? {})) {
    customTexts[locale] = { ...texts };
  }
  for (const [locale, intro] of Object.entries(v.intro ?? {})) {
    const text = intro.trim();
    const entry = { ...(customTexts[locale] ?? {}) };
    if (text) entry.intro = text;
    else delete entry.intro;
    if (Object.keys(entry).length > 0) customTexts[locale] = entry;
    else delete customTexts[locale];
  }
  return {
    ...(v.logo_url.trim() ? { logo_url: v.logo_url.trim() } : {}),
    primary_color: v.primary_color.toLowerCase(),
    accent_color: v.accent_color.toLowerCase(),
    font_family: v.font_family,
    radius: v.radius,
    show_prices_on_map: v.show_prices_on_map,
    hide_header_in_embed: v.hide_header_in_embed,
    custom_texts: customTexts,
  };
}

export function toOnlineInput(
  v: OnlineSettingsValues,
  current: OnlineSettings,
): OnlineSettingsInput {
  return {
    booking_enabled: v.booking_enabled,
    allowed_embed_origins: [
      ...new Set(splitLines(v.allowed_embed_origins).map((o) => normalizeOrigin(o) as string)),
    ],
    theme: themeOf(v, current.theme),
    flow: {
      formulas: v.formulas,
      slots: v.slots,
      show_rate_periods: v.show_rate_periods,
      ...(v.max_days_ahead ? { max_days_ahead: Number(v.max_days_ahead) } : {}),
      promo_codes_enabled: v.promo_codes_enabled,
      require_phone: v.require_phone,
    },
  };
}

/** Server field path → form field. */
export const onlineAliases: Record<string, string> = {
  'theme.logo_url': 'logo_url',
  'theme.primary_color': 'primary_color',
  'theme.accent_color': 'accent_color',
  'theme.font_family': 'font_family',
  'theme.radius': 'radius',
  'flow.formulas': 'formulas',
  'flow.slots': 'slots',
  'flow.max_days_ahead': 'max_days_ahead',
};

// ---- Terms versions (spec 12.3, 23) --------------------------------------------------

export const termsSchema = z.object({
  version: z
    .string()
    .trim()
    .min(1, V.required)
    .max(40)
    .regex(/^[A-Za-z0-9._-]+$/, V.code),
  texts: z.record(
    z.string(),
    z.object({
      terms: z.string().max(20_000),
      privacy: z.string().max(20_000),
      marketing: z.string().max(2_000),
    }),
  ),
  make_current: z.boolean(),
});

export type TermsValues = z.infer<typeof termsSchema>;

/** Terms of the default locale are mandatory; empty optional texts are omitted. */
export function toTermsInput(v: TermsValues): Schemas['TermsVersionInput'] {
  const texts: Schemas['TermsVersionInput']['texts'] = {};
  for (const [locale, t] of Object.entries(v.texts)) {
    if (!t.terms.trim()) continue;
    texts[locale] = {
      terms: t.terms.trim(),
      ...(t.privacy.trim() ? { privacy: t.privacy.trim() } : {}),
      ...(t.marketing.trim() ? { marketing: t.marketing.trim() } : {}),
    };
  }
  return { version: v.version.trim(), texts, make_current: v.make_current };
}

/** Suggested next version: today's date with a counter ("2027-05-01" or "2027-05-01.2"). */
export function nextTermsVersion(existing: readonly string[], today: string): string {
  if (!existing.includes(today)) return today;
  let n = 2;
  while (existing.includes(`${today}.${n}`)) n++;
  return `${today}.${n}`;
}
