import type { ApiProblem, Schemas } from '@arenile/shared/api';
import { call, shouldRetry, useApiClient } from '@arenile/shared/api';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { configKey, createResource } from '../configuration/api/resource';
import { newIdempotencyKey } from '../configuration/lib/validation';

export type PriceRule = Schemas['PriceRule'];
export type PriceRuleInput = Schemas['PriceRuleInput'];
export type PriceAction = Schemas['PriceAction'];
export type PriceConditions = Schemas['PriceConditions'];
export type FixedPrice = Schemas['FixedPrice'];
export type FixedPriceInput = Schemas['FixedPriceInput'];
export type PromoCode = Schemas['PromoCode'];
export type PromoCodeInput = Schemas['PromoCodeInput'];
export type Agreement = Schemas['Agreement'];
export type AgreementInput = Schemas['AgreementInput'];
export type SimulateRequest = Schemas['SimulateRequest'];
export type SimulateResult = Schemas['SimulateResult'];
export type PriceMatrix = Schemas['PriceMatrix'];
export type CoverageReport = Schemas['CoverageReport'];
export type SeasonCopyRequest = Schemas['SeasonCopyRequest'];
export type SeasonCopyResult = Schemas['SeasonCopyResult'];
export type Channel = Schemas['Channel'];
export type Formula = Schemas['Formula'];
export type Slots = Schemas['Slots'];

const base = '/api/admin/v1/establishments/{establishmentId}' as const;

export const priceRules = createResource<PriceRule, PriceRuleInput, { season_id?: string }>({
  key: 'price-rules',
  collection: `${base}/price-rules`,
  item: `${base}/price-rules/{priceRuleId}`,
  idParam: 'priceRuleId',
});

export const fixedPrices = createResource<FixedPrice, FixedPriceInput, { season_id?: string }>({
  key: 'fixed-prices',
  collection: `${base}/fixed-prices`,
  item: `${base}/fixed-prices/{fixedPriceId}`,
  idParam: 'fixedPriceId',
});

export const promoCodes = createResource<PromoCode, PromoCodeInput>({
  key: 'promo-codes',
  collection: `${base}/promo-codes`,
  item: `${base}/promo-codes/{promoCodeId}`,
  idParam: 'promoCodeId',
});

export const agreements = createResource<Agreement, AgreementInput>({
  key: 'agreements',
  collection: `${base}/agreements`,
  item: `${base}/agreements/{agreementId}`,
  idParam: 'agreementId',
});

/** Simulator (POST, nothing written): breakdown, effective policy and deposit. */
export function useSimulate(establishmentId: string) {
  const client = useApiClient();
  return useMutation<SimulateResult, ApiProblem, SimulateRequest>({
    mutationFn: (body) =>
      call(() =>
        client.POST(`${base}/pricing/simulate`, {
          params: { path: { establishmentId } },
          body,
        }),
      ),
  });
}

export interface MatrixParams {
  season_id: string;
  formula?: Formula;
  channel?: Channel;
  slots?: Slots;
}

export function usePriceMatrix(establishmentId: string, params: MatrixParams | null) {
  const client = useApiClient();
  return useQuery<PriceMatrix, ApiProblem>({
    queryKey: [...configKey(establishmentId), 'pricing', 'matrix', params],
    queryFn: () =>
      call(() =>
        client.GET(`${base}/pricing/matrix`, {
          params: { path: { establishmentId }, query: params as MatrixParams },
        }),
      ),
    enabled: !!params?.season_id,
    retry: shouldRetry,
    placeholderData: keepPreviousData,
  });
}

export interface CoverageParams {
  season_id: string;
  channels?: Channel[];
  slots?: Slots;
}

export function useCoverage(establishmentId: string, params: CoverageParams | null) {
  const client = useApiClient();
  return useQuery<CoverageReport, ApiProblem>({
    queryKey: [...configKey(establishmentId), 'pricing', 'coverage', params],
    queryFn: () =>
      call(() =>
        client.GET(`${base}/pricing/coverage`, {
          params: { path: { establishmentId }, query: params as CoverageParams },
        }),
      ),
    enabled: !!params?.season_id,
    retry: shouldRetry,
  });
}

/** Season copy (spec 8.7): periods, rules, fixed prices and policies onto another year. */
export function useCopySeason(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<SeasonCopyResult, ApiProblem, { seasonId: string; body: SeasonCopyRequest }>({
    mutationFn: ({ seasonId, body }) =>
      call(() =>
        client.POST(`${base}/seasons/{seasonId}/copy`, {
          params: {
            path: { establishmentId, seasonId },
            header: { 'Idempotency-Key': newIdempotencyKey() },
          },
          body,
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: configKey(establishmentId) }),
  });
}
