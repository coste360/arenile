import {
  establishmentKeys,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import {
  keepPreviousData,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import { ifMatch, newIdempotencyKey } from '../configuration/lib/validation';
import { callBooking } from '../bookings/problem';

export type SeasonPass = Schemas['SeasonPass'];
export type SeasonPassList = Schemas['SeasonPassList'];
export type SeasonPassDetail = Schemas['SeasonPassDetail'];
export type SeasonPassCreate = Schemas['SeasonPassCreate'];
export type SeasonRelease = Schemas['SeasonRelease'];
export type SeasonReleaseCreate = Schemas['SeasonReleaseCreate'];
export type SeasonReleaseList = Schemas['SeasonReleaseList'];

const BASE = '/api/admin/v1/establishments/{establishmentId}' as const;

export interface PassFilters {
  season_id?: string;
  customer_id?: string;
  spot_id?: string;
}

/** Query keys under the `season-passes` area (invalidated by booking/allocation SSE topics). */
export const passKeys = {
  all: (est: string) => establishmentKeys.area(est, 'season-passes'),
  list: (est: string, f: PassFilters) => [...passKeys.all(est), 'list', f] as const,
  detail: (est: string, id: string) => [...passKeys.all(est), 'detail', id] as const,
};

/** Every pass of the filter (cursor pages of 200 loaded on demand). */
export function useSeasonPasses(establishmentId: string, filters: PassFilters, enabled = true) {
  const client = useApiClient();
  return useInfiniteQuery<SeasonPassList, ApiProblem>({
    queryKey: passKeys.list(establishmentId, filters),
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      callBooking(() =>
        client.GET(`${BASE}/season-passes`, {
          params: {
            path: { establishmentId },
            query: { ...filters, cursor: pageParam as string | undefined, limit: 200 },
          },
        }),
      ),
    getNextPageParam: (last) => last.next_cursor || undefined,
    placeholderData: keepPreviousData,
    enabled,
    retry: shouldRetry,
  });
}

export function useSeasonPass(establishmentId: string, passId: string | undefined) {
  const client = useApiClient();
  return useQuery<SeasonPassDetail, ApiProblem>({
    queryKey: passKeys.detail(establishmentId, passId ?? ''),
    queryFn: () =>
      callBooking(() =>
        client.GET(`${BASE}/season-passes/{passId}`, {
          params: { path: { establishmentId, passId: passId as string } },
        }),
      ),
    enabled: !!passId,
    retry: shouldRetry,
  });
}

/** A pass, a release or a revoke changes the map, the planning and the bookings too. */
function useInvalidate(establishmentId: string) {
  const qc = useQueryClient();
  return () => {
    for (const area of [
      'season-passes',
      'bookings',
      'operations',
      'planning',
      'availability',
      'customers',
    ] as const) {
      void qc.invalidateQueries({ queryKey: establishmentKeys.area(establishmentId, area) });
    }
  };
}

export function useCreateSeasonPass(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidate(establishmentId);
  return useMutation<
    SeasonPassDetail,
    ApiProblem,
    { body: SeasonPassCreate; idempotencyKey: string }
  >({
    mutationFn: ({ body, idempotencyKey }) =>
      callBooking(() =>
        client.POST(`${BASE}/season-passes`, {
          params: { path: { establishmentId }, header: { 'Idempotency-Key': idempotencyKey } },
          body,
        }),
      ),
    onSuccess: () => invalidate(),
  });
}

/** Releases days of a pass: the spot becomes bookable on them (spec 7.7, INV-10). */
export function useCreateReleases(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidate(establishmentId);
  return useMutation<SeasonReleaseList, ApiProblem, { passId: string; body: SeasonReleaseCreate }>({
    mutationFn: ({ passId, body }) =>
      callBooking(() =>
        client.POST(`${BASE}/season-passes/{passId}/releases`, {
          params: {
            path: { establishmentId, passId },
            header: { 'Idempotency-Key': newIdempotencyKey() },
          },
          body,
        }),
      ),
    onSuccess: () => invalidate(),
  });
}

/** Revokes a release not yet resold (409 conflict otherwise): the season allocations come back. */
export function useRevokeRelease(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidate(establishmentId);
  return useMutation<void, ApiProblem, { passId: string; releaseId: string }>({
    mutationFn: async ({ passId, releaseId }) => {
      await callBooking(() =>
        client.DELETE(`${BASE}/season-passes/{passId}/releases/{releaseId}`, {
          params: { path: { establishmentId, passId, releaseId } },
        }),
      );
    },
    onSuccess: () => invalidate(),
  });
}

export type SeasonPassPriceOverrideInput = Schemas['SeasonPassPriceOverrideInput'];

/**
 * Manual price of a pass (ADR 0019 E): owner and manager only
 * (establishment.configure), audited; `null` body = back to the price list.
 */
export function usePassPriceOverride(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidate(establishmentId);
  return useMutation<
    SeasonPassDetail,
    ApiProblem,
    { passId: string; version: number; body: SeasonPassPriceOverrideInput | null }
  >({
    mutationFn: ({ passId, version, body }) =>
      callBooking(() => {
        const params = {
          path: { establishmentId, passId },
          header: { 'If-Match': ifMatch(version) },
        };
        return body
          ? client.PUT(`${BASE}/season-passes/{passId}/price-override`, { params, body })
          : client.DELETE(`${BASE}/season-passes/{passId}/price-override`, { params });
      }),
    onSuccess: () => invalidate(),
  });
}
