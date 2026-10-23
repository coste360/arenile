import {
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
import { ifMatch, newIdempotencyKey } from '../configuration/lib/validation';
import { callBooking } from '../bookings/problem';

export type Customer = Schemas['Customer'];
export type CustomerInput = Schemas['CustomerInput'];
export type CustomerList = Schemas['CustomerList'];
export type CustomerDetail = Schemas['CustomerDetail'];
export type CustomerExport = Schemas['CustomerExport'];
export type CustomerCredit = Schemas['CustomerCredit'];

const BASE = '/api/admin/v1/establishments/{establishmentId}' as const;

export interface CustomerFilters {
  q?: string;
  tag?: string;
  include_anonymized?: boolean;
}

/** Query keys under the `customers` area, invalidated by the SSE topic customer.changed. */
export const customerKeys = {
  all: (est: string) => establishmentKeys.area(est, 'customers'),
  list: (est: string, f: CustomerFilters) => [...customerKeys.all(est), 'list', f] as const,
  search: (est: string, q: string) => [...customerKeys.all(est), 'search', q] as const,
  detail: (est: string, id: string) => [...customerKeys.all(est), 'detail', id] as const,
};

const PAGE = 50;

/** Cursor-paginated customer list ("load more"). */
export function useCustomers(establishmentId: string, filters: CustomerFilters) {
  const client = useApiClient();
  return useInfiniteQuery<CustomerList, ApiProblem>({
    queryKey: customerKeys.list(establishmentId, filters),
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      callBooking(() =>
        client.GET(`${BASE}/customers`, {
          params: {
            path: { establishmentId },
            query: {
              q: filters.q || undefined,
              tag: filters.tag || undefined,
              include_anonymized: filters.include_anonymized || undefined,
              cursor: pageParam as string | undefined,
              limit: PAGE,
            },
          },
        }),
      ),
    getNextPageParam: (last) => last.next_cursor || undefined,
    placeholderData: keepPreviousData,
    retry: shouldRetry,
  });
}

/** Quick search for pickers and the duplicate hint (≥ 2 characters). */
export function useCustomerSearch(establishmentId: string, q: string, limit = 8) {
  const client = useApiClient();
  const term = q.trim();
  return useQuery<Customer[], ApiProblem>({
    queryKey: customerKeys.search(establishmentId, term),
    queryFn: async () =>
      (
        await callBooking(() =>
          client.GET(`${BASE}/customers`, {
            params: { path: { establishmentId }, query: { q: term, limit } },
          }),
        )
      ).items,
    enabled: term.length >= 2,
    placeholderData: keepPreviousData,
    staleTime: 15_000,
    retry: shouldRetry,
  });
}

export function useCustomer(establishmentId: string, customerId: string | undefined) {
  const client = useApiClient();
  return useQuery<CustomerDetail, ApiProblem>({
    queryKey: customerKeys.detail(establishmentId, customerId ?? ''),
    queryFn: () =>
      callBooking(() =>
        client.GET(`${BASE}/customers/{customerId}`, {
          params: { path: { establishmentId, customerId: customerId as string } },
        }),
      ),
    enabled: !!customerId,
    retry: shouldRetry,
  });
}

function useInvalidateCustomers(establishmentId: string) {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: customerKeys.all(establishmentId) });
}

export function useCreateCustomer(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateCustomers(establishmentId);
  return useMutation<Customer, ApiProblem, CustomerInput>({
    mutationFn: (body) =>
      callBooking(() =>
        client.POST(`${BASE}/customers`, {
          params: { path: { establishmentId }, header: { 'Idempotency-Key': newIdempotencyKey() } },
          body,
        }),
      ),
    onSuccess: () => void invalidate(),
  });
}

export interface UpdateCustomerVars {
  id: string;
  version: number;
  input: CustomerInput;
}

export function useUpdateCustomer(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateCustomers(establishmentId);
  return useMutation<Customer, ApiProblem, UpdateCustomerVars>({
    mutationFn: ({ id, version, input }) =>
      callBooking(() =>
        client.PUT(`${BASE}/customers/{customerId}`, {
          params: {
            path: { establishmentId, customerId: id },
            header: { 'If-Match': ifMatch(version) },
          },
          body: input,
        }),
      ),
    onSuccess: () => void invalidate(),
  });
}

export function useAnonymizeCustomer(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<Customer, ApiProblem, { id: string; version: number }>({
    mutationFn: ({ id, version }) =>
      callBooking(() =>
        client.POST(`${BASE}/customers/{customerId}/anonymize`, {
          params: {
            path: { establishmentId, customerId: id },
            header: { 'If-Match': ifMatch(version) },
          },
        }),
      ),
    onSuccess: () => {
      // Bookings show the customer name too.
      void qc.invalidateQueries({ queryKey: customerKeys.all(establishmentId) });
      void qc.invalidateQueries({ queryKey: establishmentKeys.area(establishmentId, 'bookings') });
    },
  });
}

/** GDPR export (spec 23): fetched on demand, never cached. */
export function useExportCustomer(establishmentId: string) {
  const client = useApiClient();
  return useMutation<CustomerExport, ApiProblem, string>({
    mutationFn: (customerId) =>
      callBooking(() =>
        client.GET(`${BASE}/customers/{customerId}/export`, {
          params: { path: { establishmentId, customerId } },
        }),
      ),
    gcTime: 0,
  });
}

/** Saves `data` as a pretty-printed JSON file in the browser. */
export function downloadJson(data: unknown, filename: string): void {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.rel = 'noopener';
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
}
