import { call, type ApiClient } from '@arenile/shared/api';
import type { OfflineOperation } from './types';

const EST = '/api/admin/v1/establishments/{establishmentId}' as const;

/**
 * Sends one queued operation with its Idempotency-Key (spec 17.5): a replay
 * of an operation the server already applied returns the stored response
 * instead of applying it twice. Throws ApiProblem on failure (classified by
 * the shared queue).
 */
export async function sendOfflineOperation(client: ApiClient, op: OfflineOperation): Promise<void> {
  const path = { establishmentId: op.establishmentId, bookingId: op.bookingId };
  switch (op.kind) {
    case 'check_in':
      await call(() =>
        client.POST(`${EST}/bookings/{bookingId}/check-in`, {
          params: {
            path,
            header: {
              'Idempotency-Key': op.idempotencyKey,
              ...(op.ifMatch ? { 'If-Match': op.ifMatch } : {}),
            },
          },
          body: {},
        }),
      );
      return;
    case 'note':
      await call(() =>
        client.POST(`${EST}/bookings/{bookingId}/notes`, {
          params: { path, header: { 'Idempotency-Key': op.idempotencyKey } },
          body: { text: op.text, visibility: 'internal' },
        }),
      );
      return;
    case 'cash_payment':
      await call(() =>
        client.POST(`${EST}/bookings/{bookingId}/payments`, {
          params: { path, header: { 'Idempotency-Key': op.idempotencyKey } },
          body: { amount: op.amount, method: 'cash', ...(op.note ? { note: op.note } : {}) },
        }),
      );
      return;
  }
}
