/**
 * Server state of the season-rollover feature (M5B: end-of-season wizard,
 * preview and apply the copy into the next season, spec 8.7, 28): types from
 * the generated schema and the TanStack Query hooks. Rollovers live with the
 * configuration queries (they create a season), so an apply invalidates them.
 */
import {
  call,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { configKey } from '../configuration/api/resource';

export type SeasonRolloverRequest = Schemas['SeasonRolloverRequest'];
export type SeasonRolloverResult = Schemas['SeasonRolloverResult'];
export type SeasonRolloverCounts = Schemas['SeasonRolloverCounts'];
export type SeasonRollover = Schemas['SeasonRollover'];
export type SeasonRolloverList = Schemas['SeasonRolloverList'];

const BASE = '/api/admin/v1/establishments/{establishmentId}' as const;

export const rolloverKeys = {
  list: (est: string) => [...configKey(est), 'season-rollovers'] as const,
};

/** Rollovers already applied (newest first, one page: one per year). */
export function useSeasonRollovers(establishmentId: string) {
  const client = useApiClient();
  return useQuery<SeasonRolloverList, ApiProblem>({
    queryKey: rolloverKeys.list(establishmentId),
    queryFn: () =>
      call(() =>
        client.GET(`${BASE}/season-rollovers`, {
          params: { path: { establishmentId }, query: { limit: 200 } },
        }),
      ),
    retry: shouldRetry,
  });
}

/**
 * Preview (`dry_run: true`) or apply the rollover of `seasonId`. Apply is
 * idempotent server side (same target year → `already_applied`), and the
 * Idempotency-Key of one wizard session protects against double clicks.
 */
export function useRolloverSeason(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<
    SeasonRolloverResult,
    ApiProblem,
    { seasonId: string; body: SeasonRolloverRequest; idempotencyKey: string }
  >({
    mutationFn: ({ seasonId, body, idempotencyKey }) =>
      call(() =>
        client.POST(`${BASE}/seasons/{seasonId}/rollover`, {
          params: {
            path: { establishmentId, seasonId },
            header: { 'Idempotency-Key': idempotencyKey },
          },
          body,
        }),
      ),
    onSuccess: (r) => {
      if (!r.dry_run) void qc.invalidateQueries({ queryKey: configKey(establishmentId) });
    },
  });
}
