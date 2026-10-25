import { ApiProblem, call, shouldRetry, toApiProblem, useApiClient } from '@arenile/shared/api';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { configKey } from '../configuration/api/resource';
import { ifMatch, newIdempotencyKey } from '../configuration/lib/validation';
import type {
  ApiMapDocument,
  ApiMapInput,
  ApiMapSaveRequest,
  MapReorientRequest,
  MapReorientResult,
  RowGeneratorRequest,
  RowGeneratorResult,
} from './model/types';

const MAPS_PATH = '/api/admin/v1/establishments/{establishmentId}/maps' as const;
const MAP_PATH = '/api/admin/v1/establishments/{establishmentId}/maps/{mapId}' as const;

export const mapDocumentKey = (establishmentId: string, mapId: string) =>
  [...configKey(establishmentId), 'maps', 'document', mapId] as const;

/** Full map document (GET /maps/{id}); the version is the ETag. */
export function useMapDocument(establishmentId: string, mapId: string) {
  const client = useApiClient();
  return useQuery<ApiMapDocument, ApiProblem>({
    queryKey: mapDocumentKey(establishmentId, mapId),
    queryFn: () =>
      call(() => client.GET(MAP_PATH, { params: { path: { establishmentId, mapId } } })),
    retry: shouldRetry,
    // The editor keeps its own draft: never refetch behind the user's back.
    refetchOnWindowFocus: false,
    staleTime: Infinity,
  });
}

/**
 * Creates an empty map (POST /maps, Idempotency-Key). The answer is the full
 * document, not a bare Map: it seeds the editor's query so the editor opens
 * on it without a second request.
 */
export function useCreateMap(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<ApiMapDocument, ApiProblem, ApiMapInput>({
    mutationFn: (body) =>
      call(() =>
        client.POST(MAPS_PATH, {
          params: { path: { establishmentId }, header: { 'Idempotency-Key': newIdempotencyKey() } },
          body,
        }),
      ),
    onSuccess: (doc) => {
      qc.setQueryData(mapDocumentKey(establishmentId, doc.map.id), doc);
      void qc.invalidateQueries({
        queryKey: configKey(establishmentId),
        predicate: (q) => !(q.queryKey.includes('document') && q.queryKey.includes(doc.map.id)),
      });
    },
  });
}

/**
 * Problem from a map save, keeping the extension members the editor needs:
 * `current_version` (412 version_conflict) and `spot_ids`
 * (spot_has_future_allocations).
 */
export class MapSaveProblem extends ApiProblem {
  readonly currentVersion?: number;
  readonly spotIds: string[];

  constructor(base: ApiProblem, raw: unknown) {
    super({
      type: base.type,
      status: base.status,
      title: base.title,
      detail: base.detail,
      retry_after: base.retryAfter,
      errors: base.fieldErrors,
      request_id: base.requestId,
    });
    const body = (typeof raw === 'object' && raw !== null ? raw : {}) as {
      current_version?: unknown;
      spot_ids?: unknown;
    };
    this.currentVersion =
      typeof body.current_version === 'number' ? body.current_version : undefined;
    this.spotIds = Array.isArray(body.spot_ids)
      ? body.spot_ids.filter((x): x is string => typeof x === 'string')
      : [];
  }
}

export interface SaveMapVars {
  version: number;
  body: ApiMapSaveRequest;
}

/** Transactional save of the whole document with If-Match (412 on a stale version). */
export function useSaveMap(establishmentId: string, mapId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<ApiMapDocument, ApiProblem, SaveMapVars>({
    mutationFn: async ({ version, body }) => {
      let raw: unknown;
      try {
        return await call(async () => {
          const res = await client.PUT(MAP_PATH, {
            params: { path: { establishmentId, mapId }, header: { 'If-Match': ifMatch(version) } },
            body,
          });
          raw = res.error;
          return res;
        });
      } catch (err) {
        const problem = err instanceof ApiProblem ? err : toApiProblem(err, undefined);
        throw new MapSaveProblem(problem, raw);
      }
    },
    onSuccess: (doc) => {
      qc.setQueryData(mapDocumentKey(establishmentId, mapId), doc);
      // Zones (spot counts), pools (implicit capacity) and the map list change too.
      void qc.invalidateQueries({
        queryKey: configKey(establishmentId),
        predicate: (q) => !(q.queryKey.includes('document') && q.queryKey.includes(mapId)),
      });
    },
  });
}

/** Row generator preview (apply=false): nothing is written on the server. */
export function useRowPreview(establishmentId: string, mapId: string) {
  const client = useApiClient();
  return useMutation<RowGeneratorResult, ApiProblem, RowGeneratorRequest>({
    mutationFn: (body) =>
      call(() =>
        client.POST(`${MAP_PATH}/generate-rows`, {
          params: { path: { establishmentId, mapId } },
          body: { ...body, apply: false },
        }),
      ),
  });
}

export interface ReorientMapVars {
  version: number;
  body: MapReorientRequest;
}

/**
 * Moves the sea side of the saved map and flips or rotates the whole layout
 * (POST /maps/{id}/reorient, spec 11). With `dry_run` it is a preview that
 * writes nothing (future allocation counts); otherwise one versioned save
 * (If-Match, 412 on a stale version, 409 spot_has_future_allocations
 * without the acknowledgement).
 */
export function useReorientMap(establishmentId: string, mapId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<MapReorientResult, ApiProblem, ReorientMapVars>({
    mutationFn: async ({ version, body }) => {
      let raw: unknown;
      try {
        return await call(async () => {
          const res = await client.POST(`${MAP_PATH}/reorient`, {
            params: { path: { establishmentId, mapId }, header: { 'If-Match': ifMatch(version) } },
            body,
          });
          raw = res.error;
          return res;
        });
      } catch (err) {
        const problem = err instanceof ApiProblem ? err : toApiProblem(err, undefined);
        throw new MapSaveProblem(problem, raw);
      }
    },
    onSuccess: (res) => {
      if (!res.applied) return;
      qc.setQueryData(mapDocumentKey(establishmentId, mapId), res.document);
      void qc.invalidateQueries({
        queryKey: configKey(establishmentId),
        predicate: (q) => !(q.queryKey.includes('document') && q.queryKey.includes(mapId)),
      });
    },
  });
}
