import { call, type ApiClient, type Schemas } from '@arenile/shared/api';
import type { QueryClient } from '@tanstack/react-query';
import { operationsKeys } from '../features/operations/api/operations';
import { offlineWindow } from './cache';

/*
 * Warm-up of the data the panel needs offline (spec 21.2: map and bookings
 * of today and tomorrow), run while online. It fills the same query keys as
 * the operational pages (operationsKeys), so the pages and the persister
 * share the entries; loaded lazily (own chunk) by OfflineShell.
 */

const EST = '/api/admin/v1/establishments/{establishmentId}' as const;
const MAPS_STALE_MS = 5 * 60_000;
const DAY_STALE_MS = 60_000;

/** Fetches maps, map documents and the operations day of today and tomorrow. */
export async function prefetchOfflineData(
  qc: QueryClient,
  client: ApiClient,
  establishmentId: string,
  timeZone: string | undefined,
  now: Date = new Date(),
): Promise<void> {
  const maps = await qc.fetchQuery<Schemas['Map'][]>({
    queryKey: operationsKeys.maps(establishmentId),
    queryFn: async () => {
      const res = await call(() =>
        client.GET(`${EST}/maps`, {
          params: { path: { establishmentId }, query: { limit: 200 } },
        }),
      );
      return [...res.items].sort((a, b) => a.sort_order - b.sort_order);
    },
    staleTime: MAPS_STALE_MS,
  });
  const days = offlineWindow(timeZone, now);
  await Promise.all([
    ...maps.map((m) =>
      qc.prefetchQuery({
        queryKey: operationsKeys.mapDocument(establishmentId, m.id),
        queryFn: () =>
          call(() =>
            client.GET(`${EST}/maps/{mapId}`, {
              params: { path: { establishmentId, mapId: m.id } },
            }),
          ),
        staleTime: MAPS_STALE_MS,
      }),
    ),
    ...days.map((date) =>
      qc.prefetchQuery({
        // Same key as the Oggi page and the single-map operational view.
        queryKey: operationsKeys.day(establishmentId, date, 'FULL', undefined),
        queryFn: () =>
          call(() =>
            client.GET(`${EST}/operations/day`, {
              params: { path: { establishmentId }, query: { date, slot: 'FULL' } },
            }),
          ),
        staleTime: DAY_STALE_MS,
      }),
    ),
  ]);
}
