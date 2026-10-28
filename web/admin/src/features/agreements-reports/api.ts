/**
 * Report per partner and agreements register (spec 30.2.5, ADR 0028): JSON
 * for the page, download URLs (cookie auth) for Excel/CSV and the printable
 * register (html, PDF through the browser print). Numbers come from the
 * server (money in integer cents); the page only formats them.
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
import type { Period } from '../partners/api';

export type PartnersReport = Schemas['PartnersReport'];
export type PartnersReportRow = Schemas['PartnersReportRow'];
export type AgreementsRegister = Schemas['AgreementsRegister'];
export type AgreementsRegisterEntry = Schemas['AgreementsRegisterEntry'];
export type AgreementsRegisterCategoryTotal = Schemas['AgreementsRegisterCategoryTotal'];

const BASE = '/api/admin/v1/establishments/{establishmentId}/reports' as const;

const keys = {
  partners: (est: string, period: Period | null) =>
    [...establishmentKeys.area(est, 'reports'), 'partners', period] as const,
  register: (est: string, period: Period | null, seasonId: string | undefined) =>
    [...establishmentKeys.area(est, 'reports'), 'agreements-register', period, seasonId] as const,
};

export function usePartnersReport(establishmentId: string, period: Period | null) {
  const client = useApiClient();
  return useQuery<PartnersReport, ApiProblem>({
    queryKey: keys.partners(establishmentId, period),
    queryFn: () =>
      call(() =>
        client.GET(`${BASE}/partners`, {
          params: { path: { establishmentId }, query: { ...(period as Period) } },
        }),
      ) as Promise<PartnersReport>,
    enabled: !!period,
    placeholderData: keepPreviousData,
    retry: shouldRetry,
  });
}

export function useAgreementsRegister(
  establishmentId: string,
  period: Period | null,
  seasonId?: string,
) {
  const client = useApiClient();
  return useQuery<AgreementsRegister, ApiProblem>({
    queryKey: keys.register(establishmentId, period, seasonId),
    queryFn: () =>
      call(() =>
        client.GET(`${BASE}/agreements-register`, {
          params: {
            path: { establishmentId },
            query: { ...(period as Period), ...(seasonId ? { season_id: seasonId } : {}) },
          },
        }),
      ) as Promise<AgreementsRegister>,
    enabled: !!period,
    placeholderData: keepPreviousData,
    retry: shouldRetry,
  });
}

export type ReportFile = 'csv' | 'xlsx' | 'html';

/** Download (csv/xlsx) or printable (html) URL of a report. */
export function reportFileUrl(
  establishmentId: string,
  report: 'partners' | 'agreements-register',
  period: Period,
  format: ReportFile,
  seasonId?: string,
): string {
  const q = new URLSearchParams({ from: period.from, to: period.to, format });
  if (seasonId) q.set('season_id', seasonId);
  return `/api/admin/v1/establishments/${encodeURIComponent(establishmentId)}/reports/${report}?${q.toString()}`;
}

/** "arenile_agreements-register_2026-06-01_2026-09-15.xlsx". */
export function reportFileName(report: string, period: Period, format: ReportFile): string {
  return `arenile_${report}_${period.from}_${period.to}.${format}`;
}
