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
import type { SlotChoice } from '../lib/status';
import { invalidateOperational } from './operations';

/*
 * Data hooks of the M8 cash-desk additions to the operational pages (spec
 * 30.4-30.10, ADR 0028): spot season calendar, lifeguard list, range view,
 * swap, group move, undo of the last move.
 * Operational data lives under the SSE-invalidated areas, like operations.ts.
 */

type SpotCalendar = Schemas['SpotCalendar'];
type LifeguardList = Schemas['LifeguardList'];
type AvailabilityResult = Schemas['AvailabilityResult'];
type BookingSwapRequest = Schemas['BookingSwapRequest'];
type BookingSwapResult = Schemas['BookingSwapResult'];
type GroupMoveRequest = Schemas['GroupMoveRequest'];
type GroupMoveResult = Schemas['GroupMoveResult'];
type LastMove = Schemas['LastMove'];
type MoveUndoResult = Schemas['MoveUndoResult'];
export type MoveUndoInfo = Schemas['MoveUndoInfo'];

const EST = '/api/admin/v1/establishments/{establishmentId}' as const;

export const m8Keys = {
  spotCalendar: (establishmentId: string, spotId: string, seasonId: string | undefined) =>
    [
      ...establishmentKeys.area(establishmentId, 'operations'),
      'spot-calendar',
      spotId,
      seasonId ?? null,
    ] as const,
  lifeguard: (establishmentId: string, date: string, slot: SlotChoice, mapId?: string) =>
    [
      ...establishmentKeys.area(establishmentId, 'operations'),
      'lifeguard',
      { date, slot, mapId: mapId ?? null },
    ] as const,
  range: (establishmentId: string, from: string, to: string, slot: SlotChoice, mapId?: string) =>
    [
      ...establishmentKeys.area(establishmentId, 'availability'),
      'operations-range',
      { from, to, slot, mapId: mapId ?? null },
    ] as const,
  lastMove: (establishmentId: string) =>
    [...establishmentKeys.area(establishmentId, 'bookings'), 'last-move'] as const,
};

/** Season calendar of a spot (spec 30.5); `enabled` false until the panel is open. */
export function useSpotCalendar(
  establishmentId: string,
  spotId: string | undefined,
  enabled: boolean,
  seasonId?: string,
) {
  const client = useApiClient();
  return useQuery<SpotCalendar, ApiProblem>({
    queryKey: m8Keys.spotCalendar(establishmentId, spotId ?? '', seasonId),
    queryFn: () =>
      call(() =>
        client.GET(`${EST}/spots/{spotId}/calendar`, {
          params: {
            path: { establishmentId, spotId: spotId as string },
            query: seasonId ? { season_id: seasonId } : {},
          },
        }),
      ),
    enabled: enabled && !!spotId,
    retry: shouldRetry,
    staleTime: 15_000,
  });
}

/** Lifeguard list of a day (spec 30.10): rows, spots, surnames, setup, arrivals and departures. */
export function useLifeguardList(
  establishmentId: string,
  date: string,
  slot: SlotChoice,
  mapId: string | undefined,
  enabled: boolean,
) {
  const client = useApiClient();
  return useQuery<LifeguardList, ApiProblem>({
    queryKey: m8Keys.lifeguard(establishmentId, date, slot, mapId),
    queryFn: () =>
      call(() =>
        client.GET(`${EST}/operations/lifeguard-list`, {
          params: {
            path: { establishmentId },
            query: { date, slot, ...(mapId ? { map_id: mapId } : {}) },
          },
        }),
      ) as Promise<LifeguardList>,
    enabled,
    retry: shouldRetry,
    staleTime: 15_000,
  });
}

/** Desk availability of every spot over a range (spec 30.4 range view: free_days of total_days). */
export function useRangeAvailability(
  establishmentId: string,
  range: { from: string; to: string } | null,
  slot: SlotChoice,
  mapId?: string,
) {
  const client = useApiClient();
  return useQuery<AvailabilityResult, ApiProblem>({
    queryKey: m8Keys.range(establishmentId, range?.from ?? '', range?.to ?? '', slot, mapId),
    queryFn: () =>
      call(() =>
        client.GET(`${EST}/availability`, {
          params: {
            path: { establishmentId },
            query: {
              from: range!.from,
              to: range!.to,
              slots: slot,
              channel: 'desk',
              ...(mapId ? { map_id: mapId } : {}),
            },
          },
        }),
      ),
    enabled: !!range,
    placeholderData: keepPreviousData,
    retry: shouldRetry,
    staleTime: 15_000,
  });
}

/** Swap of two booking lines from a day (spec 30.7); dry_run previews both price differences. */
export function swapBookings(client: ApiClient, establishmentId: string, body: BookingSwapRequest) {
  return call(() =>
    client.POST(`${EST}/bookings/swap`, { params: { path: { establishmentId } }, body }),
  );
}

export function useSwapBookings(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<BookingSwapResult, ApiProblem, BookingSwapRequest>({
    mutationFn: (body) => swapBookings(client, establishmentId, body),
    onSuccess: (res) => {
      if (res.applied) invalidateOperational(qc, establishmentId);
    },
  });
}

/** Several lines moved together keeping their arrangement (spec 30.7); all or nothing. */
export function moveGroup(client: ApiClient, establishmentId: string, body: GroupMoveRequest) {
  return call(() =>
    client.POST(`${EST}/bookings/group-move`, { params: { path: { establishmentId } }, body }),
  );
}

export function useMoveGroup(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<GroupMoveResult, ApiProblem, GroupMoveRequest>({
    mutationFn: (body) => moveGroup(client, establishmentId, body),
    onSuccess: (res) => {
      if (res.applied) invalidateOperational(qc, establishmentId);
    },
  });
}

/** The current user's last move still undoable (5 minutes). */
export function useLastMove(establishmentId: string, enabled: boolean) {
  const client = useApiClient();
  return useQuery<LastMove, ApiProblem>({
    queryKey: m8Keys.lastMove(establishmentId),
    queryFn: () =>
      call(() => client.GET(`${EST}/moves/last`, { params: { path: { establishmentId } } })),
    enabled,
    retry: false,
    staleTime: 10_000,
  });
}

/** Undo of a move, a swap or a group move (409 if the old spots are taken, 410 after 5 minutes). */
export function useUndoMove(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<MoveUndoResult, ApiProblem, { moveId: string }>({
    mutationFn: ({ moveId }) =>
      call(() =>
        client.POST(`${EST}/moves/{moveId}/undo`, {
          params: { path: { establishmentId, moveId } },
        }),
      ),
    onSettled: () => {
      invalidateOperational(qc, establishmentId);
      void qc.invalidateQueries({ queryKey: m8Keys.lastMove(establishmentId) });
    },
  });
}
