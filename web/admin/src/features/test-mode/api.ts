import {
  call,
  establishmentKeys,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ifMatch } from '../configuration/lib/validation';
import { onlineKeys } from '../online/api';

/**
 * Data hooks of the test mode (spec 15.4, ADR 0019): the switch (If-Match
 * with the version read), the preview link of the public page and the bulk
 * purge of the test bookings. The state lives under the `bookings` area so
 * that the real-time stream refreshes the test bookings count.
 */

export type TestMode = Schemas['TestMode'];
export type TestModeInput = Schemas['TestModeInput'];
export type TestModePreviewLink = Schemas['TestModePreviewLink'];
export type TestBookingsPurgeResult = Schemas['TestBookingsPurgeResult'];

export const testModeKeys = {
  state: (establishmentId: string) =>
    [...establishmentKeys.area(establishmentId, 'bookings'), 'test-mode'] as const,
};

const BASE = '/api/admin/v1/establishments/{establishmentId}' as const;
const path = (establishmentId: string) => ({ path: { establishmentId } });

/** Test-mode state; `enabled: false` turns the query off (no establishment). */
export function useTestMode(establishmentId: string | undefined, options?: { poll?: boolean }) {
  const client = useApiClient();
  return useQuery<TestMode, ApiProblem>({
    queryKey: testModeKeys.state(establishmentId ?? ''),
    queryFn: () =>
      call(() => client.GET(`${BASE}/test-mode`, { params: path(establishmentId ?? '') })),
    enabled: !!establishmentId,
    retry: shouldRetry,
    staleTime: 30_000,
    // The shell banner also follows switches made from another device.
    refetchInterval: options?.poll ? 60_000 : false,
  });
}

export function useUpdateTestMode(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<TestMode, ApiProblem, { version: number; body: TestModeInput }>({
    mutationFn: ({ version, body }) =>
      call(() =>
        client.PUT(`${BASE}/test-mode`, {
          params: { ...path(establishmentId), header: { 'If-Match': ifMatch(version) } },
          body,
        }),
      ),
    onSuccess: (s) => {
      qc.setQueryData(testModeKeys.state(establishmentId), s);
      // The payment settings report whether test keys block real customers
      // (live_payments_blocked, INV-88), which depends on the switch.
      void qc.invalidateQueries({ queryKey: onlineKeys.payments(establishmentId) });
    },
  });
}

export function useCreatePreviewLink(establishmentId: string) {
  const client = useApiClient();
  return useMutation<TestModePreviewLink, ApiProblem, void>({
    mutationFn: () =>
      call(() => client.POST(`${BASE}/test-mode/preview-link`, { params: path(establishmentId) })),
  });
}

export function usePurgeTestBookings(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<TestBookingsPurgeResult, ApiProblem, { dryRun: boolean }>({
    mutationFn: ({ dryRun }) =>
      call(() =>
        client.POST(`${BASE}/test-bookings/purge`, {
          params: path(establishmentId),
          body: { dry_run: dryRun },
        }),
      ),
    onSuccess: (res) => {
      if (!res.deleted) return;
      // Bookings, allocations, payments and KPIs of the whole establishment changed.
      void qc.invalidateQueries({ queryKey: establishmentKeys.all(establishmentId) });
    },
  });
}
