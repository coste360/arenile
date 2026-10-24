import {
  call,
  shouldRetry,
  useApiClient,
  useMe,
  type ApiProblem,
  type Me,
  type Schemas,
} from '@arenile/shared/api';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { STATUS_REFETCH_MS, updateRefetchInterval } from './lib';

/**
 * Update notice (ADR 0025): GET /api/admin/v1/system/update, readable by
 * owners and managers. The server answers from the copy kept by its daily
 * background check: this never waits on the network.
 */
export type SystemUpdate = Schemas['SystemUpdate'];

export const updateKeys = {
  status: ['instance', 'update'] as const,
};

/**
 * Owners and managers (a membership with establishment.configure) may read
 * the update notice; API keys and other roles never ask for it.
 */
export function canSeeUpdates(me: Me | undefined): boolean {
  if (!me || me.principal_type !== 'user' || me.totp_enrolment_required) return false;
  return me.memberships.some((m) => m.permissions.includes('establishment.configure'));
}

/**
 * The update status for owners and managers; disabled for everyone else.
 * The server runs its first check shortly after start-up (`pending` until
 * then): meanwhile the status is read again every 15 s, so the banner shows
 * up without a reload; then hourly (the server checks once a day).
 */
export function useSystemUpdate() {
  const client = useApiClient();
  const me = useMe();
  return useQuery<SystemUpdate, ApiProblem>({
    queryKey: updateKeys.status,
    queryFn: () => call(() => client.GET('/api/admin/v1/system/update')),
    enabled: canSeeUpdates(me.data),
    retry: shouldRetry,
    staleTime: STATUS_REFETCH_MS,
    refetchInterval: (query) => updateRefetchInterval(query.state.data),
    refetchOnWindowFocus: false,
  });
}

/**
 * "Verifica aggiornamenti": POST /api/admin/v1/system/update/check runs the
 * check now (the server debounces it) and answers with the fresh status,
 * which replaces the cached one.
 */
export function useCheckSystemUpdate() {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<SystemUpdate, ApiProblem, void>({
    mutationFn: () => call(() => client.POST('/api/admin/v1/system/update/check')),
    onSuccess: (s) => qc.setQueryData(updateKeys.status, s),
  });
}
