import type { ApiProblem } from '@arenile/shared/api';
import { call, shouldRetry, useApiClient, type ApiClient, type paths } from '@arenile/shared/api';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ifMatch, newIdempotencyKey } from '../lib/validation';

/** Establishment-scoped admin paths of the generated contract. */
type EstPath = Extract<keyof paths, `/api/admin/v1/establishments/{establishmentId}/${string}`>;

type FetchResult = { data?: unknown; error?: unknown; response: Response };
type Init = {
  params: {
    path: Record<string, string>;
    query?: Record<string, string | number | undefined>;
    header?: Record<string, string>;
  };
  body?: unknown;
};

/**
 * openapi-fetch with the path type widened. The factory below only accepts
 * paths of the generated contract (`EstPath`) and the item/input types are
 * the generated schemas, so the contract is still checked at the call sites.
 */
interface LooseClient {
  GET(path: string, init: Init): Promise<FetchResult>;
  POST(path: string, init: Init): Promise<FetchResult>;
  PUT(path: string, init: Init): Promise<FetchResult>;
  DELETE(path: string, init: Init): Promise<FetchResult>;
}

function loose(client: ApiClient): LooseClient {
  return client as unknown as LooseClient;
}

/** Root of every configuration query of an establishment (invalidated on any config mutation). */
export const configKey = (establishmentId: string) => ['config', establishmentId] as const;

/** Upper bound of pages read by `fetchAll` (200 items each). */
const MAX_PAGES = 50;

/** Reads every page of a cursor-paginated collection (configuration lists are small). */
export async function fetchAll<T>(
  client: ApiClient,
  path: EstPath,
  establishmentId: string,
  query: Record<string, string | undefined> = {},
): Promise<T[]> {
  const items: T[] = [];
  let cursor: string | undefined;
  for (let i = 0; i < MAX_PAGES; i++) {
    const page = (await call(() =>
      loose(client).GET(path, {
        params: { path: { establishmentId }, query: { ...query, limit: 200, cursor } },
      }),
    )) as { items: T[]; next_cursor?: string };
    items.push(...page.items);
    if (!page.next_cursor) break;
    cursor = page.next_cursor;
  }
  return items;
}

export interface ResourceDef {
  /** Query key segment, e.g. "seasons". */
  key: string;
  collection: EstPath;
  item: EstPath;
  /** Name of the item path parameter, e.g. "seasonId". */
  idParam: string;
}

export interface Versioned {
  id: string;
  version: number;
}

export interface UpdateVars<TInput> {
  id: string;
  /** Version read before editing: sent as If-Match (412 version_conflict when stale). */
  version: number;
  input: TInput;
}

export interface RemoveVars {
  id: string;
  version: number;
}

/**
 * TanStack Query hooks for a configuration resource with the M1/M2 contract:
 * cursor list, POST with Idempotency-Key, full-replacement PUT and DELETE with
 * If-Match. Every mutation invalidates the establishment's configuration
 * queries (zones create pools, templates create everything, ...).
 */
export function createResource<
  TItem extends Versioned,
  TInput,
  TQuery extends Record<string, string | undefined> = Record<string, never>,
>(def: ResourceDef) {
  const keys = {
    all: (establishmentId: string) => [...configKey(establishmentId), def.key] as const,
    list: (establishmentId: string, query?: TQuery) =>
      [...configKey(establishmentId), def.key, 'list', query ?? {}] as const,
  };

  function useList(establishmentId: string, query?: TQuery, options: { enabled?: boolean } = {}) {
    const client = useApiClient();
    return useQuery<TItem[], ApiProblem>({
      queryKey: keys.list(establishmentId, query),
      queryFn: () => fetchAll<TItem>(client, def.collection, establishmentId, query ?? {}),
      enabled: options.enabled ?? true,
      retry: shouldRetry,
    });
  }

  function useInvalidate(establishmentId: string) {
    const qc = useQueryClient();
    return () => qc.invalidateQueries({ queryKey: configKey(establishmentId) });
  }

  function useCreate(establishmentId: string) {
    const client = useApiClient();
    const invalidate = useInvalidate(establishmentId);
    return useMutation<TItem, ApiProblem, TInput>({
      mutationFn: (input) =>
        call(() =>
          loose(client).POST(def.collection, {
            params: {
              path: { establishmentId },
              header: { 'Idempotency-Key': newIdempotencyKey() },
            },
            body: input,
          }),
        ) as Promise<TItem>,
      onSuccess: () => invalidate(),
    });
  }

  function useUpdate(establishmentId: string) {
    const client = useApiClient();
    const invalidate = useInvalidate(establishmentId);
    return useMutation<TItem, ApiProblem, UpdateVars<TInput>>({
      mutationFn: ({ id, version, input }) =>
        call(() =>
          loose(client).PUT(def.item, {
            params: {
              path: { establishmentId, [def.idParam]: id },
              header: { 'If-Match': ifMatch(version) },
            },
            body: input,
          }),
        ) as Promise<TItem>,
      onSuccess: () => invalidate(),
    });
  }

  function useRemove(establishmentId: string) {
    const client = useApiClient();
    const invalidate = useInvalidate(establishmentId);
    return useMutation<void, ApiProblem, RemoveVars>({
      mutationFn: async ({ id, version }) => {
        await call(() =>
          loose(client).DELETE(def.item, {
            params: {
              path: { establishmentId, [def.idParam]: id },
              header: { 'If-Match': ifMatch(version) },
            },
          }),
        );
      },
      onSuccess: () => invalidate(),
    });
  }

  return { keys, useList, useCreate, useUpdate, useRemove };
}
