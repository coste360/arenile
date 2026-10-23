import { createApiClient } from '@arenile/shared/api';
import { memoryDatabase, type PersistedSnapshot } from '@arenile/shared/offline';
import { QueryClient } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { opBooking, opDay, opSpot } from '../features/operations/testing/fixtures';
import { createOfflineRuntime } from './runtime';

const EST = 'e1';
const API = `/api/admin/v1/establishments/${EST}`;
const DAY_KEY = [
  'est',
  EST,
  'operations',
  'day',
  { date: '2026-07-14', slot: 'FULL', mapId: null },
];
const ME = (id: string) => ({ principal_type: 'user', user: { id }, memberships: [] });

type Handler = (req: Request) => Response | Promise<Response>;

function json(status: number, body: unknown) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': status >= 400 ? 'application/problem+json' : 'application/json' },
  });
}

function setup(
  routes: Record<string, Handler> = {},
  opts: { db?: ReturnType<typeof memoryDatabase>; online?: { value: boolean } } = {},
) {
  const online = opts.online ?? { value: true };
  const fetchMock = vi.fn(async (req: Request) => {
    if (!online.value) throw new TypeError('Failed to fetch');
    const handler = routes[`${req.method} ${new URL(req.url).pathname}`];
    return handler ? handler(req) : json(404, { type: 'not_found', title: 'nf', status: 404 });
  });
  const api = createApiClient({
    baseUrl: 'http://localhost',
    fetch: fetchMock,
    getCsrfToken: () => 'c',
  });
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const db = opts.db ?? memoryDatabase('test');
  const runtime = createOfflineRuntime({
    queryClient,
    api,
    db,
    isOnline: () => online.value,
    now: () => new Date('2026-07-14T08:00:00Z'),
  });
  return { runtime, queryClient, fetchMock, db, online };
}

const day = opDay([
  opSpot('s2', 'A2', 'booked', { booking: opBooking({ balance_due: 2000, paid_amount: 1000 }) }),
]);

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('OfflineRuntime_Queue', () => {
  it('queues offline, shows the effect on the cached day and replays on reconnection', async () => {
    const online = { value: false };
    const { runtime, queryClient, fetchMock } = setup(
      {
        [`POST ${API}/bookings/b1/check-in`]: () => json(200, { id: 'b1' }),
        [`POST ${API}/bookings/b1/payments`]: () => json(201, { id: 'p1' }),
      },
      { online },
    );
    await runtime.start();
    queryClient.setQueryData(DAY_KEY, day);

    const checkIn = await runtime.enqueue({
      kind: 'check_in',
      establishmentId: EST,
      bookingId: 'b1',
      label: 'A2 · AR-0001',
      ifMatch: '"1"',
    });
    const pay = await runtime.enqueue({
      kind: 'cash_payment',
      establishmentId: EST,
      bookingId: 'b1',
      label: 'A2 · AR-0001',
      amount: 2000,
    });
    // Visible offline on the cached map.
    expect(queryClient.getQueryData<typeof day>(DAY_KEY)?.spots[0]).toMatchObject({
      status: 'checked_in',
      booking: { balance_due: 0 },
    });
    expect(await runtime.flush()).toBeUndefined();
    expect(fetchMock).not.toHaveBeenCalled();

    online.value = true;
    const result = await runtime.flush();
    expect(result).toMatchObject({ sent: 2, conflicts: 0 });
    const [first, second] = fetchMock.mock.calls.map(([r]) => r as Request);
    expect(new URL(first!.url).pathname).toBe(`${API}/bookings/b1/check-in`);
    expect(first!.headers.get('Idempotency-Key')).toBe(checkIn.idempotencyKey);
    expect(first!.headers.get('If-Match')).toBe('"1"');
    expect(new URL(second!.url).pathname).toBe(`${API}/bookings/b1/payments`);
    expect(second!.headers.get('Idempotency-Key')).toBe(pay.idempotencyKey);
    expect(await second!.json()).toEqual({ amount: 2000, method: 'cash' });
    expect(runtime.queue.getSnapshot()).toEqual([]);
    // The optimistic day is replaced by the server's.
    expect(queryClient.getQueryState(DAY_KEY)?.isInvalidated).toBe(true);
  });

  it('surfaces a conflict with the problem detail and keeps going', async () => {
    const { runtime } = setup({
      [`POST ${API}/bookings/b1/payments`]: () =>
        json(409, {
          type: 'cash_session_closed',
          title: 'Cassa chiusa',
          status: 409,
          detail: 'Nessuna sessione di cassa aperta',
        }),
      [`POST ${API}/bookings/b1/notes`]: () => json(201, { id: 'b1' }),
    });
    await runtime.start();
    const listener = vi.fn();
    runtime.onFlushed(listener);
    await runtime.queue.enqueue({
      kind: 'cash_payment',
      establishmentId: EST,
      bookingId: 'b1',
      label: 'A2',
      amount: 500,
    });
    await runtime.queue.enqueue({
      kind: 'note',
      establishmentId: EST,
      bookingId: 'b1',
      label: 'A2',
      text: 'Lettino in più',
    });
    await runtime.flush();
    expect(runtime.queue.getSnapshot()).toEqual([
      expect.objectContaining({
        kind: 'cash_payment',
        status: 'conflict',
        error: 'Nessuna sessione di cassa aperta',
      }),
    ]);
    expect(listener).toHaveBeenCalledWith(expect.objectContaining({ sent: 1, conflicts: 1 }));
  });

  it('keeps the queue across a reload (same IndexedDB)', async () => {
    const db = memoryDatabase('reload');
    const first = setup({}, { db, online: { value: false } });
    await first.runtime.start();
    await first.runtime.enqueue({
      kind: 'note',
      establishmentId: EST,
      bookingId: 'b1',
      label: 'A2',
      text: 'x',
    });
    first.runtime.stop();

    const second = setup({ [`POST ${API}/bookings/b1/notes`]: () => json(201, {}) }, { db });
    await second.runtime.start();
    await vi.waitFor(() => expect(second.fetchMock).toHaveBeenCalledTimes(1));
    await vi.waitFor(() => expect(second.runtime.queue.getSnapshot()).toEqual([]));
  });
});

describe('OfflineRuntime_Cache', () => {
  it('persists only the current establishment and restores it after a reload', async () => {
    const db = memoryDatabase('cache');
    const { runtime, queryClient } = setup({}, { db });
    await runtime.start();
    runtime.setEstablishment(EST);
    queryClient.setQueryData(['auth', 'me'], ME('u1'));
    queryClient.setQueryData(DAY_KEY, day);
    queryClient.setQueryData(['est', 'e2', 'operations', 'day', { date: '2026-07-14' }], day);
    queryClient.setQueryData(['est', EST, 'customers', 'list', {}], [{ email: 'a@b.c' }]);
    await runtime.persister.persistNow();

    const snap = await db.store<PersistedSnapshot>('cache').get('queries');
    expect(snap?.state.queries.map((q) => q.queryKey)).toEqual([['auth', 'me'], DAY_KEY]);

    const reloaded = setup({}, { db });
    reloaded.runtime.setEstablishment(EST);
    await reloaded.runtime.start();
    expect(reloaded.queryClient.getQueryData(DAY_KEY)).toEqual(day);
    expect(reloaded.queryClient.getQueryData(['auth', 'me'])).toEqual(ME('u1'));
  });

  it('drops the previous user data when another user signs in', async () => {
    const { runtime, queryClient, db } = setup({}, { online: { value: false } });
    await runtime.start();
    runtime.setEstablishment(EST);
    queryClient.setQueryData(['auth', 'me'], ME('u1'));
    await runtime.enqueue({
      kind: 'note',
      establishmentId: EST,
      bookingId: 'b1',
      label: 'A2',
      text: 'x',
    });
    await runtime.persister.persistNow();
    await vi.waitFor(async () => expect(await db.store('meta').get('owner')).toBe('u1'));

    queryClient.setQueryData(['auth', 'me'], ME('u2'));
    await vi.waitFor(() => expect(runtime.queue.getSnapshot()).toEqual([]));
    await vi.waitFor(async () => expect(await db.store('meta').get('owner')).toBe('u2'));
  });

  it('clearAll removes queue, snapshot, metadata and runtime caches (logout)', async () => {
    const deleted: string[] = [];
    vi.stubGlobal('caches', {
      keys: async () => ['workbox-precache-v2-/admin/', 'api-data'],
      delete: async (name: string) => {
        deleted.push(name);
        return true;
      },
    });
    const { runtime, queryClient, db } = setup({}, { online: { value: false } });
    await runtime.start();
    runtime.setEstablishment(EST);
    queryClient.setQueryData(['auth', 'me'], ME('u1'));
    await runtime.enqueue({
      kind: 'note',
      establishmentId: EST,
      bookingId: 'b1',
      label: 'A2',
      text: 'x',
    });
    await runtime.persister.persistNow();

    await runtime.clearAll();
    expect(runtime.queue.getSnapshot()).toEqual([]);
    expect(await db.store('queue').values()).toEqual([]);
    expect(await db.store('cache').get('queries')).toBeUndefined();
    expect(await db.store('meta').values()).toEqual([]);
    expect(deleted).toEqual(['api-data']);
  });

  it('beforeLogout sends pending operations while the session is valid', async () => {
    const { runtime, fetchMock, online } = setup(
      { [`POST ${API}/bookings/b1/notes`]: () => json(201, {}) },
      { online: { value: false } },
    );
    await runtime.start();
    await runtime.enqueue({
      kind: 'note',
      establishmentId: EST,
      bookingId: 'b1',
      label: 'A2',
      text: 'x',
    });
    online.value = true;
    await runtime.beforeLogout();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(runtime.queue.getSnapshot()).toEqual([]);
  });
});
