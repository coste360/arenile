/**
 * Server state of the integrations feature (M5A): types from the generated
 * schema and the query key factory under establishmentKeys.area(id,
 * 'integrations'). Hooks are added here by the integrations frontend
 * builder. Create and rotate responses carry a secret shown once: keep it
 * only in component state, never in the query cache or storage.
 */
import { establishmentKeys, type Schemas } from '@arenile/shared/api';

export type ApiKey = Schemas['ApiKey'];
export type ApiKeyCreate = Schemas['ApiKeyCreate'];
export type ApiKeyUpdate = Schemas['ApiKeyUpdate'];
export type ApiKeyCreated = Schemas['ApiKeyCreated'];
export type Permission = Schemas['Permission'];
export type WebhookEndpoint = Schemas['WebhookEndpoint'];
export type WebhookEndpointCreate = Schemas['WebhookEndpointCreate'];
export type WebhookEndpointUpdate = Schemas['WebhookEndpointUpdate'];
export type WebhookEndpointCreated = Schemas['WebhookEndpointCreated'];
export type WebhookSecretRotated = Schemas['WebhookSecretRotated'];
export type WebhookDelivery = Schemas['WebhookDelivery'];
export type WebhookDeliveryDetail = Schemas['WebhookDeliveryDetail'];
export type WebhookEventType = Schemas['WebhookEventType'];

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
};
