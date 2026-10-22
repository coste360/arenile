import { z } from 'zod';
import type { Pool, PoolInput, Zone, ZoneInput } from '../api/resources';
import { compactI18n, i18nFormValue } from '../lib/i18nText';
import {
  V,
  codeField,
  i18nTextField,
  intField,
  isoDate,
  isCalendarDate,
  optionalInt,
} from '../lib/validation';

// ---- Zones (spec 5.2, 7.5) --------------------------------------------------------------

export const ASSIGNMENT_MODES = ['seat', 'zone', 'quantity'] as const;
export const QUOTA_TYPES = ['none', 'percent', 'count'] as const;

export function zoneSchema(defaultLocale: string) {
  return z
    .object({
      map_id: z.string(),
      code: codeField,
      name: i18nTextField(defaultLocale),
      assignment_mode: z.enum(ASSIGNMENT_MODES),
      online_visible: z.boolean(),
      quota_type: z.enum(QUOTA_TYPES),
      quota_value: z.string().trim(),
      price_category: codeField,
      auto_assign: z.boolean(),
      sort_order: intField({ optional: true, max: 1000 }),
    })
    .superRefine((v, ctx) => {
      if (v.quota_type === 'none') return;
      const n = Number(v.quota_value);
      const max = v.quota_type === 'percent' ? 100 : 100_000;
      if (!/^\d+$/.test(v.quota_value) || n < 0 || n > max) {
        ctx.addIssue({
          code: 'custom',
          message: v.quota_type === 'percent' ? V.percent : V.integer,
          path: ['quota_value'],
        });
      }
    });
}

export type ZoneValues = z.infer<ReturnType<typeof zoneSchema>>;

export function toZoneValues(zone?: Zone, locales: string[] = ['it'], count = 0): ZoneValues {
  if (zone) {
    return {
      map_id: zone.map_id ?? '',
      code: zone.code,
      name: i18nFormValue(zone.name, locales),
      assignment_mode: zone.assignment_mode,
      online_visible: zone.online_visible,
      quota_type: zone.online_quota?.type ?? 'none',
      quota_value: zone.online_quota ? String(zone.online_quota.value) : '',
      price_category: zone.price_category,
      auto_assign: zone.auto_assign,
      sort_order: String(zone.sort_order),
    };
  }
  return {
    map_id: '',
    code: '',
    name: i18nFormValue({}, locales),
    assignment_mode: 'seat',
    online_visible: true,
    quota_type: 'none',
    quota_value: '',
    price_category: `row${count + 1}`,
    auto_assign: false,
    sort_order: String(count),
  };
}

export function toZoneInput(v: ZoneValues): ZoneInput {
  return {
    ...(v.map_id && v.assignment_mode !== 'quantity' ? { map_id: v.map_id } : {}),
    code: v.code,
    name: compactI18n(v.name),
    assignment_mode: v.assignment_mode,
    online_visible: v.online_visible,
    ...(v.quota_type !== 'none'
      ? { online_quota: { type: v.quota_type, value: Number(v.quota_value) } }
      : {}),
    price_category: v.price_category,
    auto_assign: v.assignment_mode === 'zone' ? v.auto_assign : false,
    sort_order: optionalInt(v.sort_order),
  };
}

export const zoneAliases = {
  online_quota: 'quota_value',
};

// ---- Pools ---------------------------------------------------------------------------

export function poolSchema(defaultLocale: string) {
  return z.object({
    zone_id: z.string(),
    component_code: z.string().trim().min(1, V.required),
    name: i18nTextField(defaultLocale),
    capacity: intField({ max: 100_000 }),
    capacity_overrides: z.array(
      z
        .object({ from: isoDate, to: isoDate, capacity: intField({ max: 100_000 }) })
        .refine((o) => !isCalendarDate(o.from) || !isCalendarDate(o.to) || o.from <= o.to, {
          message: V.dateOrder,
          path: ['to'],
        }),
    ),
  });
}

export type PoolValues = z.infer<ReturnType<typeof poolSchema>>;

export function toPoolValues(p?: Pool, locales: string[] = ['it']): PoolValues {
  if (p) {
    return {
      zone_id: p.zone_id ?? '',
      component_code: p.component_code,
      name: i18nFormValue(p.name, locales),
      capacity: String(p.capacity),
      capacity_overrides: (p.capacity_overrides ?? []).map((o) => ({
        from: o.from,
        to: o.to,
        capacity: String(o.capacity),
      })),
    };
  }
  return {
    zone_id: '',
    component_code: '',
    name: i18nFormValue({}, locales),
    capacity: '',
    capacity_overrides: [],
  };
}

export function toPoolInput(v: PoolValues): PoolInput {
  return {
    ...(v.zone_id ? { zone_id: v.zone_id } : {}),
    component_code: v.component_code,
    name: compactI18n(v.name),
    capacity: Number(v.capacity),
    // Nullable lists: nil means "none", never an empty list.
    ...(v.capacity_overrides.length
      ? {
          capacity_overrides: v.capacity_overrides.map((o) => ({
            from: o.from,
            to: o.to,
            capacity: Number(o.capacity),
          })),
        }
      : {}),
  };
}
