import {
  call,
  queryKeys,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type components,
} from '@arenile/shared/api';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

export type FirstRunRequest = components['schemas']['FirstRunRequest'];
export type Me = components['schemas']['Me'];

export const firstRunStatusKey = ['desktop', 'first-run'] as const;

/** Whether the owner still has to be created (desktop edition, loopback only). */
export function useFirstRunStatus() {
  const client = useApiClient();
  return useQuery({
    queryKey: firstRunStatusKey,
    queryFn: () => call(() => client.GET('/api/desktop/v1/first-run')),
    retry: shouldRetry,
    networkMode: 'always',
  });
}

/**
 * Creates the owner with the one-time setup token and stores the returned
 * session as the current user, so the authenticated routes open directly.
 */
export function useCompleteFirstRun() {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<Me, ApiProblem, FirstRunRequest>({
    mutationFn: (body) => call(() => client.POST('/api/desktop/v1/first-run', { body })),
    onSuccess: (me) => {
      qc.setQueryData(queryKeys.me, me);
      qc.setQueryData(firstRunStatusKey, { required: false });
    },
  });
}
