/**
 * Server state of the reports feature (M5A): types from the generated
 * schema and the query key factory. Keys live under
 * establishmentKeys.area(id, 'reports') so that the SSE stream can
 * invalidate them by prefix (booking.changed, cash.changed). Hooks
 * (useReportSummary, useOccupancyReport, ...) are added here by the reports
 * frontend builder with `call(() => client.GET(`${base}/reports/...`))`.
 */
import { establishmentKeys, type Schemas } from '@arenile/shared/api';

export type ReportSummary = Schemas['ReportSummary'];
export type OccupancyReport = Schemas['OccupancyReport'];
export type RevenueReport = Schemas['RevenueReport'];
export type BookingsReport = Schemas['BookingsReport'];
export type CancellationsReport = Schemas['CancellationsReport'];
export type SeasonPassesReport = Schemas['SeasonPassesReport'];
export type TopCustomersReport = Schemas['TopCustomersReport'];
export type OpenBalancesReport = Schemas['OpenBalancesReport'];
export type YearComparisonReport = Schemas['YearComparisonReport'];

/** Report endpoints (path segment after /reports/). */
export type ReportName =
  | 'summary'
  | 'occupancy'
  | 'revenue'
  | 'bookings'
  | 'cancellations'
  | 'season-passes'
  | 'top-customers'
  | 'open-balances'
  | 'comparison';

/** Inclusive period in the establishment timezone (YYYY-MM-DD). */
export interface ReportPeriod {
  from: string;
  to: string;
}

export const reportKeys = {
  all: (est: string) => establishmentKeys.area(est, 'reports'),
  report: (est: string, name: ReportName, params: Record<string, unknown>) =>
    [...establishmentKeys.area(est, 'reports'), name, params] as const,
};

/** URL of the CSV export of a report (download link, cookie auth). */
export function reportCsvUrl(
  establishmentId: string,
  name: ReportName,
  params: Record<string, string | number | undefined>,
): string {
  const q = new URLSearchParams({ format: 'csv' });
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== '') q.set(k, String(v));
  }
  return `/api/admin/v1/establishments/${encodeURIComponent(establishmentId)}/reports/${name}?${q.toString()}`;
}
