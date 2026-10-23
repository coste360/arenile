import { z } from 'zod';
import { V, i18nTextField, intField } from '../configuration/lib/validation';
import {
  SPOT_KINDS,
  type DraftMapSettings,
  type RowGeneratorRequest,
  type SeaSide,
  type SpotKind,
} from './model/types';

/** Finite number typed as text (dot or comma decimal separator). */
const numberText = (opts: { min?: number; positive?: boolean } = {}) =>
  z
    .string()
    .trim()
    .refine((v) => {
      const n = Number(v.replace(',', '.'));
      if (v === '' || !Number.isFinite(n)) return false;
      if (opts.positive && n <= 0) return false;
      return opts.min === undefined || n >= opts.min;
    }, 'mapEditor:validation.number');

const toNumber = (v: string) => Number(v.replace(',', '.'));

export const generatorSchema = z.object({
  zone_id: z.string().min(1, V.required),
  rows: intField({ min: 1, max: 52 }),
  spots_per_row: intField({ min: 1, max: 200 }),
  origin_x: numberText({ min: 0 }),
  origin_y: numberText({ min: 0 }),
  spacing_x: numberText({ positive: true }),
  spacing_y: numberText({ positive: true }),
  row_label_style: z.enum(['letters', 'numbers']),
  first_row_label: z.string().trim().max(4),
  number_start: intField({ min: 0, max: 9999 }),
  numbering: z.enum(['per_row', 'continuous']),
  direction: z.enum(['ltr', 'rtl']),
  label_format: z
    .string()
    .trim()
    .max(32)
    .refine((v) => v === '' || v.includes('{number}'), 'mapEditor:validation.labelFormat'),
  kind: z.enum(SPOT_KINDS),
  /** Kit illustration of the generated spots; '' = default of the kind. */
  asset: z.string(),
});

export type GeneratorValues = z.infer<typeof generatorSchema>;

/** Defaults: rows start near the sea at the first free grid position. */
export function generatorDefaults(
  map: Pick<DraftMapSettings, 'width' | 'height' | 'grid_size' | 'sea_side'>,
  zoneId: string,
): GeneratorValues {
  const g = map.grid_size > 0 ? map.grid_size : 10;
  const spacing = Math.max(g, Math.round(60 / g) * g);
  const margin = spacing;
  const origin: Record<SeaSide, [number, number]> = {
    bottom: [margin, map.height - margin],
    top: [margin, margin],
    left: [margin, margin],
    right: [map.width - margin, margin],
  };
  const [x, y] = origin[map.sea_side];
  return {
    zone_id: zoneId,
    rows: '1',
    spots_per_row: '10',
    origin_x: String(Math.max(0, x)),
    origin_y: String(Math.max(0, y)),
    spacing_x: String(spacing),
    spacing_y: String(spacing),
    row_label_style: 'letters',
    first_row_label: '',
    number_start: '1',
    numbering: 'per_row',
    direction: 'ltr',
    label_format: '',
    kind: 'umbrella',
    asset: '',
  };
}

export function toGeneratorRequest(v: GeneratorValues): RowGeneratorRequest {
  const req: RowGeneratorRequest = {
    zone_id: v.zone_id,
    rows: Number(v.rows),
    spots_per_row: Number(v.spots_per_row),
    origin_x: toNumber(v.origin_x),
    origin_y: toNumber(v.origin_y),
    spacing_x: toNumber(v.spacing_x),
    spacing_y: toNumber(v.spacing_y),
    row_label_style: v.row_label_style,
    number_start: Number(v.number_start),
    numbering: v.numbering,
    direction: v.direction,
    kind: v.kind as SpotKind,
  };
  if (v.first_row_label) req.first_row_label = v.first_row_label;
  if (v.label_format) req.label_format = v.label_format;
  if (v.asset) req.asset = v.asset;
  return req;
}

export const mapSettingsSchema = (defaultLocale: string) =>
  z.object({
    name: i18nTextField(defaultLocale),
    width: intField({ min: 50, max: 100000 }),
    height: intField({ min: 50, max: 100000 }),
    grid_size: intField({ min: 1, max: 1000 }),
    sea_side: z.enum(['top', 'bottom', 'left', 'right']),
  });

export type MapSettingsValues = z.infer<ReturnType<typeof mapSettingsSchema>>;

export function mapSettingsDefaults(
  m: Pick<DraftMapSettings, 'name' | 'width' | 'height' | 'grid_size' | 'sea_side'> | undefined,
  locales: readonly string[],
): MapSettingsValues {
  const name: Record<string, string> = {};
  for (const l of locales) name[l] = m?.name[l] ?? '';
  return {
    name,
    width: String(m?.width ?? 1200),
    height: String(m?.height ?? 800),
    grid_size: String(m?.grid_size ?? 10),
    // Italian lidos usually draw the sea at the top (spec 11 default).
    sea_side: m?.sea_side ?? 'top',
  };
}

export function toMapSettings(v: MapSettingsValues) {
  const name: Record<string, string> = {};
  for (const [k, val] of Object.entries(v.name)) if (val.trim()) name[k] = val.trim();
  return {
    name,
    width: Number(v.width),
    height: Number(v.height),
    grid_size: Number(v.grid_size),
    sea_side: v.sea_side as SeaSide,
  };
}

/** Renumbering of the selected spots as one row (spec 11.2). */
export const renumberSchema = z.object({
  row_label: z.string().trim().min(1, V.required).max(4),
  start: intField({ min: 0, max: 9999 }),
  reverse: z.boolean(),
  format: z
    .string()
    .trim()
    .max(32)
    .refine((v) => v === '' || v.includes('{number}'), 'mapEditor:validation.labelFormat'),
});

export type RenumberValues = z.infer<typeof renumberSchema>;

export function renumberDefaults(spots: readonly { row_label: string; number: number | null }[]) {
  const row = spots.find((s) => s.row_label)?.row_label ?? 'A';
  const numbers = spots.map((s) => s.number).filter((n): n is number => n != null);
  return {
    row_label: row,
    start: String(numbers.length ? Math.min(...numbers) : 1),
    reverse: false,
    format: '',
  } satisfies RenumberValues;
}
