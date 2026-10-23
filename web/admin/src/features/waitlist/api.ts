/**
 * Server state of the waitlist feature (M5B: waitlist entries, offers and
 * settings, spec 10.5): types from the generated schema, query keys under the
 * `waitlist` area (refreshed by the booking, allocation and hold SSE topics)
 * and the TanStack Query hooks.
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
import { ifMatch } from '../configuration/lib/validation';

export type WaitlistEntry = Schemas['WaitlistEntry'];
export type WaitlistEntryList = Schemas['WaitlistEntryList'];
export type WaitlistEntryDetail = Schemas['WaitlistEntryDetail'];
export type WaitlistEntryCreate = Schemas['WaitlistEntryCreate'];
export type WaitlistEntryUpdate = Schemas['WaitlistEntryUpdate'];
export type WaitlistEntryStatus = Schemas['WaitlistEntryStatus'];
export type WaitlistOffer = Schemas['WaitlistOffer'];
export type WaitlistOfferStatus = Schemas['WaitlistOfferStatus'];
export type WaitlistOfferCreate = Schemas['WaitlistOfferCreate'];
export type WaitlistReorderRequest = Schemas['WaitlistReorderRequest'];
export type WaitlistSettings = Schemas['WaitlistSettings'];
export type WaitlistSettingsInput = Schemas['WaitlistSettingsInput'];

const BASE = '/api/admin/v1/establishments/{establishmentId}' as const;

export interface WaitlistFilters {
  status?: WaitlistEntryStatus;
  zone_id?: string;
  pack_id?: string;
  q?: string;
}

export const waitlistKeys = {
  all: (est: string) => establishmentKeys.area(est, 'waitlist'),
  list: (est: string, f: WaitlistFilters) => [...waitlistKeys.all(est), 'list', f] as const,
  detail: (est: string, id: string) => [...waitlistKeys.all(est), 'detail', id] as const,
  settings: (est: string) => [...waitlistKeys.all(est), 'settings'] as const,
};

/** Entries in queue order (priority, then join date), cursor pages of 200. */
export function useWaitlist(establishmentId: string, filters: WaitlistFilters) {
  const client = useApiClient();
  return useInfiniteQuery<WaitlistEntryList, ApiProblem>({
    queryKey: waitlistKeys.list(establishmentId, filters),
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      callBooking(() =>
        client.GET(`${BASE}/waitlist`, {
          params: {
            path: { establishmentId },
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
    retry: shouldRetry,
  });
}

export function useWaitlistEntry(establishmentId: string, entryId: string | undefined) {
  const client = useApiClient();
  return useQuery<WaitlistEntryDetail, ApiProblem>({
    queryKey: waitlistKeys.detail(establishmentId, entryId ?? ''),
    queryFn: () =>
      callBooking(() =>
        client.GET(`${BASE}/waitlist/{entryId}`, {
          params: { path: { establishmentId, entryId: entryId as string } },
        }),
      ),
    enabled: !!entryId,
    retry: shouldRetry,
  });
}

export function useWaitlistSettings(establishmentId: string) {
  const client = useApiClient();
  return useQuery<WaitlistSettings, ApiProblem>({
    queryKey: waitlistKeys.settings(establishmentId),
    queryFn: () =>
      callBooking(() =>
        client.GET(`${BASE}/waitlist-settings`, { params: { path: { establishmentId } } }),
      ),
    retry: shouldRetry,
  });
}

/** An offer holds inventory: the map, the planning and the availability change too. */
function useInvalidate(establishmentId: string) {
  const qc = useQueryClient();
  return () => {
    for (const area of ['waitlist', 'holds', 'operations', 'planning', 'availability'] as const) {
      void qc.invalidateQueries({ queryKey: establishmentKeys.area(establishmentId, area) });
    }
  };
}

export function useCreateWaitlistEntry(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidate(establishmentId);
  return useMutation<
    WaitlistEntry,
    ApiProblem,
    { body: WaitlistEntryCreate; idempotencyKey: string }
  >({
    mutationFn: ({ body, idempotencyKey }) =>
      callBooking(() =>
        client.POST(`${BASE}/waitlist`, {
          params: { path: { establishmentId }, header: { 'Idempotency-Key': idempotencyKey } },
          body,
        }),
      ),
    onSuccess: () => invalidate(),
  });
}

export function useUpdateWaitlistEntry(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidate(establishmentId);
  return useMutation<
    WaitlistEntry,
    ApiProblem,
    { entry: WaitlistEntry; body: WaitlistEntryUpdate }
  >({
    mutationFn: ({ entry, body }) =>
      callBooking(() =>
        client.PATCH(`${BASE}/waitlist/{entryId}`, {
          params: {
            path: { establishmentId, entryId: entry.id },
            header: { 'If-Match': ifMatch(entry.version) },
          },
          body,
        }),
      ),
    onSuccess: () => invalidate(),
  });
}

export function useRemoveWaitlistEntry(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidate(establishmentId);
  return useMutation<void, ApiProblem, WaitlistEntry>({
    mutationFn: async (entry) => {
      await callBooking(() =>
        client.DELETE(`${BASE}/waitlist/{entryId}`, {
          params: {
            path: { establishmentId, entryId: entry.id },
            header: { 'If-Match': ifMatch(entry.version) },
          },
        }),
      );
    },
    onSuccess: () => invalidate(),
  });
}

/** New queue order: the listed entries get priority 10, 20, 30… */
export function useReorderWaitlist(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidate(establishmentId);
  return useMutation<void, ApiProblem, string[]>({
    mutationFn: async (entryIds) => {
      await callBooking(() =>
        client.POST(`${BASE}/waitlist/reorder`, {
          params: { path: { establishmentId } },
          body: { entry_ids: entryIds },
        }),
      );
    },
    onSuccess: () => invalidate(),
  });
}

/** Manual offer: holds a spot for the entry and emails the link (spec 10.5). */
export function useOfferWaitlistEntry(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidate(establishmentId);
  return useMutation<
    WaitlistOffer,
    ApiProblem,
    { entryId: string; body: WaitlistOfferCreate; idempotencyKey: string }
  >({
    mutationFn: ({ entryId, body, idempotencyKey }) =>
      callBooking(() =>
        client.POST(`${BASE}/waitlist/{entryId}/offer`, {
          params: {
            path: { establishmentId, entryId },
            header: { 'Idempotency-Key': idempotencyKey },
          },
          body,
        }),
      ),
    onSuccess: () => invalidate(),
  });
}

export function useRevokeWaitlistOffer(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidate(establishmentId);
  return useMutation<WaitlistOffer, ApiProblem, { entryId: string; offerId: string }>({
    mutationFn: ({ entryId, offerId }) =>
      callBooking(() =>
        client.POST(`${BASE}/waitlist/{entryId}/offers/{offerId}/revoke`, {
          params: { path: { establishmentId, entryId, offerId } },
        }),
      ),
    onSuccess: () => invalidate(),
  });
}

export function useUpdateWaitlistSettings(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<
    WaitlistSettings,
    ApiProblem,
    { version: number; body: WaitlistSettingsInput }
  >({
    mutationFn: ({ version, body }) =>
      callBooking(() =>
        client.PUT(`${BASE}/waitlist-settings`, {
          params: { path: { establishmentId }, header: { 'If-Match': ifMatch(version) } },
          body,
        }),
      ),
    onSuccess: (s) => qc.setQueryData(waitlistKeys.settings(establishmentId), s),
  });
}
