import type { ApiProblem } from '@arenile/shared/api';
import {
  call,
  shouldRetry,
  useApiClient,
  type Establishment,
  type Schemas,
} from '@arenile/shared/api';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ifMatch, newIdempotencyKey } from '../lib/validation';
import { configKey } from './resource';

export type EstablishmentUpdate = Schemas['EstablishmentUpdate'];
export type EstablishmentCreate = Schemas['EstablishmentCreate'];

export const establishmentKey = (id: string) => ['establishment', id] as const;

/** Full establishment (with settings and version) for the configuration pages. */
export function useEstablishment(establishmentId: string) {
  const client = useApiClient();
  return useQuery<Establishment, ApiProblem>({
    queryKey: establishmentKey(establishmentId),
    queryFn: () =>
      call(() =>
        client.GET('/api/admin/v1/establishments/{establishmentId}', {
          params: { path: { establishmentId } },
        }),
      ),
    retry: shouldRetry,
  });
}

export interface UpdateEstablishmentVars {
  /** Version the edit started from (If-Match). */
  version: number;
  patch: EstablishmentUpdate;
}

/**
 * Partial update with If-Match. On success the cache gets the new version, so
 * the next section saved on the same page sends the fresh ETag.
 */
export function useUpdateEstablishment(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<Establishment, ApiProblem, UpdateEstablishmentVars>({
    mutationFn: ({ version, patch }) =>
      call(() =>
        client.PATCH('/api/admin/v1/establishments/{establishmentId}', {
          params: { path: { establishmentId }, header: { 'If-Match': ifMatch(version) } },
          body: patch,
        }),
      ),
    onSuccess: (est) => {
      qc.setQueryData(establishmentKey(establishmentId), est);
      void qc.invalidateQueries({ queryKey: ['establishments'] });
      void qc.invalidateQueries({ queryKey: configKey(establishmentId) });
    },
  });
}

export function useCreateEstablishment() {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<Establishment, ApiProblem, EstablishmentCreate>({
    mutationFn: (body) =>
      call(() =>
        client.POST('/api/admin/v1/establishments', {
          params: { header: { 'Idempotency-Key': newIdempotencyKey() } },
          body,
        }),
      ),
    onSuccess: async (est) => {
      qc.setQueryData(establishmentKey(est.id), est);
      // The switcher lists establishments and `me` carries the new memberships.
      await Promise.all([
        qc.invalidateQueries({ queryKey: ['establishments'] }),
        qc.invalidateQueries({ queryKey: ['auth', 'me'] }),
      ]);
    },
  });
}
