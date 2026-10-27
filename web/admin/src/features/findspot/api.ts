import {
  call,
  establishmentKeys,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

export type FindSpotRequest = Schemas['FindSpotRequest'];
export type FindSpotResult = Schemas['FindSpotResult'];
export type FindSpotProposal = Schemas['FindSpotProposal'];
export type FindSpotBookRequest = Schemas['FindSpotBookRequest'];
export type BookingDetail = Schemas['BookingDetail'];

const EST = '/api/admin/v1/establishments/{establishmentId}' as const;

export const findSpotKeys = {
  search: (establishmentId: string, body: FindSpotRequest | null) =>
    [...establishmentKeys.area(establishmentId, 'availability'), 'find-spot', body] as const,
};

/**
 * "Trova posto" (spec 30.6): ranked proposals for the request (nothing is
 * written). Under the availability area, so a booking made meanwhile
 * (SSE) refreshes the open results. `body` null = no search yet.
 */
export function useFindSpot(establishmentId: string, body: FindSpotRequest | null) {
  const client = useApiClient();
  return useQuery<FindSpotResult, ApiProblem>({
    queryKey: findSpotKeys.search(establishmentId, body),
    queryFn: () =>
      call(() =>
        client.POST(`${EST}/find-spot`, {
          params: { path: { establishmentId } },
          body: body as FindSpotRequest,
        }),
      ),
    enabled: body !== null,
    retry: shouldRetry,
    staleTime: 10_000,
  });
}

/** Areas a new booking touches. */
const AREAS = ['bookings', 'operations', 'planning', 'availability', 'customers'] as const;

/**
 * Books a proposal: one booking whose lines carry the proposal segments, in
 * one transaction (409 spot_unavailable when a half-day was taken meanwhile,
 * 409 price_changed when the total differs from expected_total).
 */
export function useBookProposal(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<
    BookingDetail,
    ApiProblem,
    { body: FindSpotBookRequest; idempotencyKey: string }
  >({
    mutationFn: ({ body, idempotencyKey }) =>
      call(() =>
        client.POST(`${EST}/find-spot/book`, {
          params: { path: { establishmentId }, header: { 'Idempotency-Key': idempotencyKey } },
          body,
        }),
      ),
    onSettled: () => {
      for (const area of AREAS) {
        void qc.invalidateQueries({ queryKey: establishmentKeys.area(establishmentId, area) });
      }
    },
  });
}
