/**
 * Server state of the reports feature (M5A, spec 15.3): types from the
 * generated schema, the query key factory and one TanStack Query hook per
 * report endpoint. Keys live under establishmentKeys.area(id, 'reports') so
 * that the SSE stream can invalidate them by prefix (booking.changed,
 * cash.changed). Every number is computed by the server (money in integer
 * cents, ratios in basis points); the page only formats it.
 */
import {
  call,
  establishmentKeys,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { keepPreviousData, useQuery } from '@tanstack/react-query';

export type ReportSummary = Schemas['ReportSummary'];
export type OccupancyReport = Schemas['OccupancyReport'];
export type OccupancyRow = Schemas['OccupancyRow'];
export type OccupancyGroupBy = Schemas['OccupancyGroupBy'];
export type RevenueReport = Schemas['RevenueReport'];
export type RevenueRow = Schemas['RevenueRow'];
export type RevenueGroupBy = Schemas['RevenueGroupBy'];
export type RevenueBasis = Schemas['RevenueBasis'];
export type BookingsReport = Schemas['BookingsReport'];
export type BookingsReportRow = Schemas['BookingsReportRow'];
export type BookingsReportGroupBy = Schemas['BookingsReportGroupBy'];
export type CancellationsReport = Schemas['CancellationsReport'];
export type CancellationsRow = Schemas['CancellationsRow'];
export type CancellationsGroupBy = Schemas['CancellationsGroupBy'];
export type SeasonPassesReport = Schemas['SeasonPassesReport'];
export type SeasonPassesRow = Schemas['SeasonPassesRow'];
export type SeasonPassesGroupBy = Schemas['SeasonPassesGroupBy'];
export type TopCustomersReport = Schemas['TopCustomersReport'];
export type TopCustomerRow = Schemas['TopCustomerRow'];
export type TopCustomersSort = Schemas['TopCustomersSort'];
export type OpenBalancesReport = Schemas['OpenBalancesReport'];
export type OpenBalanceRow = Schemas['OpenBalanceRow'];
export type YearComparisonReport = Schemas['YearComparisonReport'];
export type ComparisonPoint = Schemas['ComparisonPoint'];
export type ComparisonGranularity = Schemas['ComparisonGranularity'];
export type ComparisonAlign = Schemas['ComparisonAlign'];

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
export function reportCsvUrl<P extends ReportPeriod>(
  establishmentId: string,
  name: ReportName,
  params: P,
): string {
  const q = new URLSearchParams({ format: 'csv' });
  for (const [k, v] of Object.entries(params) as [string, unknown][]) {
    if (v !== undefined && v !== '') q.set(k, String(v));
  }
  return `/api/admin/v1/establishments/${encodeURIComponent(establishmentId)}/reports/${name}?${q.toString()}`;
}

const base = '/api/admin/v1/establishments/{establishmentId}/reports' as const;

/**
 * Shared query options of every report: disabled until a valid period is
 * chosen, previous data kept while a new grouping or period loads (no
 * flashing skeletons), no retry on 4xx.
 */
function useReport<P extends ReportPeriod, T>(
  establishmentId: string,
  name: ReportName,
  params: P | undefined,
  fetcher: (params: P) => Promise<T>,
  enabled: boolean,
) {
  return useQuery<T, ApiProblem>({
    queryKey: reportKeys.report(establishmentId, name, { ...params }),
    queryFn: () => fetcher(params as P),
    enabled: enabled && params !== undefined,
    placeholderData: keepPreviousData,
    retry: shouldRetry,
  });
}

export function useReportSummary(est: string, period: ReportPeriod | undefined, enabled = true) {
  const client = useApiClient();
  return useReport(
    est,
    'summary',
    period,
    ({ from, to }) =>
      call(() =>
        client.GET(`${base}/summary`, {
          params: { path: { establishmentId: est }, query: { from, to } },
        }),
      ) as Promise<ReportSummary>,
    enabled,
  );
}

export interface OccupancyParams extends ReportPeriod {
  group_by: OccupancyGroupBy;
}

export function useOccupancyReport(
  est: string,
  params: OccupancyParams | undefined,
  enabled = true,
) {
  const client = useApiClient();
  return useReport(
    est,
    'occupancy',
    params,
    (query) =>
      call(() =>
        client.GET(`${base}/occupancy`, { params: { path: { establishmentId: est }, query } }),
      ) as Promise<OccupancyReport>,
    enabled,
  );
}

export interface RevenueParams extends ReportPeriod {
  group_by: RevenueGroupBy;
  basis: RevenueBasis;
}

export function useRevenueReport(est: string, params: RevenueParams | undefined, enabled = true) {
  const client = useApiClient();
  return useReport(
    est,
    'revenue',
    params,
    (query) =>
      call(() =>
        client.GET(`${base}/revenue`, { params: { path: { establishmentId: est }, query } }),
      ) as Promise<RevenueReport>,
    enabled,
  );
}

export interface BookingsParams extends ReportPeriod {
  group_by: BookingsReportGroupBy;
}

export function useBookingsReport(est: string, params: BookingsParams | undefined, enabled = true) {
  const client = useApiClient();
  return useReport(
    est,
    'bookings',
    params,
    (query) =>
      call(() =>
        client.GET(`${base}/bookings`, { params: { path: { establishmentId: est }, query } }),
      ) as Promise<BookingsReport>,
    enabled,
  );
}

export interface CancellationsParams extends ReportPeriod {
  group_by: CancellationsGroupBy;
}

export function useCancellationsReport(
  est: string,
  params: CancellationsParams | undefined,
  enabled = true,
) {
  const client = useApiClient();
  return useReport(
    est,
    'cancellations',
    params,
    (query) =>
      call(() =>
        client.GET(`${base}/cancellations`, { params: { path: { establishmentId: est }, query } }),
      ) as Promise<CancellationsReport>,
    enabled,
  );
}

export interface SeasonPassesParams extends ReportPeriod {
  group_by: SeasonPassesGroupBy;
}

export function useSeasonPassesReport(
  est: string,
  params: SeasonPassesParams | undefined,
  enabled = true,
) {
  const client = useApiClient();
  return useReport(
    est,
    'season-passes',
    params,
    (query) =>
      call(() =>
        client.GET(`${base}/season-passes`, { params: { path: { establishmentId: est }, query } }),
      ) as Promise<SeasonPassesReport>,
    enabled,
  );
}

export interface TopCustomersParams extends ReportPeriod {
  sort: TopCustomersSort;
  limit: number;
}

export function useTopCustomersReport(
  est: string,
  params: TopCustomersParams | undefined,
  enabled = true,
) {
  const client = useApiClient();
  return useReport(
    est,
    'top-customers',
    params,
    (query) =>
      call(() =>
        client.GET(`${base}/top-customers`, { params: { path: { establishmentId: est }, query } }),
      ) as Promise<TopCustomersReport>,
    enabled,
  );
}

export function useOpenBalancesReport(
  est: string,
  period: ReportPeriod | undefined,
  enabled = true,
) {
  const client = useApiClient();
  return useReport(
    est,
    'open-balances',
    period,
    ({ from, to }) =>
      call(() =>
        client.GET(`${base}/open-balances`, {
          params: { path: { establishmentId: est }, query: { from, to } },
        }),
      ) as Promise<OpenBalancesReport>,
    enabled,
  );
}

export interface ComparisonParams extends ReportPeriod {
  compare_year: number;
  granularity: ComparisonGranularity;
  /** calendar (same day and month, default) or weekday (same weekday, same length). */
  align: ComparisonAlign;
}

export function useYearComparisonReport(
  est: string,
  params: ComparisonParams | undefined,
  enabled = true,
) {
  const client = useApiClient();
  return useReport(
    est,
    'comparison',
    params,
    (query) =>
      call(() =>
        client.GET(`${base}/comparison`, { params: { path: { establishmentId: est }, query } }),
      ) as Promise<YearComparisonReport>,
    enabled,
  );
}
