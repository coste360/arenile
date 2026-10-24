import {
  call,
  shouldRetry,
  useApiClient,
  useMe,
  type ApiProblem,
  type Me,
  type Schemas,
} from '@arenile/shared/api';
import { useQuery } from '@tanstack/react-query';

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

/** The update status for owners and managers; disabled for everyone else. */
export function useSystemUpdate() {
  const client = useApiClient();
  const me = useMe();
  return useQuery<SystemUpdate, ApiProblem>({
    queryKey: updateKeys.status,
    queryFn: () => call(() => client.GET('/api/admin/v1/system/update')),
    enabled: canSeeUpdates(me.data),
    retry: shouldRetry,
    // The server checks once a day: an hourly refresh is plenty.
    staleTime: 60 * 60_000,
    refetchOnWindowFocus: false,
  });
}
