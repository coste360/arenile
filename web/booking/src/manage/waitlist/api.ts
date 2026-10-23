import {
  call,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { CheckoutSession } from '../../api/public';
import { checkoutStore } from '../../checkout/session';

/**
 * Hooks of the personal waitlist link (spec 10.5, M5B): the entry and its
 * offer, accept (a held booking with a checkout session, continued in the
 * normal checkout), decline, leave. The token travels only in the path.
 */
export type PublicWaitlistEntry = Schemas['PublicWaitlistEntry'];

const key = (slug: string, token: string) => ['public', slug, 'waitlist', token] as const;

export function useWaitlistEntry(slug: string, token: string | null) {
  const client = useApiClient();
  return useQuery<PublicWaitlistEntry, ApiProblem>({
    queryKey: key(slug, token ?? ''),
    queryFn: () =>
      call(() =>
        client.GET('/api/public/v1/{slug}/waitlist/{waitlistToken}', {
          params: { path: { slug, waitlistToken: token as string } },
        }),
      ),
    enabled: !!token,
    retry: shouldRetry,
  });
}

/**
 * Accepts the pending offer: stores the checkout session like a new booking
 * (sessionStorage, `X-Arenile-Checkout`), so the booking page resumes at the
 * customer step of the held booking.
 */
export function useAcceptOffer(slug: string, token: string) {
  const client = useApiClient();
  return useMutation<CheckoutSession, ApiProblem, { idempotencyKey: string }>({
    mutationFn: ({ idempotencyKey }) =>
      call(() =>
        client.POST('/api/public/v1/{slug}/waitlist/{waitlistToken}/accept', {
          params: {
            path: { slug, waitlistToken: token },
            header: { 'Idempotency-Key': idempotencyKey },
          },
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

export function useDeclineOffer(slug: string, token: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<PublicWaitlistEntry, ApiProblem, { leave: boolean }>({
    mutationFn: (body) =>
      call(() =>
        client.POST('/api/public/v1/{slug}/waitlist/{waitlistToken}/decline', {
          params: { path: { slug, waitlistToken: token } },
          body,
        }),
      ),
    onSuccess: (entry) => qc.setQueryData(key(slug, token), entry),
  });
}

export function useLeaveWaitlist(slug: string, token: string) {
  const client = useApiClient();
  return useMutation<void, ApiProblem, void>({
    mutationFn: async () => {
      await call(() =>
        client.DELETE('/api/public/v1/{slug}/waitlist/{waitlistToken}', {
          params: { path: { slug, waitlistToken: token } },
        }),
      );
    },
  });
}

export function useRefreshWaitlist(slug: string, token: string) {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: key(slug, token) });
}
