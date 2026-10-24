/**
 * Server state of the cash feature: TanStack Query hooks on the generated
 * client (repository role on the client side). Query keys live under
 * establishmentKeys.area(id, 'cash' | 'fiscal' | 'bookings') so the SSE
 * stream (cash.changed, booking.changed) invalidates them by prefix.
 *
 * Writes carry an Idempotency-Key (safe retries, offline queue of spec 21.2)
 * and the close uses If-Match on the session version (optimistic concurrency).
 */
import {
  ApiProblem,
  call,
  establishmentKeys,
  shouldRetry,
  useApiClient,
  type Schemas,
} from '@arenile/shared/api';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { CorrispettiviQuery, CorrispettiviReport } from './lib/corrispettivi';

export type CashSession = Schemas['CashSession'];
export type CashSessionDetail = Schemas['CashSessionDetail'];
export type CashMovement = Schemas['CashMovement'];
export type CashMovementInput = Schemas['CashMovementInput'];
export type CashSessionOpen = Schemas['CashSessionOpen'];
export type CashSessionClose = Schemas['CashSessionClose'];
export type Payment = Schemas['Payment'];
export type ManualPaymentInput = Schemas['ManualPaymentInput'];
export type BookingSummary = Schemas['BookingSummary'];
export type CashRegister = Schemas['CashRegister'];
export type CashRegisterInput = Schemas['CashRegisterInput'];

/** Code of the default register ("Cassa principale", always present). */
export const MAIN_REGISTER = 'main';

const base = '/api/admin/v1/establishments/{establishmentId}' as const;

export function newIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `k-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
}

export const cashKeys = {
  all: (est: string) => establishmentKeys.area(est, 'cash'),
  current: (est: string, registerId: string) =>
    [...establishmentKeys.area(est, 'cash'), 'current', registerId] as const,
  registers: (est: string, includeInactive: boolean) =>
    [...establishmentKeys.area(est, 'cash'), 'registers', includeInactive] as const,
  history: (est: string) => [...establishmentKeys.area(est, 'cash'), 'history'] as const,
  session: (est: string, id: string) =>
    [...establishmentKeys.area(est, 'cash'), 'session', id] as const,
  corrispettivi: (est: string, q: CorrispettiviQuery) =>
    [...establishmentKeys.area(est, 'fiscal'), 'corrispettivi', q] as const,
  bookingSearch: (est: string, q: string) =>
    [...establishmentKeys.area(est, 'bookings'), 'cash-search', q] as const,
};

/**
 * Open session of the register (by id; the default register when empty)
 * with movements and totals; null when none is open (404).
 */
export function useCurrentSession(establishmentId: string, registerId = '', enabled = true) {
  const client = useApiClient();
  return useQuery<CashSessionDetail | null, ApiProblem>({
    queryKey: cashKeys.current(establishmentId, registerId || MAIN_REGISTER),
    queryFn: async () => {
      try {
        return await call(() =>
          client.GET(`${base}/cash-sessions/current`, {
            params: {
              path: { establishmentId },
              query: registerId ? { register_id: registerId } : { register: MAIN_REGISTER },
            },
          }),
        );
      } catch (err) {
        if (err instanceof ApiProblem && err.status === 404 && err.type === 'not_found')
          return null;
        throw err;
      }
    },
    enabled,
    retry: shouldRetry,
  });
}

/** Session history, newest first (first page of up to 100). */
export function useSessionHistory(establishmentId: string, enabled = true) {
  const client = useApiClient();
  return useQuery<CashSession[], ApiProblem>({
    queryKey: cashKeys.history(establishmentId),
    queryFn: async () => {
      const page = await call(() =>
        client.GET(`${base}/cash-sessions`, {
          params: { path: { establishmentId }, query: { limit: 100 } },
        }),
      );
      return [...page.items].sort((a, b) => b.opened_at.localeCompare(a.opened_at));
    },
    enabled,
    retry: shouldRetry,
  });
}

export function useSession(establishmentId: string, sessionId: string | undefined) {
  const client = useApiClient();
  return useQuery<CashSessionDetail, ApiProblem>({
    queryKey: cashKeys.session(establishmentId, sessionId ?? ''),
    queryFn: () =>
      call(() =>
        client.GET(`${base}/cash-sessions/{cashSessionId}`, {
          params: { path: { establishmentId, cashSessionId: sessionId as string } },
        }),
      ),
    enabled: !!sessionId,
    retry: shouldRetry,
  });
}

function useInvalidateCash(establishmentId: string) {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: cashKeys.all(establishmentId) });
}

export function useOpenSession(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateCash(establishmentId);
  return useMutation<CashSession, ApiProblem, CashSessionOpen>({
    mutationFn: (body) =>
      call(() =>
        client.POST(`${base}/cash-sessions`, {
          params: {
            path: { establishmentId },
            header: { 'Idempotency-Key': newIdempotencyKey() },
          },
          body,
        }),
      ),
    onSettled: invalidate,
  });
}

export function useAddMovement(establishmentId: string, sessionId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateCash(establishmentId);
  return useMutation<CashMovement, ApiProblem, CashMovementInput & { idempotencyKey: string }>({
    mutationFn: ({ idempotencyKey, ...body }) =>
      call(() =>
        client.POST(`${base}/cash-sessions/{cashSessionId}/movements`, {
          params: {
            path: { establishmentId, cashSessionId: sessionId },
            header: { 'Idempotency-Key': idempotencyKey },
          },
          body,
        }),
      ),
    onSettled: invalidate,
  });
}

export function useCloseSession(
  establishmentId: string,
  session: Pick<CashSession, 'id' | 'version'>,
) {
  const client = useApiClient();
  const invalidate = useInvalidateCash(establishmentId);
  return useMutation<CashSession, ApiProblem, CashSessionClose>({
    mutationFn: (body) =>
      call(() =>
        client.POST(`${base}/cash-sessions/{cashSessionId}/close`, {
          params: {
            path: { establishmentId, cashSessionId: session.id },
            header: { 'If-Match': `"${session.version}"` },
          },
          body,
        }),
      ),
    onSettled: invalidate,
  });
}

/** Cash registers (default first, then by sort order and name), with their open session. */
export function useRegisters(establishmentId: string, includeInactive = false, enabled = true) {
  const client = useApiClient();
  return useQuery<CashRegister[], ApiProblem>({
    queryKey: cashKeys.registers(establishmentId, includeInactive),
    queryFn: async () => {
      const list = await call(() =>
        client.GET(`${base}/cash-registers`, {
          params: {
            path: { establishmentId },
            query: includeInactive ? { include_inactive: true } : {},
          },
        }),
      );
      return list.items;
    },
    enabled,
    retry: shouldRetry,
  });
}

export function useCreateRegister(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateCash(establishmentId);
  return useMutation<CashRegister, ApiProblem, CashRegisterInput>({
    mutationFn: (body) =>
      call(() =>
        client.POST(`${base}/cash-registers`, {
          params: {
            path: { establishmentId },
            header: { 'Idempotency-Key': newIdempotencyKey() },
          },
          body,
        }),
      ),
    onSettled: invalidate,
  });
}

/** Update with If-Match on the version read (412 when someone else changed it). */
export function useUpdateRegister(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateCash(establishmentId);
  return useMutation<
    CashRegister,
    ApiProblem,
    { register: Pick<CashRegister, 'id' | 'version'>; body: CashRegisterInput }
  >({
    mutationFn: ({ register, body }) =>
      call(() =>
        client.PUT(`${base}/cash-registers/{registerId}`, {
          params: {
            path: { establishmentId, registerId: register.id },
            header: { 'If-Match': `"${register.version}"` },
          },
          body,
        }),
      ),
    onSettled: invalidate,
  });
}

export function useDeleteRegister(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateCash(establishmentId);
  return useMutation<void, ApiProblem, Pick<CashRegister, 'id' | 'version'>>({
    mutationFn: async (register) => {
      await call(() =>
        client.DELETE(`${base}/cash-registers/{registerId}`, {
          params: {
            path: { establishmentId, registerId: register.id },
            header: { 'If-Match': `"${register.version}"` },
          },
        }),
      );
    },
    onSettled: invalidate,
  });
}

/** Bookings matching a code (or customer name) for the quick "incassa". */
export function useBookingSearch(establishmentId: string, q: string) {
  const client = useApiClient();
  const term = q.trim();
  return useQuery<BookingSummary[], ApiProblem>({
    queryKey: cashKeys.bookingSearch(establishmentId, term),
    queryFn: async () => {
      const page = await call(() =>
        client.GET(`${base}/bookings`, {
          params: { path: { establishmentId }, query: { q: term, limit: 10 } },
        }),
      );
      return page.items;
    },
    enabled: term.length >= 2,
    placeholderData: keepPreviousData,
    retry: shouldRetry,
  });
}

export function useRecordBookingPayment(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<
    Payment,
    ApiProblem,
    { bookingId: string; idempotencyKey: string; body: ManualPaymentInput }
  >({
    mutationFn: ({ bookingId, idempotencyKey, body }) =>
      call(() =>
        client.POST(`${base}/bookings/{bookingId}/payments`, {
          params: {
            path: { establishmentId, bookingId },
            header: { 'Idempotency-Key': idempotencyKey },
          },
          body,
        }),
      ),
    onSettled: async () => {
      await Promise.all(
        (['cash', 'bookings', 'payments', 'operations'] as const).map((area) =>
          qc.invalidateQueries({ queryKey: establishmentKeys.area(establishmentId, area) }),
        ),
      );
    },
  });
}

export function useCorrispettivi(establishmentId: string, q: CorrispettiviQuery, enabled = true) {
  const client = useApiClient();
  return useQuery<CorrispettiviReport, ApiProblem>({
    queryKey: cashKeys.corrispettivi(establishmentId, q),
    queryFn: () =>
      call(() =>
        client.GET(`${base}/fiscal/corrispettivi`, {
          params: { path: { establishmentId }, query: { ...q, format: 'json' } },
        }),
      ) as Promise<CorrispettiviReport>,
    enabled,
    placeholderData: keepPreviousData,
    retry: shouldRetry,
  });
}
