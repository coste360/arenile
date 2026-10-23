/**
 * Server state of the credits feature (M5B: customer credit ledger, spec
 * 13.4): types from the generated schema, query keys under the `credits`
 * area (refreshed by the booking and customer SSE topics) and the hooks.
 */
import {
  establishmentKeys,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { callBooking } from '../bookings/problem';

export type CustomerCredit = Schemas['CustomerCredit'];
export type CustomerCreditLedger = Schemas['CustomerCreditLedger'];
export type CustomerCreditCreate = Schemas['CustomerCreditCreate'];
export type CustomerCreditExpiring = Schemas['CustomerCreditExpiring'];

const BASE = '/api/admin/v1/establishments/{establishmentId}' as const;

export const creditKeys = {
  all: (est: string) => establishmentKeys.area(est, 'credits'),
  ledger: (est: string, customerId: string) =>
    [...creditKeys.all(est), 'ledger', customerId] as const,
};

/** Balance, expiring credits and movements (newest first, cursor pages of 100). */
export function useCustomerCredits(establishmentId: string, customerId: string | undefined) {
  const client = useApiClient();
  return useInfiniteQuery<CustomerCreditLedger, ApiProblem>({
    queryKey: creditKeys.ledger(establishmentId, customerId ?? ''),
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      callBooking(() =>
        client.GET(`${BASE}/customers/{customerId}/credits`, {
          params: {
            path: { establishmentId, customerId: customerId as string },
            query: { cursor: pageParam as string | undefined, limit: 100 },
          },
        }),
      ),
    getNextPageParam: (last) => last.next_cursor || undefined,
    enabled: !!customerId,
    retry: shouldRetry,
  });
}

/** Refreshes the ledgers and what shows a customer's credit (detail, bookings). */
export function useInvalidateCredits(establishmentId: string) {
  const qc = useQueryClient();
  return () => {
    for (const area of ['credits', 'customers', 'bookings', 'payments'] as const) {
      void qc.invalidateQueries({ queryKey: establishmentKeys.area(establishmentId, area) });
    }
  };
}

/** "Riconosci credito" (refunds.issue): goodwill / manual credit or an adjustment. */
export function useCreateCustomerCredit(establishmentId: string, customerId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateCredits(establishmentId);
  return useMutation<
    CustomerCredit,
    ApiProblem,
    { body: CustomerCreditCreate; idempotencyKey: string }
  >({
    mutationFn: ({ body, idempotencyKey }) =>
      callBooking(() =>
        client.POST(`${BASE}/customers/{customerId}/credits`, {
          params: {
            path: { establishmentId, customerId },
            header: { 'Idempotency-Key': idempotencyKey },
          },
          body,
        }),
      ),
    onSuccess: () => invalidate(),
  });
}
