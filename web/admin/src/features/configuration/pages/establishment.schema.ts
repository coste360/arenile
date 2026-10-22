import type { Establishment } from '@arenile/shared/api';
import { z } from 'zod';
import type { EstablishmentUpdate } from '../api/establishment';
import {
  apiDecimalToInput,
  basisPointsToPercentText as basisPointsToPercentInput,
  parsePercentToBasisPoints as percentToBasisPoints,
  toApiDecimal,
} from '@arenile/shared/lib/money';
import { normalizeOrigin, splitLines } from '../lib/publicPage';
import {
  HEX_COLOR_RE,
  SLUG_RE,
  V,
  decimalField,
  hexColor,
  percentField,
  requiredText,
} from '../lib/validation';

export const SUPPORTED_LOCALES = ['it', 'en', 'de', 'fr'] as const;

// ---- General data, languages, contacts ---------------------------------------

export const generalSchema = z
  .object({
    name: requiredText(120),
    slug: z.string().trim().regex(SLUG_RE, V.slug),
    timezone: z.string().trim().min(1, V.required),
    default_locale: z.enum(SUPPORTED_LOCALES),
    enabled_locales: z.array(z.enum(SUPPORTED_LOCALES)).min(1, V.localeRequired),
    contact_email: z
      .string()
      .trim()
      .max(254)
      .refine((v) => v === '' || z.email().safeParse(v).success, V.email),
    contact_phone: z.string().trim().max(40),
    street: z.string().trim().max(200),
    city: z.string().trim().max(120),
    province: z.string().trim().max(40),
    postal_code: z.string().trim().max(20),
    country: z.string().trim().max(2),
    concession_ref: z.string().trim().max(120),
  })
  .refine((v) => v.enabled_locales.includes(v.default_locale), {
    message: V.localeRequired,
    path: ['enabled_locales'],
  });

export type GeneralValues = z.infer<typeof generalSchema>;

function asLocale(v: string): (typeof SUPPORTED_LOCALES)[number] {
  return (SUPPORTED_LOCALES as readonly string[]).includes(v)
    ? (v as (typeof SUPPORTED_LOCALES)[number])
    : 'it';
}

export function toGeneralValues(e: Establishment): GeneralValues {
  return {
    name: e.name,
    slug: e.slug,
    timezone: e.timezone,
    default_locale: asLocale(e.default_locale),
    enabled_locales: e.enabled_locales.filter((l) =>
      (SUPPORTED_LOCALES as readonly string[]).includes(l),
    ) as GeneralValues['enabled_locales'],
    contact_email: e.contact_email ?? '',
    contact_phone: e.contact_phone ?? '',
    street: e.address?.street ?? '',
    city: e.address?.city ?? '',
    province: e.address?.province ?? '',
    postal_code: e.address?.postal_code ?? '',
    country: e.address?.country ?? 'IT',
    concession_ref: e.concession_ref ?? '',
  };
}

export function generalPatch(v: GeneralValues): EstablishmentUpdate {
  return {
    name: v.name,
    slug: v.slug,
    timezone: v.timezone,
    default_locale: v.default_locale,
    enabled_locales: v.enabled_locales,
    contact_email: v.contact_email,
    contact_phone: v.contact_phone,
    address: {
      street: v.street,
      city: v.city,
      province: v.province,
      postal_code: v.postal_code,
      country: v.country.toUpperCase(),
    },
    concession_ref: v.concession_ref,
  };
}

export const generalAliases = {
  'address.street': 'street',
  'address.city': 'city',
  'address.province': 'province',
  'address.postal_code': 'postal_code',
  'address.country': 'country',
};

// ---- Public page: booking toggle and embed origins ---------------------------

export const publicPageSchema = z.object({
  booking_enabled: z.boolean(),
  allowed_embed_origins: z
    .string()
    .refine((text) => splitLines(text).every((o) => normalizeOrigin(o) !== null), V.origin),
});

export type PublicPageValues = z.infer<typeof publicPageSchema>;

export function toPublicPageValues(e: Establishment): PublicPageValues {
  return {
    booking_enabled: e.booking_enabled,
    allowed_embed_origins: (e.allowed_embed_origins ?? []).join('\n'),
  };
}

export function publicPagePatch(v: PublicPageValues): EstablishmentUpdate {
  const origins = splitLines(v.allowed_embed_origins)
    .map((o) => normalizeOrigin(o))
    .filter((o): o is string => o !== null);
  return { booking_enabled: v.booking_enabled, allowed_embed_origins: [...new Set(origins)] };
}

// ---- Theme (spec 12.5) ----------------------------------------------------------

export const FONT_FAMILIES = ['Inter', 'system', 'serif'] as const;
export const RADII = ['none', 'sm', 'md', 'lg'] as const;

export const themeSchema = z.object({
  logo_url: z
    .string()
    .trim()
    .refine((v) => v === '' || /^https:\/\/\S+$/i.test(v), V.url),
  primary_color: hexColor,
  accent_color: hexColor,
  font_family: z.enum(FONT_FAMILIES),
  radius: z.enum(RADII),
  show_prices_on_map: z.boolean(),
  hide_header_in_embed: z.boolean(),
  /** custom_texts.<locale>.intro */
  intro: z.record(z.string(), z.string().max(1000)),
});

export type ThemeValues = z.infer<typeof themeSchema>;

export function toThemeValues(e: Establishment): ThemeValues {
  const th = e.theme ?? {};
  const intro: Record<string, string> = {};
  for (const l of e.enabled_locales) intro[l] = th.custom_texts?.[l]?.intro ?? '';
  return {
    logo_url: th.logo_url ?? '',
    primary_color:
      th.primary_color && HEX_COLOR_RE.test(th.primary_color) ? th.primary_color : '#0e6ba8',
    accent_color:
      th.accent_color && HEX_COLOR_RE.test(th.accent_color) ? th.accent_color : '#f2b134',
    font_family: th.font_family ?? 'system',
    radius: th.radius ?? 'md',
    show_prices_on_map: th.show_prices_on_map ?? true,
    hide_header_in_embed: th.hide_header_in_embed ?? true,
    intro,
  };
}

/** Theme patch; other custom texts (e.g. terms) of the current theme are preserved. */
export function themePatch(v: ThemeValues, current: Establishment): EstablishmentUpdate {
  const customTexts: Record<string, Record<string, string>> = {};
  for (const [locale, texts] of Object.entries(current.theme?.custom_texts ?? {})) {
    customTexts[locale] = { ...texts };
  }
  for (const [locale, intro] of Object.entries(v.intro)) {
    const text = intro.trim();
    const entry = { ...(customTexts[locale] ?? {}) };
    if (text) entry.intro = text;
    else delete entry.intro;
    if (Object.keys(entry).length > 0) customTexts[locale] = entry;
    else delete customTexts[locale];
  }
  return {
    theme: {
      ...(v.logo_url ? { logo_url: v.logo_url } : {}),
      primary_color: v.primary_color.toLowerCase(),
      accent_color: v.accent_color.toLowerCase(),
      font_family: v.font_family,
      radius: v.radius,
      show_prices_on_map: v.show_prices_on_map,
      hide_header_in_embed: v.hide_header_in_embed,
      custom_texts: customTexts,
    },
  };
}

export const themeAliases = {
  'theme.logo_url': 'logo_url',
  'theme.primary_color': 'primary_color',
  'theme.accent_color': 'accent_color',
  'theme.font_family': 'font_family',
  'theme.radius': 'radius',
};

// ---- Pricing settings (spec 8.1, 8.5) ------------------------------------------

export const ROUNDINGS = ['none', 'euro', 'fifty_cents'] as const;

export const pricingSettingsSchema = z.object({
  half_day_factor: decimalField.refine((v) => {
    const d = toApiDecimal(v);
    return d !== null && Number(d) > 0 && Number(d) <= 1;
  }, V.halfDayFactor),
  rounding: z.enum(ROUNDINGS),
  default_vat_rate: percentField,
});

export type PricingSettingsValues = z.infer<typeof pricingSettingsSchema>;

export function toPricingSettingsValues(e: Establishment): PricingSettingsValues {
  const p = e.settings.pricing;
  return {
    half_day_factor: apiDecimalToInput(p.half_day_factor || '0.6'),
    rounding: p.rounding ?? 'none',
    default_vat_rate: basisPointsToPercentInput(p.default_vat_rate_bp ?? 0),
  };
}

/** Settings are replaced as a whole document: everything not edited here is carried over. */
export function pricingSettingsPatch(
  v: PricingSettingsValues,
  current: Establishment,
): EstablishmentUpdate {
  return {
    settings: {
      ...current.settings,
      pricing: {
        ...current.settings.pricing,
        half_day_factor:
          toApiDecimal(v.half_day_factor) ?? current.settings.pricing.half_day_factor,
        rounding: v.rounding,
        default_vat_rate_bp: percentToBasisPoints(v.default_vat_rate) ?? 0,
      },
    },
  };
}

export const pricingSettingsAliases = {
  'settings.pricing.half_day_factor': 'half_day_factor',
  'settings.pricing.rounding': 'rounding',
  'settings.pricing.default_vat_rate_bp': 'default_vat_rate',
};
