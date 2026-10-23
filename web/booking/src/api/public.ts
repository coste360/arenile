import {
  call,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CHECKOUT_HEADER, checkoutStore } from '../checkout/session';

/**
 * Data hooks of the public booking flow (spec 10.1, 12.3, 12.4), typed from
 * api/openapi.yaml through openapi-fetch. Booking-app only (they never ship
 * in the admin bundle). Owner: booking app builder.
 *
 * The checkout session token is passed explicitly as `X-Arenile-Checkout`
 * (from `checkoutStore`, sessionStorage); the magic link token travels in
 * the path of the manage endpoints. Neither is ever logged.
 */

export type PublicCalendar = Schemas['PublicCalendar'];
export type PublicMapDocument = Schemas['PublicMapDocument'];
export type PublicAvailability = Schemas['PublicAvailability'];
export type PublicCatalog = Schemas['PublicCatalog'];
export type PublicSelection = Schemas['PublicSelection'];
export type PublicQuote = Schemas['PublicQuote'];
export type CheckoutSession = Schemas['CheckoutSession'];
export type CheckoutState = Schemas['CheckoutState'];
export type CheckoutCustomerInput = Schemas['CheckoutCustomerInput'];
export type PayResult = Schemas['PayResult'];
export type ManageBooking = Schemas['ManageBooking'];
export type ManageModifyRequest = Schemas['ManageModifyRequest'];
export type ManageModifyResult = Schemas['ManageModifyResult'];
export type ManageCancelRequest = Schemas['ManageCancelRequest'];
export type ManageCancelResult = Schemas['ManageCancelResult'];
export type ManageReleaseRequest = Schemas['ManageReleaseRequest'];
export type ManageReleaseResult = Schemas['ManageReleaseResult'];

export interface AvailabilityQuery {
  from: string;
  to: string;
  slots: Schemas['Slots'];
  formula?: Schemas['Formula'];
  zone_id?: string;
}

export interface CatalogQuery {
  from?: string;
  to?: string;
  slots?: Schemas['Slots'];
  formula?: Schemas['Formula'];
  zone_id?: string;
  spot_id?: string;
}

/** Query keys of the public flow, rooted at the slug. */
export const publicKeys = {
  calendar: (slug: string, month: string, slots?: string, zoneId?: string) =>
    ['public', slug, 'calendar', month, slots ?? 'FULL', zoneId ?? ''] as const,
  map: (slug: string, mapId: string) => ['public', slug, 'map', mapId] as const,
  availability: (slug: string, q: AvailabilityQuery | null) =>
    ['public', slug, 'availability', q] as const,
  catalog: (slug: string, q: CatalogQuery) => ['public', slug, 'catalog', q] as const,
  quote: (slug: string, selection: PublicSelection | null) =>
    ['public', slug, 'quote', selection] as const,
  checkout: (slug: string, bookingId: string) => ['public', slug, 'checkout', bookingId] as const,
  manage: (slug: string, token: string) => ['public', slug, 'manage', token] as const,
};

function checkoutHeader(token: string) {
  return { [CHECKOUT_HEADER]: token } as { 'X-Arenile-Checkout': string };
}

export function usePublicCalendar(
  slug: string,
  month: string,
  opts: { slots?: Schemas['Slots']; zoneId?: string; enabled?: boolean } = {},
) {
  const client = useApiClient();
  return useQuery<PublicCalendar, ApiProblem>({
    queryKey: publicKeys.calendar(slug, month, opts.slots, opts.zoneId),
    queryFn: () =>
      call(() =>
        client.GET('/api/public/v1/{slug}/calendar', {
          params: { path: { slug }, query: { month, slots: opts.slots, zone_id: opts.zoneId } },
        }),
      ),
    enabled: opts.enabled ?? true,
    retry: shouldRetry,
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  });
}

export function usePublicMap(slug: string, mapId: string | null) {
  const client = useApiClient();
  return useQuery<PublicMapDocument, ApiProblem>({
    queryKey: publicKeys.map(slug, mapId ?? ''),
    queryFn: () =>
      call(() =>
        client.GET('/api/public/v1/{slug}/maps/{mapId}', {
          params: { path: { slug, mapId: mapId as string } },
        }),
      ),
    enabled: !!mapId,
    retry: shouldRetry,
    staleTime: 5 * 60_000,
  });
}

export function usePublicAvailability(slug: string, q: AvailabilityQuery | null) {
  const client = useApiClient();
  return useQuery<PublicAvailability, ApiProblem>({
    queryKey: publicKeys.availability(slug, q),
    queryFn: () =>
      call(() =>
        client.GET('/api/public/v1/{slug}/availability', {
          params: { path: { slug }, query: q as AvailabilityQuery },
        }),
      ),
    enabled: !!q,
    retry: shouldRetry,
    staleTime: 10_000,
    placeholderData: keepPreviousData,
  });
}

export function usePublicCatalog(slug: string, q: CatalogQuery) {
  const client = useApiClient();
  return useQuery<PublicCatalog, ApiProblem>({
    queryKey: publicKeys.catalog(slug, q),
    queryFn: () =>
      call(() =>
        client.GET('/api/public/v1/{slug}/catalog', { params: { path: { slug }, query: q } }),
      ),
    retry: shouldRetry,
    staleTime: 60_000,
  });
}

/** Price summary; the caller debounces `selection` (spec 12.3 step 3). */
export function useQuote(slug: string, selection: PublicSelection | null) {
  const client = useApiClient();
  return useQuery<PublicQuote, ApiProblem>({
    queryKey: publicKeys.quote(slug, selection),
    queryFn: () =>
      call(() =>
        client.POST('/api/public/v1/{slug}/quote', {
          params: { path: { slug } },
          body: selection as PublicSelection,
        }),
      ),
    enabled: !!selection,
    retry: shouldRetry,
    placeholderData: keepPreviousData,
  });
}

/** Creates the held booking and stores the session token (sessionStorage). */
export function useCreateBooking(slug: string) {
  const client = useApiClient();
  return useMutation<
    CheckoutSession,
    ApiProblem,
    { body: Schemas['PublicBookingCreate']; idempotencyKey: string }
  >({
    mutationFn: ({ body, idempotencyKey }) =>
      call(() =>
        client.POST('/api/public/v1/{slug}/bookings', {
          params: { path: { slug }, header: { 'Idempotency-Key': idempotencyKey } },
          body,
        }),
      ),
    onSuccess: (s) => {
      checkoutStore.save(slug, {
        bookingId: s.booking_id,
        token: s.checkout_token,
        holdExpiresAt: s.hold_expires_at,
      });
    },
  });
}

/**
 * State of the checkout; `pollMs` polls after the payment return until a
 * final state (the page never confirms by itself, INV-19).
 */
export function useCheckoutState(
  slug: string,
  session: { bookingId: string; token: string } | null,
  pollMs?: number,
) {
  const client = useApiClient();
  return useQuery<CheckoutState, ApiProblem>({
    queryKey: publicKeys.checkout(slug, session?.bookingId ?? ''),
    queryFn: () =>
      call(() =>
        client.GET('/api/public/v1/{slug}/bookings/{bookingId}', {
          params: {
            path: { slug, bookingId: session!.bookingId },
            header: checkoutHeader(session!.token),
          },
        }),
      ),
    enabled: !!session,
    retry: shouldRetry,
    refetchInterval: (q) => {
      const next = q.state.data?.next_action;
      return pollMs && (next === undefined || next === 'wait') ? pollMs : false;
    },
  });
}

export function useUpdateCheckout(slug: string, session: { bookingId: string; token: string }) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<CheckoutState, ApiProblem, CheckoutCustomerInput>({
    mutationFn: (body) =>
      call(() =>
        client.PATCH('/api/public/v1/{slug}/bookings/{bookingId}', {
          params: {
            path: { slug, bookingId: session.bookingId },
            header: checkoutHeader(session.token),
          },
          body,
        }),
      ),
    onSuccess: (state) => qc.setQueryData(publicKeys.checkout(slug, session.bookingId), state),
  });
}

export function usePayCheckout(slug: string, session: { bookingId: string; token: string }) {
  const client = useApiClient();
  return useMutation<
    PayResult,
    ApiProblem,
    { body?: Schemas['CheckoutPayRequest']; idempotencyKey: string }
  >({
    mutationFn: ({ body, idempotencyKey }) =>
      call(() =>
        client.POST('/api/public/v1/{slug}/bookings/{bookingId}/pay', {
          params: {
            path: { slug, bookingId: session.bookingId },
            header: { ...checkoutHeader(session.token), 'Idempotency-Key': idempotencyKey },
          },
          body: body ?? {},
        }),
      ),
  });
}

export function useAbandonCheckout(slug: string) {
  const client = useApiClient();
  return useMutation<void, ApiProblem, { bookingId: string; token: string }>({
    mutationFn: async (session) => {
      await call(() =>
        client.DELETE('/api/public/v1/{slug}/bookings/{bookingId}', {
          params: {
            path: { slug, bookingId: session.bookingId },
            header: checkoutHeader(session.token),
          },
        }),
      );
      checkoutStore.clear(slug);
    },
  });
}

export function useManagedBooking(slug: string, token: string | null) {
  const client = useApiClient();
  return useQuery<ManageBooking, ApiProblem>({
    queryKey: publicKeys.manage(slug, token ?? ''),
    queryFn: () =>
      call(() =>
        client.GET('/api/public/v1/{slug}/manage/{token}', {
          params: { path: { slug, token: token as string } },
        }),
      ),
    enabled: !!token,
    retry: shouldRetry,
  });
}

export function useRequestManageLink(slug: string) {
  const client = useApiClient();
  return useMutation<void, ApiProblem, Schemas['ManageLinkRequest']>({
    mutationFn: async (body) => {
      await call(() =>
        client.POST('/api/public/v1/{slug}/manage-links', { params: { path: { slug } }, body }),
      );
    },
  });
}

/** Fresh Idempotency-Key for a create / pay attempt (a retry of the same attempt reuses it). */
export function newIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

export type CheckoutDocument = 'ics' | 'qr';

/**
 * Downloads the .ics or the QR (SVG) of a confirmed booking with the session
 * header (they cannot be plain links: the token never goes in a URL).
 */
export async function fetchCheckoutDocument(
  client: ReturnType<typeof useApiClient>,
  slug: string,
  session: { bookingId: string; token: string },
  doc: CheckoutDocument,
): Promise<Blob> {
  const params = {
    path: { slug, bookingId: session.bookingId },
    header: checkoutHeader(session.token),
  };
  return doc === 'ics'
    ? call(() =>
        client.GET('/api/public/v1/{slug}/bookings/{bookingId}/ics', { params, parseAs: 'blob' }),
      )
    : call(() =>
        client.GET('/api/public/v1/{slug}/bookings/{bookingId}/qr', {
          params: { ...params, query: { format: 'svg' } },
          parseAs: 'blob',
        }),
      );
}

/** Starts the payment of the balance from the manage area (redirect like the checkout). */
export function useManagePay(slug: string, token: string) {
  const client = useApiClient();
  return useMutation<PayResult, ApiProblem, { idempotencyKey: string }>({
    mutationFn: ({ idempotencyKey }) =>
      call(() =>
        client.POST('/api/public/v1/{slug}/manage/{token}/pay', {
          params: { path: { slug, token }, header: { 'Idempotency-Key': idempotencyKey } },
        }),
      ),
  });
}

function ifMatch(version: number): string {
  return `"${String(version)}"`;
}

/** Applies a manage action result to the cached booking (version moves forward). */
function useManageCache(slug: string, token: string) {
  const qc = useQueryClient();
  return (booking: ManageBooking | undefined) => {
    if (booking) qc.setQueryData(publicKeys.manage(slug, token), booking);
    else void qc.invalidateQueries({ queryKey: publicKeys.manage(slug, token) });
  };
}

/** Modify within the policy: `dry_run` (default) previews, `false` applies with If-Match. */
export function useManageModify(slug: string, token: string) {
  const client = useApiClient();
  const update = useManageCache(slug, token);
  return useMutation<
    ManageModifyResult,
    ApiProblem,
    { version: number; body: ManageModifyRequest }
  >({
    mutationFn: ({ version, body }) =>
      call(() =>
        client.POST('/api/public/v1/{slug}/manage/{token}/modify', {
          params: { path: { slug, token }, header: { 'If-Match': ifMatch(version) } },
          body,
        }),
      ),
    onSuccess: (r) => {
      if (r.applied) update(r.booking);
    },
  });
}

/** Cancellation with refund / credit preview (`dry_run`) or for real. */
export function useManageCancel(slug: string, token: string) {
  const client = useApiClient();
  const update = useManageCache(slug, token);
  return useMutation<
    ManageCancelResult,
    ApiProblem,
    { version: number; body: ManageCancelRequest }
  >({
    mutationFn: ({ version, body }) =>
      call(() =>
        client.POST('/api/public/v1/{slug}/manage/{token}/cancel', {
          params: { path: { slug, token }, header: { 'If-Match': ifMatch(version) } },
          body,
        }),
      ),
    onSuccess: (r) => {
      if (r.applied) update(r.booking);
    },
  });
}

/** Season pass holders: release single days for a credit (preview or apply). */
export function useManageRelease(slug: string, token: string) {
  const client = useApiClient();
  const update = useManageCache(slug, token);
  return useMutation<ManageReleaseResult, ApiProblem, ManageReleaseRequest>({
    mutationFn: (body) =>
      call(() =>
        client.POST('/api/public/v1/{slug}/manage/{token}/releases', {
          params: { path: { slug, token } },
          body,
        }),
      ),
    onSuccess: (r) => {
      if (r.applied) update(r.booking);
    },
  });
}

/** Public URL of a manage document (the magic link token is already in the path). */
export function manageDocumentUrl(slug: string, token: string, doc: CheckoutDocument): string {
  const base = `/api/public/v1/${encodeURIComponent(slug)}/manage/${encodeURIComponent(token)}`;
  return doc === 'ics' ? `${base}/ics` : `${base}/qr?format=svg`;
}
