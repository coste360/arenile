/**
 * Server state of the integrations feature (M5A: API keys and outgoing
 * webhooks, spec 17, 18.2): types from the generated schema, the query key
 * factory and TanStack Query hooks under establishmentKeys.area(id,
 * 'integrations').
 *
 * Secrets: create and rotate responses carry a secret shown once. The
 * mutations return it to the component (kept only in React state, cleared
 * when the dialog closes) with `gcTime: 0`, never write it into the query
 * cache (lists are invalidated, not patched) and never persist it. These
 * calls carry no Idempotency-Key on purpose: a stored replay would contain
 * the secret (see the OpenAPI contract).
 */
import {
  call,
  establishmentKeys,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import {
  keepPreviousData,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';

export type ApiKey = Schemas['ApiKey'];
export type ApiKeyCreate = Schemas['ApiKeyCreate'];
export type ApiKeyUpdate = Schemas['ApiKeyUpdate'];
export type ApiKeyCreated = Schemas['ApiKeyCreated'];
export type ApiKeyStatus = Schemas['ApiKeyStatus'];
export type Permission = Schemas['Permission'];
export type WebhookEndpoint = Schemas['WebhookEndpoint'];
export type WebhookEndpointCreate = Schemas['WebhookEndpointCreate'];
export type WebhookEndpointUpdate = Schemas['WebhookEndpointUpdate'];
export type WebhookEndpointCreated = Schemas['WebhookEndpointCreated'];
export type WebhookSecretRotated = Schemas['WebhookSecretRotated'];
export type WebhookDelivery = Schemas['WebhookDelivery'];
export type WebhookDeliveryDetail = Schemas['WebhookDeliveryDetail'];
export type WebhookDeliveryList = Schemas['WebhookDeliveryList'];
export type WebhookDeliveryStatus = Schemas['WebhookDeliveryStatus'];
export type WebhookEventType = Schemas['WebhookEventType'];
export type WebhookEndpointStatus = Schemas['WebhookEndpointStatus'];

/** Scopes a key may have: every permission except users.manage. */
export const API_KEY_SCOPES: readonly Permission[] = [
  'establishment.configure',
  'payments.configure',
  'bookings.write',
  'cash.operate',
  'refunds.issue',
  'operations.run',
  'bookings.read',
  'reports.read',
];

export const integrationKeys = {
  all: (est: string) => establishmentKeys.area(est, 'integrations'),
  apiKeys: (est: string, includeRevoked: boolean) =>
    [...establishmentKeys.area(est, 'integrations'), 'api-keys', { includeRevoked }] as const,
  endpoints: (est: string) => [...establishmentKeys.area(est, 'integrations'), 'webhooks'] as const,
  endpoint: (est: string, id: string) =>
    [...establishmentKeys.area(est, 'integrations'), 'webhooks', id] as const,
  deliveries: (est: string, id: string, status?: string) =>
    [
      ...establishmentKeys.area(est, 'integrations'),
      'webhooks',
      id,
      'deliveries',
      { status },
    ] as const,
  delivery: (est: string, id: string, deliveryId: string) =>
    [
      ...establishmentKeys.area(est, 'integrations'),
      'webhooks',
      id,
      'delivery',
      deliveryId,
    ] as const,
};

const base = '/api/admin/v1/establishments/{establishmentId}' as const;

/** Mutations whose result holds a secret: drop it from the mutation cache at once. */
const SECRET_MUTATION = { gcTime: 0 } as const;

// ---- API keys ----------------------------------------------------------------

export function useApiKeys(establishmentId: string, includeRevoked: boolean, enabled = true) {
  const client = useApiClient();
  return useQuery<ApiKey[], ApiProblem>({
    queryKey: integrationKeys.apiKeys(establishmentId, includeRevoked),
    queryFn: async () =>
      (
        await call(() =>
          client.GET(`${base}/api-keys`, {
            params: {
              path: { establishmentId },
              query: { include_revoked: includeRevoked || undefined },
            },
          }),
        )
      ).items,
    enabled,
    placeholderData: keepPreviousData,
    retry: shouldRetry,
  });
}

function useInvalidateIntegrations(establishmentId: string) {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: integrationKeys.all(establishmentId) });
}

export function useCreateApiKey(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateIntegrations(establishmentId);
  return useMutation<ApiKeyCreated, ApiProblem, ApiKeyCreate>({
    ...SECRET_MUTATION,
    mutationFn: (body) =>
      call(() => client.POST(`${base}/api-keys`, { params: { path: { establishmentId } }, body })),
    onSuccess: () => void invalidate(),
  });
}

export function useUpdateApiKey(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateIntegrations(establishmentId);
  return useMutation<ApiKey, ApiProblem, { id: string; patch: ApiKeyUpdate }>({
    mutationFn: ({ id, patch }) =>
      call(() =>
        client.PATCH(`${base}/api-keys/{apiKeyId}`, {
          params: { path: { establishmentId, apiKeyId: id } },
          body: patch,
        }),
      ),
    onSuccess: () => void invalidate(),
  });
}

export function useRevokeApiKey(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateIntegrations(establishmentId);
  return useMutation<void, ApiProblem, string>({
    mutationFn: async (id) => {
      await call(() =>
        client.DELETE(`${base}/api-keys/{apiKeyId}`, {
          params: { path: { establishmentId, apiKeyId: id } },
        }),
      );
    },
    onSuccess: () => void invalidate(),
  });
}

// ---- Webhook endpoints ------------------------------------------------------

export function useWebhookEndpoints(establishmentId: string, enabled = true) {
  const client = useApiClient();
  return useQuery<WebhookEndpoint[], ApiProblem>({
    queryKey: integrationKeys.endpoints(establishmentId),
    queryFn: async () =>
      (
        await call(() =>
          client.GET(`${base}/webhook-endpoints`, { params: { path: { establishmentId } } }),
        )
      ).items,
    enabled,
    retry: shouldRetry,
  });
}

export function useCreateWebhookEndpoint(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateIntegrations(establishmentId);
  return useMutation<WebhookEndpointCreated, ApiProblem, WebhookEndpointCreate>({
    ...SECRET_MUTATION,
    mutationFn: (body) =>
      call(() =>
        client.POST(`${base}/webhook-endpoints`, { params: { path: { establishmentId } }, body }),
      ),
    onSuccess: () => void invalidate(),
  });
}

export function useUpdateWebhookEndpoint(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateIntegrations(establishmentId);
  return useMutation<WebhookEndpoint, ApiProblem, { id: string; patch: WebhookEndpointUpdate }>({
    mutationFn: ({ id, patch }) =>
      call(() =>
        client.PATCH(`${base}/webhook-endpoints/{endpointId}`, {
          params: { path: { establishmentId, endpointId: id } },
          body: patch,
        }),
      ),
    onSuccess: () => void invalidate(),
  });
}

export function useDeleteWebhookEndpoint(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateIntegrations(establishmentId);
  return useMutation<void, ApiProblem, string>({
    mutationFn: async (id) => {
      await call(() =>
        client.DELETE(`${base}/webhook-endpoints/{endpointId}`, {
          params: { path: { establishmentId, endpointId: id } },
        }),
      );
    },
    onSuccess: () => void invalidate(),
  });
}

export function useRotateWebhookSecret(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateIntegrations(establishmentId);
  return useMutation<WebhookSecretRotated, ApiProblem, { id: string; graceHours: number }>({
    ...SECRET_MUTATION,
    mutationFn: ({ id, graceHours }) =>
      call(() =>
        client.POST(`${base}/webhook-endpoints/{endpointId}/rotate-secret`, {
          params: { path: { establishmentId, endpointId: id } },
          body: { grace_hours: graceHours },
        }),
      ),
    onSuccess: () => void invalidate(),
  });
}

/** One synchronous `webhook.test` attempt; resolves with the delivery (also when the receiver failed). */
export function useSendWebhookTest(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateIntegrations(establishmentId);
  return useMutation<WebhookDelivery, ApiProblem, string>({
    mutationFn: (id) =>
      call(() =>
        client.POST(`${base}/webhook-endpoints/{endpointId}/test`, {
          params: { path: { establishmentId, endpointId: id } },
        }),
      ),
    onSettled: () => void invalidate(),
  });
}

const DELIVERY_PAGE = 50;

/** Delivery log of an endpoint, newest first, cursor pagination ("load more"). */
export function useWebhookDeliveries(
  establishmentId: string,
  endpointId: string,
  status: WebhookDeliveryStatus | undefined,
) {
  const client = useApiClient();
  return useInfiniteQuery<WebhookDeliveryList, ApiProblem>({
    queryKey: integrationKeys.deliveries(establishmentId, endpointId, status),
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      call(() =>
        client.GET(`${base}/webhook-endpoints/{endpointId}/deliveries`, {
          params: {
            path: { establishmentId, endpointId },
            query: { status, cursor: pageParam as string | undefined, limit: DELIVERY_PAGE },
          },
        }),
      ),
    getNextPageParam: (last) => last.next_cursor || undefined,
    placeholderData: keepPreviousData,
    retry: shouldRetry,
  });
}

export function useWebhookDelivery(
  establishmentId: string,
  endpointId: string,
  deliveryId: string | undefined,
) {
  const client = useApiClient();
  return useQuery<WebhookDeliveryDetail, ApiProblem>({
    queryKey: integrationKeys.delivery(establishmentId, endpointId, deliveryId ?? ''),
    queryFn: () =>
      call(() =>
        client.GET(`${base}/webhook-endpoints/{endpointId}/deliveries/{deliveryId}`, {
          params: { path: { establishmentId, endpointId, deliveryId: deliveryId ?? '' } },
        }),
      ),
    enabled: !!deliveryId,
    retry: shouldRetry,
  });
}

export function useRedeliverWebhook(establishmentId: string, endpointId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateIntegrations(establishmentId);
  return useMutation<WebhookDelivery, ApiProblem, string>({
    mutationFn: (deliveryId) =>
      call(() =>
        client.POST(`${base}/webhook-endpoints/{endpointId}/deliveries/{deliveryId}/redeliver`, {
          params: { path: { establishmentId, endpointId, deliveryId } },
        }),
      ),
    onSuccess: () => void invalidate(),
  });
}
