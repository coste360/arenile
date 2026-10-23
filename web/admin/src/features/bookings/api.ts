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
import type { BookingAction, BookingStatus, Channel, Slots } from './model';
import { callBooking } from './problem';

export type BookingSummary = Schemas['BookingSummary'];
export type BookingList = Schemas['BookingList'];
export type BookingDetail = Schemas['BookingDetail'];
export type BookingCreate = Schemas['BookingCreate'];
export type BookingQuote = Schemas['BookingQuote'];
export type BookingPatch = Schemas['BookingPatch'];
export type BookingChangeResult = Schemas['BookingChangeResult'];
export type BookingCancelRequest = Schemas['BookingCancelRequest'];
export type BookingCancelResult = Schemas['BookingCancelResult'];
export type BookingMoveRequest = Schemas['BookingMoveRequest'];
export type BookingNoteInput = Schemas['BookingNoteInput'];
export type RefundPreview = Schemas['RefundPreview'];
export type Payment = Schemas['Payment'];
export type ManualPaymentInput = Schemas['ManualPaymentInput'];
export type RefundInput = Schemas['RefundInput'];
export type AvailabilityResult = Schemas['AvailabilityResult'];
export type PriceBreakdown = Schemas['PriceBreakdown'];
export type PolicyEvaluation = Schemas['PolicyEvaluation'];
export type PolicyRules = Schemas['PolicyRules'];

const BASE = '/api/admin/v1/establishments/{establishmentId}' as const;

export interface BookingFilters {
  from?: string;
  to?: string;
  status?: BookingStatus[];
  customer_id?: string;
  spot_id?: string;
  channel?: Channel;
  season_pass?: boolean;
  balance_due?: boolean;
  q?: string;
}

/** Query keys under the `bookings` area (invalidated by booking/allocation/cash SSE topics). */
export const bookingKeys = {
  all: (est: string) => establishmentKeys.area(est, 'bookings'),
  list: (est: string, f: BookingFilters) => [...bookingKeys.all(est), 'list', f] as const,
  detail: (est: string, id: string) => [...bookingKeys.all(est), 'detail', id] as const,
  quote: (est: string, body: unknown) => [...bookingKeys.all(est), 'quote', body] as const,
  availability: (est: string, params: AvailabilityParams) =>
    [...establishmentKeys.area(est, 'availability'), 'bookings', params] as const,
};

const PAGE = 50;

/** Cursor-paginated booking list ("load more"), sorted by starts_on on the server. */
export function useBookings(establishmentId: string, filters: BookingFilters) {
  const client = useApiClient();
  return useInfiniteQuery<BookingList, ApiProblem>({
    queryKey: bookingKeys.list(establishmentId, filters),
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      callBooking(() =>
        client.GET(`${BASE}/bookings`, {
          params: {
            path: { establishmentId },
            query: {
              ...filters,
              status: filters.status?.length ? filters.status : undefined,
              q: filters.q || undefined,
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

export function useBooking(establishmentId: string, bookingId: string | undefined) {
  const client = useApiClient();
  return useQuery<BookingDetail, ApiProblem>({
    queryKey: bookingKeys.detail(establishmentId, bookingId ?? ''),
    queryFn: () =>
      callBooking(() =>
        client.GET(`${BASE}/bookings/{bookingId}`, {
          params: { path: { establishmentId, bookingId: bookingId as string } },
        }),
      ),
    enabled: !!bookingId,
    retry: shouldRetry,
  });
}

/**
 * Live quote of a selection (POST /bookings/quote, nothing written): price
 * breakdown, effective policy, deposit and availability. `body` null
 * disables it; callers debounce the body.
 */
export function useBookingQuote(establishmentId: string, body: BookingCreate | null) {
  const client = useApiClient();
  return useQuery<BookingQuote, ApiProblem>({
    queryKey: bookingKeys.quote(establishmentId, body),
    queryFn: () =>
      callBooking(() =>
        client.POST(`${BASE}/bookings/quote`, {
          params: { path: { establishmentId } },
          body: body as BookingCreate,
        }),
      ),
    enabled: body !== null,
    placeholderData: keepPreviousData,
    staleTime: 10_000,
    retry: false,
  });
}

export interface AvailabilityParams {
  from: string;
  to: string;
  slots: Slots;
  map_id?: string;
  zone_id?: string;
  pack_id?: string;
}

/** Spot and pool availability on a range, for the pickers (desk channel). */
export function useAvailability(establishmentId: string, params: AvailabilityParams | null) {
  const client = useApiClient();
  return useQuery<AvailabilityResult, ApiProblem>({
    queryKey: bookingKeys.availability(
      establishmentId,
      params ?? { from: '', to: '', slots: 'FULL' },
    ),
    queryFn: () =>
      callBooking(() =>
        client.GET(`${BASE}/availability`, {
          params: {
            path: { establishmentId },
            query: { ...(params as AvailabilityParams), channel: 'desk' },
          },
        }),
      ),
    enabled: params !== null,
    placeholderData: keepPreviousData,
    retry: shouldRetry,
  });
}

/** Invalidates every area a booking change may touch (the SSE stream does it too, later). */
function useInvalidateOperational(establishmentId: string) {
  const qc = useQueryClient();
  return () => {
    for (const area of [
      'bookings',
      'operations',
      'planning',
      'availability',
      'customers',
      'season-passes',
      'payments',
      'cash',
    ] as const) {
      void qc.invalidateQueries({ queryKey: establishmentKeys.area(establishmentId, area) });
    }
  };
}

export interface CreateBookingVars {
  body: BookingCreate;
  /** Same key for retries of the same body (offline queue, double clicks). */
  idempotencyKey: string;
}

export function useCreateBooking(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateOperational(establishmentId);
  return useMutation<BookingDetail, ApiProblem, CreateBookingVars>({
    mutationFn: ({ body, idempotencyKey }) =>
      callBooking(() =>
        client.POST(`${BASE}/bookings`, {
          params: { path: { establishmentId }, header: { 'Idempotency-Key': idempotencyKey } },
          body,
        }),
      ),
    onSuccess: () => invalidate(),
  });
}

/** Stores a fresh detail in the cache after a mutation that returns it. */
function useSetDetail(establishmentId: string) {
  const qc = useQueryClient();
  return (detail: BookingDetail) =>
    qc.setQueryData(bookingKeys.detail(establishmentId, detail.id), detail);
}

const ACTION_PATHS = {
  confirm: `${BASE}/bookings/{bookingId}/confirm`,
  approve: `${BASE}/bookings/{bookingId}/approve`,
  reject: `${BASE}/bookings/{bookingId}/reject`,
  'check-in': `${BASE}/bookings/{bookingId}/check-in`,
  'no-show': `${BASE}/bookings/{bookingId}/no-show`,
} as const;

export interface TransitionVars {
  action: Exclude<BookingAction, 'cancel'>;
  bookingId: string;
  version: number;
  note?: string;
}

/** State machine transitions other than cancel (409 invalid_transition, 412 on a stale version). */
export function useBookingTransition(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateOperational(establishmentId);
  const setDetail = useSetDetail(establishmentId);
  return useMutation<BookingDetail, ApiProblem, TransitionVars>({
    mutationFn: ({ action, bookingId, version, note }) =>
      callBooking(() =>
        client.POST(ACTION_PATHS[action], {
          params: {
            path: { establishmentId, bookingId },
            header: { 'If-Match': ifMatch(version) },
          },
          body: note ? { note } : {},
        }),
      ),
    onSuccess: (detail) => {
      setDetail(detail);
      invalidate();
    },
  });
}

export interface CancelVars {
  bookingId: string;
  version: number;
  request: BookingCancelRequest;
}

/** Cancel with settlement; `request.dry_run` only previews the refund (nothing written). */
export function useCancelBooking(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateOperational(establishmentId);
  const setDetail = useSetDetail(establishmentId);
  return useMutation<BookingCancelResult, ApiProblem, CancelVars>({
    mutationFn: ({ bookingId, version, request }) =>
      callBooking(() =>
        client.POST(`${BASE}/bookings/{bookingId}/cancel`, {
          params: {
            path: { establishmentId, bookingId },
            header: { 'If-Match': ifMatch(version) },
          },
          body: request,
        }),
      ),
    onSuccess: (result) => {
      if (!result.applied) return;
      setDetail(result.booking);
      invalidate();
    },
  });
}

export interface PatchVars {
  bookingId: string;
  version: number;
  patch: BookingPatch;
  dryRun?: boolean;
}

/** Dates / lines / notes change with re-quote; dry run first to show the difference. */
export function usePatchBooking(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateOperational(establishmentId);
  const setDetail = useSetDetail(establishmentId);
  return useMutation<BookingChangeResult, ApiProblem, PatchVars>({
    mutationFn: ({ bookingId, version, patch, dryRun }) =>
      callBooking(() =>
        client.PATCH(`${BASE}/bookings/{bookingId}`, {
          params: {
            path: { establishmentId, bookingId },
            query: dryRun ? { dry_run: true } : {},
            header: { 'If-Match': ifMatch(version) },
          },
          body: patch,
        }),
      ),
    onSuccess: (result) => {
      if (!result.applied) return;
      setDetail(result.booking);
      invalidate();
    },
  });
}

export interface MoveVars {
  bookingId: string;
  version: number;
  request: BookingMoveRequest;
}

export function useMoveBooking(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateOperational(establishmentId);
  const setDetail = useSetDetail(establishmentId);
  return useMutation<BookingChangeResult, ApiProblem, MoveVars>({
    mutationFn: ({ bookingId, version, request }) =>
      callBooking(() =>
        client.POST(`${BASE}/bookings/{bookingId}/move`, {
          params: {
            path: { establishmentId, bookingId },
            header: { 'If-Match': ifMatch(version) },
          },
          body: request,
        }),
      ),
    onSuccess: (result) => {
      if (!result.applied) return;
      setDetail(result.booking);
      invalidate();
    },
  });
}

export function useAddBookingNote(establishmentId: string) {
  const client = useApiClient();
  const setDetail = useSetDetail(establishmentId);
  return useMutation<BookingDetail, ApiProblem, { bookingId: string; note: BookingNoteInput }>({
    mutationFn: ({ bookingId, note }) =>
      callBooking(() =>
        client.POST(`${BASE}/bookings/{bookingId}/notes`, {
          params: { path: { establishmentId, bookingId } },
          body: note,
        }),
      ),
    onSuccess: (detail) => setDetail(detail),
  });
}

export interface PaymentVars<T> {
  bookingId: string;
  input: T;
  idempotencyKey?: string;
}

/** Manual payment (driver manual); cash needs an open cash session (409 cash_session_closed). */
export function useRecordPayment(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateOperational(establishmentId);
  return useMutation<Payment, ApiProblem, PaymentVars<ManualPaymentInput>>({
    mutationFn: ({ bookingId, input, idempotencyKey }) =>
      callBooking(() =>
        client.POST(`${BASE}/bookings/{bookingId}/payments`, {
          params: {
            path: { establishmentId, bookingId },
            header: { 'Idempotency-Key': idempotencyKey ?? newIdempotencyKey() },
          },
          body: input,
        }),
      ),
    onSuccess: () => invalidate(),
  });
}

/** Manual refund or credit (refunds.issue); refunds never exceed the paid amount (INV-24). */
export function useRecordRefund(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateOperational(establishmentId);
  return useMutation<Payment, ApiProblem, PaymentVars<RefundInput>>({
    mutationFn: ({ bookingId, input, idempotencyKey }) =>
      callBooking(() =>
        client.POST(`${BASE}/bookings/{bookingId}/refunds`, {
          params: {
            path: { establishmentId, bookingId },
            header: { 'Idempotency-Key': idempotencyKey ?? newIdempotencyKey() },
          },
          body: input,
        }),
      ),
    onSuccess: () => invalidate(),
  });
}
