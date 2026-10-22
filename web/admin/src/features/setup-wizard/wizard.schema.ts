import { z } from 'zod';
import { euroToCents } from '../configuration/lib/money';
import { V, intField, isCalendarDate, isoDate } from '../configuration/lib/validation';
import type { ApplyTemplateRequest, QuickPricelistRequest } from './api';
import type { TemplateCode } from './progress';

// ---- Step 3: row generator (apply-template) ----------------------------------------

/** Templates with a map of rows; the others create zones by quantity only. */
export function templateHasRows(template: TemplateCode, templateRows?: number): boolean {
  if (templateRows !== undefined) return templateRows > 0;
  return template === 'classic_rows' || template === 'lido_with_pool';
}

export const rowsSchema = z
  .object({
    withRows: z.boolean(),
    rows: z.string().trim(),
    spots_per_row: z.string().trim(),
    season_year: intField({ min: 2000, max: 2100 }),
    opens_on: isoDate,
    closes_on: isoDate,
  })
  .superRefine((v, ctx) => {
    if (v.withRows) {
      for (const [key, max] of [
        ['rows', 50],
        ['spots_per_row', 100],
      ] as const) {
        const n = Number(v[key]);
        if (!/^\d+$/.test(v[key]) || n < 1 || n > max) {
          ctx.addIssue({ code: 'custom', message: V.integer, path: [key] });
        }
      }
    }
    if (isCalendarDate(v.opens_on) && isCalendarDate(v.closes_on) && v.opens_on > v.closes_on) {
      ctx.addIssue({ code: 'custom', message: V.dateOrder, path: ['closes_on'] });
    }
  });

export type RowsValues = z.infer<typeof rowsSchema>;

export function rowsDefaults(
  withRows: boolean,
  year: number,
  rows = 8,
  spotsPerRow = 15,
): RowsValues {
  return {
    withRows,
    rows: withRows ? String(rows || 8) : '',
    spots_per_row: withRows ? String(spotsPerRow || 15) : '',
    season_year: String(year),
    opens_on: `${year}-05-01`,
    closes_on: `${year}-09-30`,
  };
}

export function toApplyTemplateRequest(
  template: TemplateCode,
  v: RowsValues,
  mapName?: Record<string, string>,
): ApplyTemplateRequest {
  return {
    template,
    season_year: Number(v.season_year),
    opens_on: v.opens_on,
    closes_on: v.closes_on,
    ...(v.withRows
      ? {
          rows: Number(v.rows),
          spots_per_row: Number(v.spots_per_row),
          ...(mapName ? { map_name: mapName } : {}),
        }
      : {}),
  };
}

// ---- Step 4: quick price list (price category × rate period) --------------------------

export const pricelistSchema = z
  .object({
    /** cells[i][j] = daily price of category i in period j, in euro as typed. */
    cells: z.array(
      z.array(
        z
          .string()
          .trim()
          .refine((v) => v === '' || euroToCents(v) !== null, V.amount),
      ),
    ),
  })
  .refine((v) => v.cells.some((row) => row.some((c) => c.trim() !== '')), {
    message: 'setupWizard:pricelist.atLeastOne',
    path: ['cells'],
  });

export type PricelistValues = z.infer<typeof pricelistSchema>;

export function emptyGrid(categories: number, periods: number): PricelistValues {
  return { cells: Array.from({ length: categories }, () => Array<string>(periods).fill('')) };
}

/** Grid → request; empty cells are omitted, euro strings become integer cents. */
export function toQuickPricelistRequest(
  seasonId: string,
  categories: string[],
  periodCodes: string[],
  v: PricelistValues,
): QuickPricelistRequest {
  const prices: QuickPricelistRequest['prices'] = [];
  categories.forEach((category, i) => {
    periodCodes.forEach((code, j) => {
      const raw = v.cells[i]?.[j] ?? '';
      const cents = raw.trim() === '' ? null : euroToCents(raw);
      if (cents !== null) {
        prices.push({ price_category: category, rate_period_code: code, amount: cents });
      }
    });
  });
  return { season_id: seasonId, prices, replace_existing: true };
}
