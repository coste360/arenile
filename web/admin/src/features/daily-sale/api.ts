/**
 * Server state of the daily sale feature ("Vendita giornaliera", spec 14.3,
 * ADR 0020): types from the generated schema, query keys under the `tickets`
 * area (refreshed by the booking, allocation and cash SSE topics) and the
 * TanStack Query hooks.
 */
import {
  call,
  establishmentKeys,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { callBooking } from '../bookings/problem';
import { ifMatch } from '../configuration/lib/validation';

export type Ticket = Schemas['Ticket'];
export type TicketState = Schemas['TicketState'];
export type TicketKind = Schemas['TicketKind'];
export type TicketList = Schemas['TicketList'];
export type TicketDaySummary = Schemas['TicketDaySummary'];
export type TicketPrintData = Schemas['TicketPrintData'];
export type TicketPrintFormat = Schemas['TicketPrintFormat'];
export type TicketPaymentMethod = Schemas['TicketPaymentMethod'];
export type DailySaleQuoteRequest = Schemas['DailySaleQuoteRequest'];
export type DailySaleQuote = Schemas['DailySaleQuote'];
export type DailySaleCreate = Schemas['DailySaleCreate'];
export type DailySaleResult = Schemas['DailySaleResult'];
export type DailySaleSettings = Schemas['DailySaleSettings'];
export type DailySaleSettingsUpdate = Schemas['DailySaleSettingsUpdate'];
export type TicketAutoAssignResult = Schemas['TicketAutoAssignResult'];
export type TicketVoidRequest = Schemas['TicketVoidRequest'];

const BASE = '/api/admin/v1/establishments/{establishmentId}' as const;

export interface TicketFilters {
  date: string;
  state?: TicketState;
  kind?: TicketKind;
  zone_id?: string;
}

export const ticketKeys = {
  all: (est: string) => establishmentKeys.area(est, 'tickets'),
  list: (est: string, f: TicketFilters) => [...ticketKeys.all(est), 'list', f] as const,
  unassigned: (est: string, date: string, zoneId?: string) =>
    [...ticketKeys.all(est), 'unassigned', date, zoneId ?? null] as const,
  detail: (est: string, id: string) => [...ticketKeys.all(est), 'detail', id] as const,
  print: (est: string, id: string, format?: TicketPrintFormat) =>
    [...ticketKeys.all(est), 'print', id, format ?? null] as const,
  settings: (est: string) => [...ticketKeys.all(est), 'settings'] as const,
};

/** "Ticket del giorno": tickets of one day with the day summary. */
export function useTickets(establishmentId: string, filters: TicketFilters) {
  const client = useApiClient();
  return useQuery<TicketList, ApiProblem>({
    queryKey: ticketKeys.list(establishmentId, filters),
    // The operation also serves text/csv (format=csv): JSON is the default.
    queryFn: async () =>
      (await call(() =>
        client.GET(`${BASE}/tickets`, {
          params: { path: { establishmentId }, query: { ...filters, limit: 200 } },
        }),
      )) as TicketList,
    retry: shouldRetry,
  });
}

/** Zone-first tickets still to place ("Da assegnare"), by number. */
export function useUnassignedTickets(establishmentId: string, date: string, zoneId?: string) {
  const client = useApiClient();
  return useQuery({
    queryKey: ticketKeys.unassigned(establishmentId, date, zoneId),
    queryFn: () =>
      call(() =>
        client.GET(`${BASE}/tickets/unassigned`, {
          params: { path: { establishmentId }, query: { date, zone_id: zoneId } },
        }),
      ),
    retry: shouldRetry,
  });
}

export function useTicketPrint(
  establishmentId: string,
  ticketId: string | undefined,
  format?: TicketPrintFormat,
) {
  const client = useApiClient();
  return useQuery<TicketPrintData, ApiProblem>({
    queryKey: ticketKeys.print(establishmentId, ticketId ?? '', format),
    enabled: !!ticketId,
    queryFn: () =>
      call(() =>
        client.GET(`${BASE}/tickets/{ticketId}/print`, {
          params: { path: { establishmentId, ticketId: ticketId as string }, query: { format } },
        }),
      ),
    retry: shouldRetry,
  });
}

export function useDailySaleSettings(establishmentId: string) {
  const client = useApiClient();
  return useQuery<DailySaleSettings, ApiProblem>({
    queryKey: ticketKeys.settings(establishmentId),
    queryFn: () =>
      call(() =>
        client.GET(`${BASE}/daily-sale-settings`, { params: { path: { establishmentId } } }),
      ),
    retry: shouldRetry,
  });
}

/** Saves the ticket settings (establishment.configure) with If-Match on the version read. */
export function useUpdateDailySaleSettings(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<
    DailySaleSettings,
    ApiProblem,
    { version: number; body: DailySaleSettingsUpdate }
  >({
    mutationFn: ({ version, body }) =>
      call(() =>
        client.PUT(`${BASE}/daily-sale-settings`, {
          params: { path: { establishmentId }, header: { 'If-Match': ifMatch(version) } },
          body,
        }),
      ),
    onSuccess: (s) => qc.setQueryData(ticketKeys.settings(establishmentId), s),
  });
}

/**
 * Price and availability of a sale while the form changes (no write). The
 * body is the cache key: the same selection is quoted once; the previous
 * figures stay on screen while the next selection loads.
 */
export function useDailySaleQuote(establishmentId: string, body: DailySaleQuoteRequest | null) {
  const client = useApiClient();
  return useQuery<DailySaleQuote, ApiProblem>({
    queryKey: [...ticketKeys.all(establishmentId), 'quote', body] as const,
    enabled: !!body,
    // callBooking keeps the policy violations of the problem (same-day cutoff...).
    queryFn: () =>
      callBooking(() =>
        client.POST(`${BASE}/daily-sales/quote`, {
          params: { path: { establishmentId } },
          body: body as DailySaleQuoteRequest,
        }),
      ),
    placeholderData: keepPreviousData,
    retry: shouldRetry,
    staleTime: 10_000,
  });
}

/** Print data of a ticket for an immediate print (first print after the sale). */
export function useFetchTicketPrint(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return (ticketId: string, format?: TicketPrintFormat) =>
    qc.fetchQuery<TicketPrintData, ApiProblem>({
      queryKey: ticketKeys.print(establishmentId, ticketId, format),
      queryFn: () =>
        call(() =>
          client.GET(`${BASE}/tickets/{ticketId}/print`, {
            params: { path: { establishmentId, ticketId }, query: { format } },
          }),
        ),
      staleTime: 0,
    });
}

function useInvalidateTickets(establishmentId: string) {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: establishmentKeys.all(establishmentId) });
}

/** Creates the sale: confirmed booking, manual payment and numbered ticket. */
export function useCreateDailySale(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateTickets(establishmentId);
  return useMutation<
    DailySaleResult,
    ApiProblem,
    { body: DailySaleCreate; idempotencyKey: string }
  >({
    mutationFn: ({ body, idempotencyKey }) =>
      callBooking(() =>
        client.POST(`${BASE}/daily-sales`, {
          params: { path: { establishmentId }, header: { 'Idempotency-Key': idempotencyKey } },
          body,
        }),
      ),
    onSuccess: invalidate,
  });
}

/**
 * Places a "da assegnare" ticket on a free spot of its zone. If-Match
 * carries the ticket version read (428 without it, 412 when stale, INV-80):
 * a stale tab never moves a ticket that changed in the meantime.
 */
export function useAssignTicket(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateTickets(establishmentId);
  return useMutation<Ticket, ApiProblem, { ticketId: string; spotId: string; version: number }>({
    mutationFn: ({ ticketId, spotId, version }) =>
      call(() =>
        client.POST(`${BASE}/tickets/{ticketId}/assign`, {
          params: { path: { establishmentId, ticketId }, header: { 'If-Match': ifMatch(version) } },
          body: { spot_id: spotId },
        }),
      ),
    onSettled: invalidate,
  });
}

export function useAutoAssignTickets(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateTickets(establishmentId);
  return useMutation<TicketAutoAssignResult, ApiProblem, { date: string; zoneId?: string }>({
    mutationFn: ({ date, zoneId }) =>
      call(() =>
        client.POST(`${BASE}/tickets/auto-assign`, {
          params: { path: { establishmentId } },
          body: { date, zone_id: zoneId },
        }),
      ),
    onSettled: invalidate,
  });
}

export function useVoidTicket(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateTickets(establishmentId);
  return useMutation<
    Ticket,
    ApiProblem,
    { ticketId: string; version: number; body: TicketVoidRequest; idempotencyKey: string }
  >({
    mutationFn: ({ ticketId, version, body, idempotencyKey }) =>
      call(() =>
        client.POST(`${BASE}/tickets/{ticketId}/void`, {
          params: {
            path: { establishmentId, ticketId },
            header: { 'Idempotency-Key': idempotencyKey, 'If-Match': ifMatch(version) },
          },
          body,
        }),
      ),
    onSettled: invalidate,
  });
}

/** Reprint: counts the print, writes the audit and returns the data marked "Ristampa". */
export function useReprintTicket(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateTickets(establishmentId);
  return useMutation<TicketPrintData, ApiProblem, { ticketId: string; format?: TicketPrintFormat }>(
    {
      mutationFn: ({ ticketId, format }) =>
        call(() =>
          client.POST(`${BASE}/tickets/{ticketId}/reprint`, {
            params: { path: { establishmentId, ticketId } },
            body: format ? { format } : {},
          }),
        ),
      onSuccess: invalidate,
    },
  );
}

/** Moves an assigned ticket (or reassigns it) to another free spot: 409 when taken. */
export function useMoveTicket(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateTickets(establishmentId);
  return useMutation<Ticket, ApiProblem, { ticketId: string; spotId: string; version: number }>({
    mutationFn: ({ ticketId, spotId, version }) =>
      call(() =>
        client.POST(`${BASE}/tickets/{ticketId}/move`, {
          params: { path: { establishmentId, ticketId }, header: { 'If-Match': ifMatch(version) } },
          body: { spot_id: spotId },
        }),
      ),
    onSettled: invalidate,
  });
}
