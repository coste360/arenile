import type { WebhookDelivery, WebhookEndpoint } from '../api';

/**
 * A delivery can be queued again unless it is already waiting or the
 * endpoint was disabled automatically (the server answers 409 otherwise).
 */
export function canRedeliver(
  endpoint: Pick<WebhookEndpoint, 'status'>,
  delivery: Pick<WebhookDelivery, 'status'>,
): boolean {
  return delivery.status !== 'pending' && endpoint.status !== 'disabled';
}
