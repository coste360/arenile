import { call, useApiClient, type ApiProblem, type Schemas } from '@arenile/shared/api';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { publicKeys, type ManageBooking } from '../../api/public';

/**
 * Magic-link hooks of the M5B manage additions (spec 10.4, 13.4): accept or
 * decline the renewal proposal of the held booking, use the credit balance
 * on the balance due. The token travels only in the path; never logged.
 */
export type PublicRenewalProposal = Schemas['PublicRenewalProposal'];
export type PublicRenewalAcceptRequest = Schemas['PublicRenewalAcceptRequest'];
export type PublicRenewalAcceptResult = Schemas['PublicRenewalAcceptResult'];

export function useRefreshManage(slug: string, token: string) {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: publicKeys.manage(slug, token) });
}

export function useAcceptRenewal(slug: string, token: string) {
  const client = useApiClient();
  return useMutation<
    PublicRenewalAcceptResult,
    ApiProblem,
    { body: PublicRenewalAcceptRequest; idempotencyKey: string }
  >({
    mutationFn: ({ body, idempotencyKey }) =>
      call(() =>
        client.POST('/api/public/v1/{slug}/manage/{token}/renewal/accept', {
          params: { path: { slug, token }, header: { 'Idempotency-Key': idempotencyKey } },
          body,
        }),
      ),
  });
}

export function useDeclineRenewal(slug: string, token: string) {
  const client = useApiClient();
  const refresh = useRefreshManage(slug, token);
  return useMutation<PublicRenewalProposal, ApiProblem, { reason?: string }>({
    mutationFn: (body) =>
      call(() =>
        client.POST('/api/public/v1/{slug}/manage/{token}/renewal/decline', {
          params: { path: { slug, token } },
          body,
        }),
      ),
    onSettled: () => void refresh(),
  });
}

/** Applies the credit balance to the balance due; the server answers with the updated booking. */
export function useApplyCredit(slug: string, token: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<ManageBooking, ApiProblem, { amount?: number; idempotencyKey: string }>({
    mutationFn: ({ amount, idempotencyKey }) =>
      call(() =>
        client.POST('/api/public/v1/{slug}/manage/{token}/apply-credit', {
          params: { path: { slug, token }, header: { 'Idempotency-Key': idempotencyKey } },
          body: amount === undefined ? {} : { amount },
        }),
      ),
    onSuccess: (booking) => qc.setQueryData(publicKeys.manage(slug, token), booking),
  });
}
