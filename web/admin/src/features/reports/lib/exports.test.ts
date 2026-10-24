import { describe, expect, it } from 'vitest';
import { exportListFileName, exportListUrl, toXlsx } from './exports';

const period = { from: '2027-06-01', to: '2027-06-30' } as Parameters<typeof exportListUrl>[3];

describe('exportListUrl', () => {
  it('adds the period only to bookings and payments', () => {
    expect(exportListUrl('e1', 'bookings', 'xlsx', period)).toBe(
      '/api/admin/v1/establishments/e1/exports/bookings?format=xlsx&from=2027-06-01&to=2027-06-30',
    );
    expect(exportListUrl('e1', 'customers', 'csv', period)).toBe(
      '/api/admin/v1/establishments/e1/exports/customers?format=csv',
    );
    expect(exportListFileName('payments', 'xlsx', period)).toBe(
      'arenile_payments_2027-06-01_2027-06-30.xlsx',
    );
    expect(exportListFileName('season-passes', 'csv', period)).toBe('arenile_season-passes.csv');
  });

  it('turns a CSV report link into the XLSX one', () => {
    expect(toXlsx('/r/revenue?format=csv&from=2027-06-01')).toBe(
      '/r/revenue?format=xlsx&from=2027-06-01',
    );
  });
});
