import { z } from 'zod';
import type {
  CatalogComponent,
  ComponentInput,
  Extra,
  ExtraInput,
  Pack,
  PackInput,
} from '../api/resources';
import { compactI18n, i18nFormValue } from '../lib/i18nText';
import { basisPointsToPercentInput, percentToBasisPoints } from '../lib/money';
import {
  V,
  codeField,
  i18nTextField,
  intField,
  optionalI18nText,
  optionalInt,
  optionalPercentField,
} from '../lib/validation';

export const FORMULAS = [
  'half_day',
  'day',
  'multi_day',
  'week',
  'fortnight',
  'month',
  'season',
] as const;
export const CHANNELS = ['desk', 'online', 'api'] as const;
export const SPOT_KINDS = ['umbrella', 'gazebo', 'tent', 'palm', 'custom'] as const;
export const PRICING_UNITS = ['per_booking', 'per_day', 'per_slot'] as const;
export const ATTACH_TO = ['line', 'booking'] as const;

const nonEmpty = <T extends z.ZodTypeAny>(item: T) => z.array(item).min(1, V.required);

/** undefined for an empty list: nullable lists mean "no restriction", never []. */
function listOrUndefined<T>(list: T[]): T[] | undefined {
  return list.length ? list : undefined;
}

function vatToBp(v: string): number | undefined {
  return v.trim() === '' ? undefined : (percentToBasisPoints(v) ?? undefined);
}

function bpToVat(bp: number | undefined, locale: string): string {
  return bp === undefined || bp === null ? '' : basisPointsToPercentInput(bp, locale);
}

// ---- Components ---------------------------------------------------------------------

export function componentSchema(defaultLocale: string) {
  return z.object({
    code: codeField,
    name: i18nTextField(defaultLocale),
    tracked: z.boolean(),
    pool_id: z.string(),
  });
}

export type ComponentValues = z.infer<ReturnType<typeof componentSchema>>;

export function toComponentValues(
  c: CatalogComponent | undefined,
  locales: string[],
): ComponentValues {
  return {
    code: c?.code ?? '',
    name: i18nFormValue(c?.name, locales),
    tracked: c?.tracked ?? false,
    pool_id: c?.pool_id ?? '',
  };
}

export function toComponentInput(v: ComponentValues): ComponentInput {
  return {
    code: v.code,
    name: compactI18n(v.name),
    tracked: v.tracked,
    ...(v.tracked && v.pool_id ? { pool_id: v.pool_id } : {}),
  };
}

// ---- Packs ---------------------------------------------------------------------------

export function packSchema(defaultLocale: string) {
  return z.object({
    code: codeField,
    name: i18nTextField(defaultLocale),
    description: optionalI18nText,
    image: z
      .string()
      .trim()
      .refine((v) => v === '' || /^https?:\/\/\S+$/i.test(v), V.url),
    composition: z
      .array(
        z.object({
          component_code: z.string().trim().min(1, V.required),
          quantity: intField({ min: 1, max: 100 }),
        }),
      )
      .min(1, V.required)
      .superRefine((rows, ctx) => {
        const seen = new Set<string>();
        rows.forEach((r, i) => {
          if (seen.has(r.component_code)) {
            ctx.addIssue({
              code: 'custom',
              message: 'configuration:validation.duplicate',
              path: [i, 'component_code'],
            });
          }
          seen.add(r.component_code);
        });
      }),
    requires_spot_kind: z.array(z.enum(SPOT_KINDS)),
    allowed_zone_ids: z.array(z.string()),
    allowed_formulas: nonEmpty(z.enum(FORMULAS)),
    channels: nonEmpty(z.enum(CHANNELS)),
    max_per_booking: intField({ optional: true, min: 1, max: 1000 }),
    requires_approval: z.boolean(),
    sort_order: intField({ optional: true, max: 1000 }),
    active: z.boolean(),
    vat_rate: optionalPercentField,
  });
}

export type PackValues = z.infer<ReturnType<typeof packSchema>>;

export function toPackValues(p: Pack | undefined, locales: string[], locale = 'it'): PackValues {
  return {
    code: p?.code ?? '',
    name: i18nFormValue(p?.name, locales),
    description: i18nFormValue(p?.description, locales),
    image: p?.image ?? '',
    composition: p
      ? Object.entries(p.composition).map(([component_code, q]) => ({
          component_code,
          quantity: String(q),
        }))
      : [],
    requires_spot_kind: p?.requires_spot_kind ?? [],
    allowed_zone_ids: p?.allowed_zone_ids ?? [],
    allowed_formulas: p?.allowed_formulas ?? ['half_day', 'day', 'multi_day'],
    channels: p?.channels ?? ['desk', 'online'],
    max_per_booking: p?.max_per_booking ? String(p.max_per_booking) : '',
    requires_approval: p?.requires_approval ?? false,
    sort_order: p ? String(p.sort_order) : '',
    active: p?.active ?? true,
    vat_rate: bpToVat(p?.vat_rate_bp, locale),
  };
}

export function toPackInput(v: PackValues): PackInput {
  const composition: Record<string, number> = {};
  for (const row of v.composition) composition[row.component_code] = Number(row.quantity);
  const description = compactI18n(v.description);
  return {
    code: v.code,
    name: compactI18n(v.name),
    ...(Object.keys(description).length ? { description } : {}),
    ...(v.image ? { image: v.image } : {}),
    composition,
    requires_spot_kind: listOrUndefined(v.requires_spot_kind),
    allowed_zone_ids: listOrUndefined(v.allowed_zone_ids),
    allowed_formulas: v.allowed_formulas,
    channels: v.channels,
    max_per_booking: optionalInt(v.max_per_booking),
    requires_approval: v.requires_approval,
    sort_order: optionalInt(v.sort_order),
    active: v.active,
    vat_rate_bp: vatToBp(v.vat_rate),
  };
}

export const packAliases = { vat_rate_bp: 'vat_rate' };

// ---- Extras --------------------------------------------------------------------------

export function extraSchema(defaultLocale: string) {
  return z.object({
    code: codeField,
    name: i18nTextField(defaultLocale),
    description: optionalI18nText,
    component_code: z.string(),
    pricing_unit: z.enum(PRICING_UNITS),
    max_quantity: intField({ optional: true, min: 1, max: 1000 }),
    attach_to: z.enum(ATTACH_TO),
    allowed_pack_ids: z.array(z.string()),
    channels: nonEmpty(z.enum(CHANNELS)),
    sort_order: intField({ optional: true, max: 1000 }),
    active: z.boolean(),
    vat_rate: optionalPercentField,
  });
}

export type ExtraValues = z.infer<ReturnType<typeof extraSchema>>;

export function toExtraValues(e: Extra | undefined, locales: string[], locale = 'it'): ExtraValues {
  return {
    code: e?.code ?? '',
    name: i18nFormValue(e?.name, locales),
    description: i18nFormValue(e?.description, locales),
    component_code: e?.component_code ?? '',
    pricing_unit: e?.pricing_unit ?? 'per_day',
    max_quantity: e?.max_quantity ? String(e.max_quantity) : '',
    attach_to: e?.attach_to ?? 'line',
    allowed_pack_ids: e?.allowed_pack_ids ?? [],
    channels: e?.channels ?? ['desk', 'online'],
    sort_order: e ? String(e.sort_order) : '',
    active: e?.active ?? true,
    vat_rate: bpToVat(e?.vat_rate_bp, locale),
  };
}

export function toExtraInput(v: ExtraValues): ExtraInput {
  const description = compactI18n(v.description);
  return {
    code: v.code,
    name: compactI18n(v.name),
    ...(Object.keys(description).length ? { description } : {}),
    ...(v.component_code ? { component_code: v.component_code } : {}),
    pricing_unit: v.pricing_unit,
    max_quantity: optionalInt(v.max_quantity),
    attach_to: v.attach_to,
    allowed_pack_ids: v.attach_to === 'line' ? listOrUndefined(v.allowed_pack_ids) : undefined,
    channels: v.channels,
    sort_order: optionalInt(v.sort_order),
    active: v.active,
    vat_rate_bp: vatToBp(v.vat_rate),
  };
}

export const extraAliases = { vat_rate_bp: 'vat_rate' };
