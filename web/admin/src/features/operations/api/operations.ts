import {
  call,
  establishmentKeys,
  shouldRetry,
  useApiClient,
  type ApiClient,
  type ApiProblem,
  type EstablishmentArea,
  type Schemas,
} from '@arenile/shared/api';
import type { MapGeometry } from '@arenile/shared/map';
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { configKey } from '../../configuration/api/resource';
import { localized } from '../../map-editor/model/document';
import { ifMatch, newIdempotencyKey } from '../../configuration/lib/validation';
import { netTakings } from '../lib/kpis';
import type { OperationsDay, SlotChoice } from '../lib/status';

/*
 * Data hooks of the operational pages, on the generated client.
 *
 * Query keys: operational data lives under establishmentKeys.area(id, area)
 * so that the SSE stream (useEstablishmentEvents in EstablishmentLayout)
 * invalidates it by prefix; TanStack refetches only the *active* queries, so
 * a burst of events costs one refetch of the visible day, not one per date
 * ever opened. Map geometry is configuration (config key, long staleTime):
 * booking events never refetch it, saving the map in the editor does.
 */

type MapDocument = Schemas['MapDocument'];
type MapSummary = Schemas['Map'];
type BookingDetail = Schemas['BookingDetail'];
type BookingChangeResult = Schemas['BookingChangeResult'];
type SeasonReleaseList = Schemas['SeasonReleaseList'];

const EST = '/api/admin/v1/establishments/{establishmentId}' as const;

export const operationsKeys = {
  day: (establishmentId: string, date: string, slot: SlotChoice, mapId: string | undefined) =>
    [
      ...establishmentKeys.area(establishmentId, 'operations'),
      'day',
      { date, slot, mapId: mapId ?? null },
    ] as const,
  takings: (establishmentId: string, date: string) =>
    [...establishmentKeys.area(establishmentId, 'payments'), 'takings', date] as const,
  maps: (establishmentId: string) =>
    [...configKey(establishmentId), 'maps', 'operational'] as const,
  // Same key as the map editor's document query: a save there updates this cache too.
  mapDocument: (establishmentId: string, mapId: string) =>
    [...configKey(establishmentId), 'maps', 'document', mapId] as const,
};

/** Converts the admin map document into the read-only renderer geometry. */
export function toGeometry(doc: MapDocument, locale: string): MapGeometry {
  return {
    id: doc.map.id,
    name: localized(doc.map.name, locale),
    width: doc.map.width,
    height: doc.map.height,
    grid_size: doc.map.grid_size,
    sea_side: doc.map.sea_side,
    spots: doc.spots.map((s) => ({
      id: s.id,
      label: s.label,
      row_label: s.row_label || null,
      number: s.number,
      kind: s.kind,
      x: s.x,
      y: s.y,
      rotation: s.rotation,
      shape: s.shape,
      zone_id: s.zone_id,
      active: s.active,
    })),
    decorations: doc.decorations.map((d) => ({
      id: d.id,
      kind: d.kind,
      x: d.x,
      y: d.y,
      rotation: d.rotation,
      shape: d.shape,
      label: d.label ? localized(d.label, locale) || null : null,
    })),
  };
}

/** Maps of the establishment, in display order. */
export function useOperationalMaps(establishmentId: string) {
  const client = useApiClient();
  return useQuery<MapSummary[], ApiProblem>({
    queryKey: operationsKeys.maps(establishmentId),
    queryFn: async () => {
      const res = await call(() =>
        client.GET(`${EST}/maps`, {
          params: { path: { establishmentId }, query: { limit: 200 } },
        }),
      );
      return [...res.items].sort((a, b) => a.sort_order - b.sort_order);
    },
    retry: shouldRetry,
    staleTime: 5 * 60_000,
  });
}

/** Geometry of one map for the operational renderer. */
export function useMapGeometry(establishmentId: string, mapId: string | undefined) {
  const client = useApiClient();
  const { i18n } = useTranslation();
  const locale = i18n.language;
  return useQuery<MapDocument, ApiProblem, MapGeometry>({
    queryKey: operationsKeys.mapDocument(establishmentId, mapId ?? ''),
    queryFn: () =>
      call(() =>
        client.GET(`${EST}/maps/{mapId}`, {
          params: { path: { establishmentId, mapId: mapId as string } },
        }),
      ),
    select: (doc) => toGeometry(doc, locale),
    enabled: !!mapId,
    retry: shouldRetry,
    staleTime: 5 * 60_000,
  });
}

/** Operational status of every spot for a date and slot (GET /operations/day). */
export function useOperationsDay(
  establishmentId: string,
  date: string,
  slot: SlotChoice,
  mapId?: string,
) {
  const client = useApiClient();
  return useQuery<OperationsDay, ApiProblem>({
    queryKey: operationsKeys.day(establishmentId, date, slot, mapId),
    queryFn: () =>
      call(() =>
        client.GET(`${EST}/operations/day`, {
          params: {
            path: { establishmentId },
            query: { date, slot, ...(mapId ? { map_id: mapId } : {}) },
          },
        }),
      ),
    retry: shouldRetry,
    // Keep the previous day on screen while the next one loads (no flashing map).
    placeholderData: keepPreviousData,
    staleTime: 15_000,
  });
}

const MAX_PAYMENT_PAGES = 20;

/** Net takings of a day in cents (payments in minus refunds, succeeded only). */
export function useDayTakings(establishmentId: string, date: string, enabled: boolean) {
  const client = useApiClient();
  return useQuery<number, ApiProblem>({
    queryKey: operationsKeys.takings(establishmentId, date),
    queryFn: async () => {
      const items: Schemas['Payment'][] = [];
      let cursor: string | undefined;
      for (let page = 0; page < MAX_PAYMENT_PAGES; page++) {
        const res = await call(() =>
          client.GET(`${EST}/payments`, {
            params: {
              path: { establishmentId },
              query: { from: date, to: date, limit: 200, ...(cursor ? { cursor } : {}) },
            },
          }),
        );
        items.push(...res.items);
        cursor = res.next_cursor;
        if (!cursor) break;
      }
      return netTakings(items);
    },
    enabled,
    retry: shouldRetry,
    staleTime: 15_000,
  });
}

/** Areas touched by a booking change made from the operational pages. */
const CHANGED_AREAS: readonly EstablishmentArea[] = [
  'operations',
  'planning',
  'availability',
  'bookings',
  'season-passes',
];

export function invalidateOperational(qc: QueryClient, establishmentId: string): void {
  for (const area of CHANGED_AREAS) {
    void qc.invalidateQueries({ queryKey: establishmentKeys.area(establishmentId, area) });
  }
}

/**
 * Check-in of a confirmed booking (operations.run). `version` is sent as
 * If-Match: a booking changed since the map was read answers 412 instead
 * of being checked in blindly.
 */
export function useCheckIn(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<BookingDetail, ApiProblem, { bookingId: string; version?: number }>({
    mutationFn: ({ bookingId, version }) =>
      call(() =>
        client.POST(`${EST}/bookings/{bookingId}/check-in`, {
          params: {
            path: { establishmentId, bookingId },
            ...(version !== undefined ? { header: { 'If-Match': ifMatch(version) } } : {}),
          },
          body: {},
        }),
      ),
    onSuccess: () => invalidateOperational(qc, establishmentId),
  });
}

export interface ReleaseDayVars {
  passId: string;
  date: string;
  slot: SlotChoice;
}

/** Releases one day of a season pass: the spot becomes bookable (spec 7.7). */
export function useReleaseDay(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<SeasonReleaseList, ApiProblem, ReleaseDayVars>({
    mutationFn: ({ passId, date, slot }) =>
      call(() =>
        client.POST(`${EST}/season-passes/{passId}/releases`, {
          params: {
            path: { establishmentId, passId },
            header: { 'Idempotency-Key': newIdempotencyKey() },
          },
          body: { dates: [date], slots: slot },
        }),
      ),
    onSuccess: () => invalidateOperational(qc, establishmentId),
  });
}

/** Revokes a release that was not resold yet. */
export function useRevokeRelease(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<void, ApiProblem, { passId: string; releaseId: string }>({
    mutationFn: async ({ passId, releaseId }) => {
      await call(() =>
        client.DELETE(`${EST}/season-passes/{passId}/releases/{releaseId}`, {
          params: { path: { establishmentId, passId, releaseId } },
        }),
      );
    },
    onSuccess: () => invalidateOperational(qc, establishmentId),
  });
}

export interface MoveLineVars {
  bookingId: string;
  lineId: string;
  spotId: string;
  /** First day moved (default: the whole line). */
  from?: string;
  dryRun?: boolean;
  /** Booking version read with the preview, sent as If-Match on the apply. */
  version?: number;
}

/**
 * Moves a booking line to another spot (POST /move); dryRun previews
 * conflicts and price. With `version` the apply carries If-Match, so a
 * booking changed between the preview and the apply answers 412 instead of
 * moving with a price difference other than the one shown.
 */
export function moveLine(client: ApiClient, establishmentId: string, vars: MoveLineVars) {
  return call(() =>
    client.POST(`${EST}/bookings/{bookingId}/move`, {
      params: {
        path: { establishmentId, bookingId: vars.bookingId },
        ...(vars.version !== undefined && !vars.dryRun
          ? { header: { 'If-Match': ifMatch(vars.version) } }
          : {}),
      },
      body: {
        line_id: vars.lineId,
        spot_id: vars.spotId,
        ...(vars.from ? { from: vars.from } : {}),
        dry_run: vars.dryRun ?? false,
      },
    }),
  );
}

export function useMoveLine(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<BookingChangeResult, ApiProblem, MoveLineVars>({
    mutationFn: (vars) => moveLine(client, establishmentId, vars),
    onSuccess: (res) => {
      if (res.applied) invalidateOperational(qc, establishmentId);
    },
  });
}
