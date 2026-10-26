import { describe, expect, it } from 'vitest';
import {
  emptyGrid,
  pricelistSchema,
  rowsDefaults,
  rowsSchema,
  templateHasRows,
  toApplyTemplateRequest,
  toQuickPricelistRequest,
} from './wizard.schema';

describe('row generator', () => {
  it('defaults to 8 × 15 (spec 28 M1 acceptance) and builds the request', () => {
    const v = rowsDefaults(true, 2027);
    expect(v).toMatchObject({ rows: '8', spots_per_row: '15', opens_on: '2027-05-01' });
    expect(rowsSchema.safeParse(v).success).toBe(true);
    expect(toApplyTemplateRequest('classic_rows', v, { it: 'Spiaggia' })).toEqual({
      template: 'classic_rows',
      season_year: 2027,
      opens_on: '2027-05-01',
      closes_on: '2027-09-30',
      rows: 8,
      spots_per_row: 15,
      map_name: { it: 'Spiaggia' },
    });
  });

  it('omits rows for quantity templates', () => {
    const v = rowsDefaults(false, 2027);
    expect(rowsSchema.safeParse(v).success).toBe(true);
    expect(toApplyTemplateRequest('quantity_beach', v)).not.toHaveProperty('rows');
    expect(templateHasRows('quantity_beach')).toBe(false);
    expect(templateHasRows('lido_with_pool')).toBe(true);
    expect(templateHasRows('classic_rows', 0)).toBe(false);
  });

  it('validates bounds and dates', () => {
    const r = rowsSchema.safeParse({
      ...rowsDefaults(true, 2027),
      rows: '0',
      closes_on: '2027-04-01',
    });
    expect(r.success).toBe(false);
    expect(r.error?.issues.map((i) => i.path.join('.')).sort()).toEqual(['closes_on', 'rows']);
  });

  it('refuses a season longer than a year, as apply-template does', () => {
    const r = rowsSchema.safeParse({ ...rowsDefaults(true, 2026), closes_on: '2027-09-30' });
    expect(r.error?.issues.map((i) => i.message)).toEqual([
      'configuration:validation.seasonMaxOneYear',
    ]);
  });
});

describe('quick price list', () => {
  it('converts euro inputs to integer cents and drops empty cells', () => {
    const v = {
      cells: [
        ['25', '30,50', ''],
        ['19.99', '', '0,5'],
      ],
    };
    expect(pricelistSchema.safeParse(v).success).toBe(true);
    expect(toQuickPricelistRequest('s1', ['row1', 'row2'], ['low', 'mid', 'high'], v)).toEqual({
      season_id: 's1',
      replace_existing: true,
      prices: [
        { price_category: 'row1', rate_period_code: 'low', amount: 2500 },
        { price_category: 'row1', rate_period_code: 'mid', amount: 3050 },
        { price_category: 'row2', rate_period_code: 'low', amount: 1999 },
        { price_category: 'row2', rate_period_code: 'high', amount: 50 },
      ],
    });
  });

  it('requires at least one valid price', () => {
    const empty = pricelistSchema.safeParse(emptyGrid(2, 2));
    expect(empty.error?.issues[0]?.message).toBe('setupWizard:pricelist.atLeastOne');
    const bad = pricelistSchema.safeParse({ cells: [['12,345']] });
    expect(bad.error?.issues[0]?.path).toEqual(['cells', 0, 0]);
  });
});
