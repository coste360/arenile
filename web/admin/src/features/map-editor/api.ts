import { ApiProblem, call, shouldRetry, toApiProblem, useApiClient } from '@arenile/shared/api';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { configKey } from '../configuration/api/resource';
import { ifMatch } from '../configuration/lib/validation';
import type {
  ApiMapDocument,
  ApiMapSaveRequest,
  RowGeneratorRequest,
  RowGeneratorResult,
} from './model/types';

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
