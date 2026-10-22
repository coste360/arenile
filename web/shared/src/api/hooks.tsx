import { useMutation, useQuery, useQueryClient, type UseQueryOptions } from '@tanstack/react-query';
import { createContext, useContext, type ReactNode } from 'react';
import {
  ApiProblem,
  call,
  type ApiClient,
  type EstablishmentList,
  type LoginRequest,
  type Me,
  type PublicEstablishment,
} from './client';

const ApiClientContext = createContext<ApiClient | null>(null);

export function ApiClientProvider({
  client,
  children,
}: {
  client: ApiClient;
  children: ReactNode;
}) {
  return <ApiClientContext.Provider value={client}>{children}</ApiClientContext.Provider>;
}

export function useApiClient(): ApiClient {
  const client = useContext(ApiClientContext);
  if (!client) throw new Error('useApiClient must be used inside <ApiClientProvider>');
  return client;
}

/** Query keys of the M0 endpoints, shared by hooks and invalidations (spec 21.1). */
export const queryKeys = {
  me: ['auth', 'me'] as const,
  establishments: (params: { cursor?: string; limit?: number } = {}) =>
    ['establishments', params] as const,
  publicEstablishment: (slug: string) => ['public', slug, 'establishment'] as const,
};

/** Retries only transient failures, never 4xx problems. */
export function shouldRetry(failureCount: number, error: unknown): boolean {
  if (error instanceof ApiProblem && error.status >= 400 && error.status < 500) return false;
  return failureCount < 2;
}

export function meQueryOptions(client: ApiClient) {
  return {
    queryKey: queryKeys.me,
    queryFn: () => call(() => client.GET('/api/admin/v1/auth/me')),
    retry: shouldRetry,
    staleTime: 60_000,
    // Fail fast when offline so route guards never hang on a paused query.
    networkMode: 'always',
  } satisfies UseQueryOptions<Me, ApiProblem, Me, typeof queryKeys.me>;
}

/** Current user; errors with status 401 when not signed in. */
export function useMe() {
  const client = useApiClient();
  return useQuery<Me, ApiProblem, Me, typeof queryKeys.me>(meQueryOptions(client));
}

export function useLogin() {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<Me, ApiProblem, LoginRequest>({
    mutationFn: (body) => call(() => client.POST('/api/admin/v1/auth/login', { body })),
    onSuccess: (me) => {
      qc.setQueryData(queryKeys.me, me);
    },
  });
}

export function useLogout() {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<void, ApiProblem, void>({
    mutationFn: async () => {
      await call(() => client.POST('/api/admin/v1/auth/logout', {}));
    },
    onSettled: () => {
      qc.clear();
    },
  });
}

export function useEstablishments(params: { cursor?: string; limit?: number } = {}) {
  const client = useApiClient();
  return useQuery<EstablishmentList, ApiProblem>({
    queryKey: queryKeys.establishments(params),
    queryFn: () =>
      call(() => client.GET('/api/admin/v1/establishments', { params: { query: params } })),
    retry: shouldRetry,
  });
}

export function usePublicEstablishment(slug: string | null) {
  const client = useApiClient();
  return useQuery<PublicEstablishment, ApiProblem>({
    queryKey: queryKeys.publicEstablishment(slug ?? ''),
    queryFn: () =>
      call(() =>
        client.GET('/api/public/v1/{slug}', { params: { path: { slug: slug as string } } }),
      ),
    enabled: !!slug,
    retry: shouldRetry,
    staleTime: 5 * 60_000,
  });
}
