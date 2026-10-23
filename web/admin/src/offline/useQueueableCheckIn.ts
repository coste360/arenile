import {
  ApiProblem,
  call,
  establishmentKeys,
  useApiClient,
  type EstablishmentArea,
  type Schemas,
} from '@arenile/shared/api';
import { defaultNewId } from '@arenile/shared/offline';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useOfflineRuntime } from './context';
import type { OfflineOperation } from './types';

const EST = '/api/admin/v1/establishments/{establishmentId}' as const;

const CHANGED_AREAS: readonly EstablishmentArea[] = [
  'operations',
  'planning',
  'availability',
  'bookings',
  'season-passes',
];

export interface QueueableCheckInVars {
  bookingId: string;
  /** Booking version read with the map, sent as If-Match. */
  version?: number;
  /** Shown in the offline queue (spot label and booking code). */
  label?: string;
}

export type CheckInOutcome =
  | { queued: false; booking: Schemas['BookingDetail'] }
  | { queued: true; operation: OfflineOperation };

/** True when the check-in was queued instead of applied. */
export function isQueued(outcome: CheckInOutcome | undefined): boolean {
  return !!outcome?.queued;
}

/**
 * Check-in that keeps working offline (spec 21.2). Online it is sent at
 * once with an Idempotency-Key; offline, or when the request does not reach
 * the server, it is queued with that same key (a request that did reach
 * the server is then replayed safely) and shown as checked in on the cached
 * map. Same variables as useCheckIn (operations/api), so pages can switch.
 */
export function useQueueableCheckIn(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  const runtime = useOfflineRuntime();
  return useMutation<CheckInOutcome, ApiProblem, QueueableCheckInVars>({
    mutationFn: async ({ bookingId, version, label }) => {
      const idempotencyKey = defaultNewId();
      const ifMatch = version !== undefined ? `"${version}"` : undefined;
      const enqueue = async (): Promise<CheckInOutcome> => ({
        queued: true,
        operation: await (runtime as NonNullable<typeof runtime>).enqueue({
          kind: 'check_in',
          establishmentId,
          bookingId,
          label: label ?? '',
          idempotencyKey,
          ...(ifMatch ? { ifMatch } : {}),
        }),
      });
      if (runtime && !navigator.onLine) return enqueue();
      try {
        const booking = await call(() =>
          client.POST(`${EST}/bookings/{bookingId}/check-in`, {
            params: {
              path: { establishmentId, bookingId },
              header: {
                'Idempotency-Key': idempotencyKey,
                ...(ifMatch ? { 'If-Match': ifMatch } : {}),
              },
            },
            body: {},
          }),
        );
        return { queued: false, booking };
      } catch (err) {
        if (runtime && err instanceof ApiProblem && err.isNetworkError) return enqueue();
        throw err;
      }
    },
    mutationKey: ['offline', 'check-in', establishmentId],
    // Offline the mutation must run (to queue), not pause.
    networkMode: 'always',
    onSuccess: (outcome) => {
      if (outcome.queued) return;
      for (const area of CHANGED_AREAS) {
        void qc.invalidateQueries({ queryKey: establishmentKeys.area(establishmentId, area) });
      }
    },
  });
}
