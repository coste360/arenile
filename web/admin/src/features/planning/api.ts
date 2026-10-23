import {
  call,
  establishmentKeys,
  shouldRetry,
  useApiClient,
  type ApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { configKey } from '../configuration/api/resource';
import { ifMatch } from '../configuration/lib/validation';
import { localized } from '../map-editor/model/document';
import { invalidateOperational } from '../operations/api/operations';
import type { BookingPatch } from './lib/move';
import type { Planning } from './lib/grid';

/*
 * Data hooks of the planning (generated client). The grid lives under the
 * 'planning' area (SSE invalidation by prefix); zone names are configuration
 * with a long staleTime; a booking opened for a move lives under 'bookings'.
 */

type BookingDetail = Schemas['BookingDetail'];
type BookingChangeResult = Schemas['BookingChangeResult'];

const EST = '/api/admin/v1/establishments/{establishmentId}' as const;

export const planningKeys = {
  grid: (establishmentId: string, from: string, to: string) =>
    [...establishmentKeys.area(establishmentId, 'planning'), 'grid', { from, to }] as const,
  zones: (establishmentId: string) => [...configKey(establishmentId), 'zones', 'planning'] as const,
  booking: (establishmentId: string, bookingId: string) =>
    [...establishmentKeys.area(establishmentId, 'bookings'), 'detail', bookingId] as const,
};

export function usePlanning(establishmentId: string, from: string, to: string) {
  const client = useApiClient();
  return useQuery<Planning, ApiProblem>({
    queryKey: planningKeys.grid(establishmentId, from, to),
    queryFn: () =>
      call(() =>
        client.GET(`${EST}/planning`, {
          params: { path: { establishmentId }, query: { from, to } },
        }),
      ),
    retry: shouldRetry,
    placeholderData: keepPreviousData,
    staleTime: 15_000,
  });
}

/** Zone names (localised) and display order, for the row groups. */
export function usePlanningZones(establishmentId: string) {
  const client = useApiClient();
  const { i18n } = useTranslation();
  const locale = i18n.language;
  return useQuery<Schemas['Zone'][], ApiProblem, { names: Map<string, string>; order: string[] }>({
    queryKey: planningKeys.zones(establishmentId),
    queryFn: async () =>
      (
        await call(() =>
          client.GET(`${EST}/zones`, {
            params: { path: { establishmentId }, query: { limit: 200 } },
          }),
        )
      ).items,
    select: (zones) => {
      const sorted = [...zones].sort((a, b) => a.sort_order - b.sort_order);
      return {
        names: new Map(sorted.map((z) => [z.id, localized(z.name, locale) || z.code])),
        order: sorted.map((z) => z.id),
      };
    },
    retry: shouldRetry,
    staleTime: 5 * 60_000,
  });
}

/** Booking detail (with its version, used as If-Match) for the move dialog. */
export function useBookingDetail(establishmentId: string, bookingId: string | undefined) {
  const client = useApiClient();
  return useQuery<BookingDetail, ApiProblem>({
    queryKey: planningKeys.booking(establishmentId, bookingId ?? ''),
    queryFn: () =>
      call(() =>
        client.GET(`${EST}/bookings/{bookingId}`, {
          params: { path: { establishmentId, bookingId: bookingId as string } },
        }),
      ),
    enabled: !!bookingId,
    retry: shouldRetry,
    staleTime: 0,
  });
}

export interface PatchBookingVars {
  bookingId: string;
  version: number;
  patch: BookingPatch;
  dryRun: boolean;
}

/** PATCH /bookings/{id} with If-Match; a stale version rejects with an ApiProblem of status 412. */
export async function patchBooking(
  client: ApiClient,
  establishmentId: string,
  { bookingId, version, patch, dryRun }: PatchBookingVars,
): Promise<BookingChangeResult> {
  return call(() =>
    client.PATCH(`${EST}/bookings/{bookingId}`, {
      params: {
        path: { establishmentId, bookingId },
        query: dryRun ? { dry_run: true } : {},
        header: { 'If-Match': ifMatch(version) },
      },
      body: patch,
    }),
  );
}

/** Dry run of a PATCH: price difference and conflicts, nothing written. */
export function usePatchPreview(establishmentId: string, vars: PatchBookingVars | null) {
  const client = useApiClient();
  return useQuery<BookingChangeResult, ApiProblem>({
    // Outside the SSE-invalidated areas: applying the PATCH must not re-run the preview.
    queryKey: ['patch-preview', establishmentId, vars ?? null],
    queryFn: () =>
      patchBooking(client, establishmentId, { ...(vars as PatchBookingVars), dryRun: true }),
    enabled: !!vars,
    retry: false,
    gcTime: 0,
  });
}

/**
 * Applies the PATCH (optimistic concurrency, spec 10.3): on success the
 * operational areas refresh (grid, map, lists, booking detail).
 */
export function usePatchBooking(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<BookingChangeResult, ApiProblem, PatchBookingVars>({
    mutationFn: (vars) => patchBooking(client, establishmentId, vars),
    onSuccess: (res) => {
      // Grid, map, lists and this booking's detail (all under the invalidated areas).
      if (res.applied) invalidateOperational(qc, establishmentId);
    },
  });
}
