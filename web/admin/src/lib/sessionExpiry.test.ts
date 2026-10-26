import { ApiProblem, queryKeys } from '@arenile/shared/api';
import { QueryClient } from '@tanstack/react-query';
import type { AnyRouter } from '@tanstack/react-router';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { waitFor } from '@testing-library/react';
import { watchSessionExpiry } from './sessionExpiry';

function problem(status: number, type: ConstructorParameters<typeof ApiProblem>[0]['type']) {
  return new ApiProblem({ type, title: type, status });
}

/** Router double: navigating moves latestLocation at once, like the real history. */
function fakeRouter(pathname: string, href = pathname) {
  const resolved = new Set<(e: { toLocation: { pathname: string } }) => void>();
  const router = {
    latestLocation: { pathname, href },
    navigate: vi.fn(async ({ to }: { to: string }) => {
      router.latestLocation = { pathname: to, href: to };
      await Promise.resolve();
      resolved.forEach((fn) => fn({ toLocation: { pathname: to } }));
    }),
    subscribe: (_: 'onResolved', fn: (e: { toLocation: { pathname: string } }) => void) => {
      resolved.add(fn);
      return () => resolved.delete(fn);
    },
  };
  return { router: router as unknown as AnyRouter, navigate: router.navigate };
}

describe('watchSessionExpiry', () => {
  let stop: (() => void) | undefined;
  afterEach(() => stop?.());

  async function failMutation(qc: QueryClient, error: unknown) {
    await qc
      .getMutationCache()
      .build(qc, { mutationFn: () => Promise.reject(error) })
      .execute(undefined)
      .catch(() => undefined);
  }

  async function failQuery(qc: QueryClient, error: unknown) {
    await qc
      .fetchQuery({ queryKey: ['page'], queryFn: () => Promise.reject(error), retry: false })
      .catch(() => undefined);
  }

  it.each([
    ['query', failQuery],
    ['mutation', failMutation],
  ])('redirects to login on a 401 from a %s, keeping the page', async (_, fail) => {
    const qc = new QueryClient();
    qc.setQueryData(queryKeys.me, { principal_type: 'user' });
    const { router, navigate } = fakeRouter('/e/e1/cassa', '/e/e1/cassa?day=2026-07-13');
    stop = watchSessionExpiry(qc, router);
    await fail(qc, problem(401, 'unauthorized'));
    expect(navigate).toHaveBeenCalledWith({
      to: '/login',
      search: { redirect: '/e/e1/cassa?day=2026-07-13' },
    });
    await waitFor(() => expect(qc.getQueryData(queryKeys.me)).toBeUndefined());
  });

  it('keeps the cached user while a page blocks the navigation', async () => {
    const qc = new QueryClient();
    qc.setQueryData(queryKeys.me, { principal_type: 'user' });
    const { router, navigate } = fakeRouter('/e/e1/maps/m1');
    // Unsaved work: the navigation never reaches the login form.
    navigate.mockImplementation(() => new Promise<void>(() => undefined));
    stop = watchSessionExpiry(qc, router);
    await failMutation(qc, problem(401, 'unauthorized'));
    expect(navigate).toHaveBeenCalledTimes(1);
    expect(qc.getQueryData(queryKeys.me)).toEqual({ principal_type: 'user' });
  });

  it('navigates once when several requests fail together', async () => {
    const qc = new QueryClient();
    const { router, navigate } = fakeRouter('/configurazione');
    stop = watchSessionExpiry(qc, router);
    await Promise.all([
      failQuery(qc, problem(401, 'unauthorized')),
      failMutation(qc, problem(401, 'unauthorized')),
    ]);
    expect(navigate).toHaveBeenCalledTimes(1);
  });

  it.each([
    ['a 403', '/e/e1/cassa', problem(403, 'forbidden')],
    ['a 500', '/e/e1/cassa', problem(500, 'internal_error')],
    ['a network error', '/e/e1/cassa', problem(0, 'network_error')],
    ['a 401 on the login form', '/login', problem(401, 'unauthorized')],
    ['a wrong verification code', '/configurazione', problem(401, 'invalid_totp')],
    ['wrong credentials', '/configurazione', problem(401, 'invalid_credentials')],
  ])('ignores %s', async (_, pathname, error) => {
    const qc = new QueryClient();
    const { router, navigate } = fakeRouter(pathname);
    stop = watchSessionExpiry(qc, router);
    await failMutation(qc, error);
    await failQuery(qc, error);
    expect(navigate).not.toHaveBeenCalled();
  });
});
