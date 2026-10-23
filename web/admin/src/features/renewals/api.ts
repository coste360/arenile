/**
 * Server state of the renewals feature (M5B: renewal campaigns and proposals,
 * spec 10.4): types from the generated schema, query keys under the
 * `renewals` area (refreshed by the booking SSE topic) and the TanStack Query
 * hooks.
 */
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
import { callBooking } from '../bookings/problem';
import { ifMatch, newIdempotencyKey } from '../configuration/lib/validation';

export type RenewalCampaign = Schemas['RenewalCampaign'];
export type RenewalCampaignList = Schemas['RenewalCampaignList'];
export type RenewalCampaignCreate = Schemas['RenewalCampaignCreate'];
export type RenewalCampaignUpdate = Schemas['RenewalCampaignUpdate'];
export type RenewalCampaignResult = Schemas['RenewalCampaignResult'];
export type RenewalCampaignStatus = Schemas['RenewalCampaignStatus'];
export type RenewalProposal = Schemas['RenewalProposal'];
export type RenewalProposalList = Schemas['RenewalProposalList'];
export type RenewalProposalStatus = Schemas['RenewalProposalStatus'];
export type RenewalBulkRequest = Schemas['RenewalBulkRequest'];
export type RenewalBulkResult = Schemas['RenewalBulkResult'];
export type RenewalBulkAction = Schemas['RenewalBulkAction'];
export type RenewalStaffAcceptRequest = Schemas['RenewalStaffAcceptRequest'];
export type RenewalDeclineRequest = Schemas['RenewalDeclineRequest'];
export type RenewalSkipped = Schemas['RenewalSkipped'];
export type RenewalProposalPreview = Schemas['RenewalProposalPreview'];

const BASE = '/api/admin/v1/establishments/{establishmentId}' as const;

export interface ProposalFilters {
  status?: RenewalProposalStatus;
  sent?: boolean;
  zone_id?: string;
  q?: string;
}

export const renewalKeys = {
  all: (est: string) => establishmentKeys.area(est, 'renewals'),
  campaigns: (est: string) => [...renewalKeys.all(est), 'campaigns'] as const,
  proposals: (est: string, campaignId: string, f: ProposalFilters) =>
    [...renewalKeys.all(est), 'proposals', campaignId, f] as const,
};

/** Every campaign of the establishment (a handful per year: one page of 200). */
export function useRenewalCampaigns(establishmentId: string) {
  const client = useApiClient();
  return useQuery<RenewalCampaignList, ApiProblem>({
    queryKey: renewalKeys.campaigns(establishmentId),
    queryFn: () =>
      callBooking(() =>
        client.GET(`${BASE}/renewal-campaigns`, {
          params: { path: { establishmentId }, query: { limit: 200 } },
        }),
      ),
    retry: shouldRetry,
  });
}

/** Proposals of a campaign, filtered server side (cursor pages of 200). */
export function useRenewalProposals(
  establishmentId: string,
  campaignId: string | undefined,
  filters: ProposalFilters,
) {
  const client = useApiClient();
  return useInfiniteQuery<RenewalProposalList, ApiProblem>({
    queryKey: renewalKeys.proposals(establishmentId, campaignId ?? '', filters),
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      callBooking(() =>
        client.GET(`${BASE}/renewal-campaigns/{campaignId}/proposals`, {
          params: {
            path: { establishmentId, campaignId: campaignId as string },
            query: {
              ...filters,
              q: filters.q || undefined,
              cursor: pageParam as string | undefined,
              limit: 200,
            },
          },
        }),
      ),
    getNextPageParam: (last) => last.next_cursor || undefined,
    placeholderData: keepPreviousData,
    enabled: !!campaignId,
    retry: shouldRetry,
  });
}

/** A renewal changes bookings, passes, the map and the planning too. */
function useInvalidate(establishmentId: string) {
  const qc = useQueryClient();
  return () => {
    for (const area of [
      'renewals',
      'bookings',
      'season-passes',
      'operations',
      'planning',
      'availability',
      'customers',
    ] as const) {
      void qc.invalidateQueries({ queryKey: establishmentKeys.area(establishmentId, area) });
    }
  };
}

export function useCreateRenewalCampaign(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidate(establishmentId);
  return useMutation<
    RenewalCampaignResult,
    ApiProblem,
    { body: RenewalCampaignCreate; idempotencyKey: string }
  >({
    mutationFn: ({ body, idempotencyKey }) =>
      callBooking(() =>
        client.POST(`${BASE}/renewal-campaigns`, {
          params: { path: { establishmentId }, header: { 'Idempotency-Key': idempotencyKey } },
          body,
        }),
      ),
    onSuccess: (r) => {
      if (!r.dry_run) invalidate();
    },
  });
}

export function useUpdateRenewalCampaign(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidate(establishmentId);
  return useMutation<
    RenewalCampaign,
    ApiProblem,
    { id: string; version: number; body: RenewalCampaignUpdate }
  >({
    mutationFn: ({ id, version, body }) =>
      callBooking(() =>
        client.PATCH(`${BASE}/renewal-campaigns/{campaignId}`, {
          params: {
            path: { establishmentId, campaignId: id },
            header: { 'If-Match': ifMatch(version) },
          },
          body,
        }),
      ),
    onSuccess: () => invalidate(),
  });
}

export function useBulkRenewal(establishmentId: string, campaignId: string) {
  const client = useApiClient();
  const invalidate = useInvalidate(establishmentId);
  return useMutation<RenewalBulkResult, ApiProblem, RenewalBulkRequest>({
    mutationFn: (body) =>
      callBooking(() =>
        client.POST(`${BASE}/renewal-campaigns/{campaignId}/proposals/bulk`, {
          params: {
            path: { establishmentId, campaignId },
            header: { 'Idempotency-Key': newIdempotencyKey() },
          },
          body,
        }),
      ),
    onSuccess: () => invalidate(),
  });
}

export function useAcceptProposal(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidate(establishmentId);
  return useMutation<
    RenewalProposal,
    ApiProblem,
    { proposal: RenewalProposal; body: RenewalStaffAcceptRequest; idempotencyKey: string }
  >({
    mutationFn: ({ proposal, body, idempotencyKey }) =>
      callBooking(() =>
        client.POST(`${BASE}/renewal-proposals/{proposalId}/accept`, {
          params: {
            path: { establishmentId, proposalId: proposal.id },
            header: { 'If-Match': ifMatch(proposal.version), 'Idempotency-Key': idempotencyKey },
          },
          body,
        }),
      ),
    onSuccess: () => invalidate(),
  });
}

export function useDeclineProposal(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidate(establishmentId);
  return useMutation<
    RenewalProposal,
    ApiProblem,
    { proposal: RenewalProposal; body: RenewalDeclineRequest }
  >({
    mutationFn: ({ proposal, body }) =>
      callBooking(() =>
        client.POST(`${BASE}/renewal-proposals/{proposalId}/decline`, {
          params: {
            path: { establishmentId, proposalId: proposal.id },
            header: { 'If-Match': ifMatch(proposal.version) },
          },
          body,
        }),
      ),
    onSuccess: () => invalidate(),
  });
}

export function useSendProposal(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidate(establishmentId);
  return useMutation<RenewalProposal, ApiProblem, RenewalProposal>({
    mutationFn: (proposal) =>
      callBooking(() =>
        client.POST(`${BASE}/renewal-proposals/{proposalId}/send`, {
          params: {
            path: { establishmentId, proposalId: proposal.id },
            header: { 'Idempotency-Key': newIdempotencyKey() },
          },
        }),
      ),
    onSuccess: () => invalidate(),
  });
}
