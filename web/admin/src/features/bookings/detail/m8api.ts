import {
  call,
  establishmentKeys,
  useApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { useMutation, useQueryClient } from '@tanstack/react-query';

/*
 * Booking actions of the cash desk (spec 30.9, ADR 0028): duplicate on new
 * dates, share summary (wa.me link), printable summary with QR.
 */

export type BookingDuplicateRequest = Schemas['BookingDuplicateRequest'];
export type BookingDuplicateResult = Schemas['BookingDuplicateResult'];
export type BookingShareSummary = Schemas['BookingShareSummary'];

const EST = '/api/admin/v1/establishments/{establishmentId}' as const;

const AREAS = ['bookings', 'operations', 'planning', 'availability', 'customers'] as const;

/** Duplicate (dry_run previews price and conflicts; 201 creates). */
export function useDuplicateBooking(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<
    BookingDuplicateResult,
    ApiProblem,
    { bookingId: string; body: BookingDuplicateRequest; idempotencyKey: string }
  >({
    mutationFn: ({ bookingId, body, idempotencyKey }) =>
      call(() =>
        client.POST(`${EST}/bookings/{bookingId}/duplicate`, {
          params: {
            path: { establishmentId, bookingId },
            header: { 'Idempotency-Key': idempotencyKey },
          },
          body,
        }),
      ),
    onSuccess: (res) => {
      if (!res.applied) return;
      for (const area of AREAS) {
        void qc.invalidateQueries({ queryKey: establishmentKeys.area(establishmentId, area) });
      }
    },
  });
}

/**
 * Ready text and wa.me link of a booking (a new manage link each call, so
 * it is fetched on demand and never cached).
 */
export function useShareSummary(establishmentId: string) {
  const client = useApiClient();
  return useMutation<BookingShareSummary, ApiProblem, { bookingId: string }>({
    mutationFn: ({ bookingId }) =>
      call(() =>
        client.GET(`${EST}/bookings/{bookingId}/share-summary`, {
          params: { path: { establishmentId, bookingId } },
        }),
      ),
  });
}

/** URL of the printable summary with the booking QR (HTML, printed by the browser). */
export function summaryPrintUrl(
  establishmentId: string,
  bookingId: string,
  paper: 'a4' | 'receipt80' = 'a4',
): string {
  return `/api/admin/v1/establishments/${encodeURIComponent(establishmentId)}/bookings/${encodeURIComponent(bookingId)}/summary-print?paper=${paper}`;
}
