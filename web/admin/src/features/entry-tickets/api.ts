/**
 * Server state of the entry tickets feature (spec 14.3, ADR 0020): ticket
 * types, entry sales and the gate check-in, under the `tickets` area (the
 * booking, allocation and cash SSE topics refresh it).
 */
import {
  call,
  establishmentKeys,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { callBooking } from '../bookings/problem';
import { ifMatch } from '../configuration/lib/validation';

export type EntryTicketType = Schemas['EntryTicketType'];
export type EntryTicketTypeList = Schemas['EntryTicketTypeList'];
export type EntryTicketTypeInput = Schemas['EntryTicketTypeInput'];
export type EntryTicketTypeUpdate = Schemas['EntryTicketTypeUpdate'];
export type EntrySaleItem = Schemas['EntrySaleItem'];
export type EntrySaleQuoteRequest = Schemas['EntrySaleQuoteRequest'];
export type EntrySaleQuote = Schemas['EntrySaleQuote'];
export type EntrySaleCreate = Schemas['EntrySaleCreate'];
export type DailySaleResult = Schemas['DailySaleResult'];
export type GateCheckInResult = Schemas['GateCheckInResult'];
export type Ticket = Schemas['Ticket'];

const BASE = '/api/admin/v1/establishments/{establishmentId}' as const;

export const entryTicketKeys = {
  types: (est: string) => [...establishmentKeys.area(est, 'tickets'), 'entry-types'] as const,
  quote: (est: string) => [...establishmentKeys.area(est, 'tickets'), 'entry-quote'] as const,
};

export function useEntryTicketTypes(establishmentId: string) {
  const client = useApiClient();
  return useQuery<EntryTicketTypeList, ApiProblem>({
    queryKey: entryTicketKeys.types(establishmentId),
    queryFn: () =>
      call(() =>
        client.GET(`${BASE}/entry-ticket-types`, { params: { path: { establishmentId } } }),
      ),
    retry: shouldRetry,
  });
}

function useInvalidateTypes(establishmentId: string) {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: entryTicketKeys.types(establishmentId) });
}

export function useCreateEntryTicketType(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateTypes(establishmentId);
  return useMutation<EntryTicketType, ApiProblem, EntryTicketTypeInput>({
    mutationFn: (body) =>
      call(() =>
        client.POST(`${BASE}/entry-ticket-types`, { params: { path: { establishmentId } }, body }),
      ),
    onSuccess: invalidate,
  });
}

/** PATCH with If-Match on the version the form was opened with (412 when stale). */
export function useUpdateEntryTicketType(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateTypes(establishmentId);
  return useMutation<
    EntryTicketType,
    ApiProblem,
    { id: string; version: number; body: EntryTicketTypeUpdate }
  >({
    mutationFn: ({ id, version, body }) =>
      call(() =>
        client.PATCH(`${BASE}/entry-ticket-types/{entryTicketTypeId}`, {
          params: {
            path: { establishmentId, entryTicketTypeId: id },
            header: { 'If-Match': ifMatch(version) },
          },
          body,
        }),
      ),
    onSuccess: invalidate,
  });
}

/** 409 conflict when tickets were already sold: the type must be deactivated instead. */
export function useDeleteEntryTicketType(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateTypes(establishmentId);
  return useMutation<void, ApiProblem, string>({
    mutationFn: async (id) => {
      await call(() =>
        client.DELETE(`${BASE}/entry-ticket-types/{entryTicketTypeId}`, {
          params: { path: { establishmentId, entryTicketTypeId: id } },
        }),
      );
    },
    onSuccess: invalidate,
  });
}

/**
 * Price and remaining daily capacity of the selection (POST without side
 * effects). Keyed under the `tickets` area so a sale refreshes the remaining
 * capacity; disabled while nothing is selected.
 */
export function useEntrySaleQuote(establishmentId: string, body: EntrySaleQuoteRequest | null) {
  const client = useApiClient();
  return useQuery<EntrySaleQuote, ApiProblem>({
    queryKey: [...entryTicketKeys.quote(establishmentId), body] as const,
    enabled: body !== null,
    queryFn: () =>
      callBooking(() =>
        client.POST(`${BASE}/entry-sales/quote`, {
          params: { path: { establishmentId } },
          body: body as EntrySaleQuoteRequest,
        }),
      ),
    retry: shouldRetry,
  });
}

/**
 * Sells the entry tickets: confirmed booking on the quantity zones (shared
 * pool, 409 pool_exhausted), manual payment and one numbered ticket. Errors
 * are BookingProblem so the conflicts reach the alert.
 */
export function useCreateEntrySale(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<
    DailySaleResult,
    ApiProblem,
    { body: EntrySaleCreate; idempotencyKey: string }
  >({
    mutationFn: ({ body, idempotencyKey }) =>
      callBooking(() =>
        client.POST(`${BASE}/entry-sales`, {
          params: { path: { establishmentId }, header: { 'Idempotency-Key': idempotencyKey } },
          body,
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: establishmentKeys.all(establishmentId) }),
  });
}

/**
 * One scan at the gate: admitted, already_entered, voided or wrong_date.
 * `code` is the QR token; `manual` marks a code typed and confirmed by the
 * staff, the only case where the server also resolves the printed number
 * or the booking code.
 */
export function useGateCheckIn(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<GateCheckInResult, ApiProblem, { code: string; manual: boolean }>({
    mutationFn: ({ code, manual }) =>
      call(() =>
        client.POST(`${BASE}/entry-gate/check-in`, {
          params: { path: { establishmentId } },
          body: { code, manual },
        }),
      ),
    onSettled: () =>
      qc.invalidateQueries({ queryKey: establishmentKeys.area(establishmentId, 'tickets') }),
  });
}
