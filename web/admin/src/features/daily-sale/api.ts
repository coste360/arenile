/**
 * Server state of the daily sale feature ("Vendita giornaliera", spec 14.3,
 * ADR 0020): types from the generated schema, query keys under the `tickets`
 * area (refreshed by the booking, allocation and cash SSE topics) and the
 * TanStack Query hooks. The endpoints answer 501 until the backend is built.
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

/** Price and availability of a sale while the form changes (no write). */
export function useQuoteDailySale(establishmentId: string) {
  const client = useApiClient();
  return useMutation<DailySaleQuote, ApiProblem, DailySaleQuoteRequest>({
    mutationFn: (body) =>
      call(() =>
        client.POST(`${BASE}/daily-sales/quote`, { params: { path: { establishmentId } }, body }),
      ),
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
      call(() =>
        client.POST(`${BASE}/daily-sales`, {
          params: { path: { establishmentId }, header: { 'Idempotency-Key': idempotencyKey } },
          body,
        }),
      ),
    onSuccess: invalidate,
  });
}

export function useAssignTicket(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateTickets(establishmentId);
  return useMutation<Ticket, ApiProblem, { ticketId: string; spotId: string }>({
    mutationFn: ({ ticketId, spotId }) =>
      call(() =>
        client.POST(`${BASE}/tickets/{ticketId}/assign`, {
          params: { path: { establishmentId, ticketId } },
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
    { ticketId: string; body: TicketVoidRequest; idempotencyKey: string }
  >({
    mutationFn: ({ ticketId, body, idempotencyKey }) =>
      call(() =>
        client.POST(`${BASE}/tickets/{ticketId}/void`, {
          params: {
            path: { establishmentId, ticketId },
            header: { 'Idempotency-Key': idempotencyKey },
          },
          body,
        }),
      ),
    onSettled: invalidate,
  });
}
