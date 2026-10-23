import { describe, expect, it } from 'vitest';
import { reportCsvUrl } from './api';

describe('reportCsvUrl', () => {
  it('builds the CSV export link with the period and skips empty params', () => {
    expect(
      reportCsvUrl('e1', 'revenue', {
        from: '2027-06-01',
        to: '2027-06-30',
        group_by: 'channel',
        basis: undefined,
      }),
    ).toBe(
      '/api/admin/v1/establishments/e1/reports/revenue?format=csv&from=2027-06-01&to=2027-06-30&group_by=channel',
    );
  });
});
