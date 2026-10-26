import { ApiProblem, queryKeys } from '@arenile/shared/api';
import type { QueryClient } from '@tanstack/react-query';
import type { AnyRouter } from '@tanstack/react-router';
import { safeRedirect } from './redirect';

/**
 * Only the session problem: the other 401s (`invalid_credentials`,
 * `totp_required`, `invalid_totp`) are answers to what the user typed.
 */
function isSessionExpired(error: unknown): boolean {
  return error instanceof ApiProblem && error.status === 401 && error.type === 'unauthorized';
}

/**
 * Sends the user back to the login form as soon as any query or mutation finds
 * the session gone: the route guard only checks the session on navigation, so
 * without this an expired session shows the generic error card with a
 * "Riprova" that can never succeed. Pages with unsaved work keep it through
 * their navigation blocker. Returns the unsubscribe function.
 */
export function watchSessionExpiry(queryClient: QueryClient, router: AnyRouter): () => void {
  const onError = (error: unknown) => {
    if (!isSessionExpired(error)) return;
    // latestLocation moves to /login synchronously when the navigation starts,
    // so the other requests of the page that fail together add nothing; while a
    // page blocks the navigation (unsaved work) it stays put, and each further
    // failure asks again.
    if (router.latestLocation.pathname === '/login') return;
    void router.navigate({
      to: '/login',
      search: { redirect: safeRedirect(router.latestLocation.href) },
    });
  };
  // The cached user goes only once the login form is reached: a page that kept
  // the user (unsaved work) must not turn read-only meanwhile.
  const unsubscribeRouter = router.subscribe('onResolved', ({ toLocation }) => {
    if (toLocation.pathname === '/login') queryClient.removeQueries({ queryKey: queryKeys.me });
  });
  const unsubscribeQueries = queryClient.getQueryCache().subscribe((event) => {
    if (event.type === 'updated' && event.action.type === 'error') onError(event.action.error);
  });
  const unsubscribeMutations = queryClient.getMutationCache().subscribe((event) => {
    if (event.type === 'updated' && event.action.type === 'error') onError(event.action.error);
  });
  return () => {
    unsubscribeRouter();
    unsubscribeQueries();
    unsubscribeMutations();
  };
}
