import {
  establishmentKeys,
  queryKeys,
  type ApiClient,
  type EstablishmentArea,
  type EstablishmentList,
  type Me,
} from '@arenile/shared/api';
import {
  clearRuntimeCaches,
  createOperationQueue,
  createQueryPersister,
  openOfflineDatabase,
  type FlushResult,
  type NewQueuedOperation,
  type OfflineDatabase,
  type PersistedSnapshot,
  type QueryPersister,
} from '@arenile/shared/offline';
import type { QueryClient } from '@tanstack/react-query';
import { createPersistPolicy, OFFLINE_MAX_AGE_MS } from './cache';
import { applyOptimistic } from './optimistic';
import { sendOfflineOperation } from './senders';
import type { OfflineOperation, OfflineQueue } from './types';

/*
 * Composition of the panel's offline mode: one IndexedDB database with the
 * operation queue, the query snapshot and a little metadata, wired to the
 * TanStack Query client and the API client. Created once in main.tsx
 * (restored before the first render so route guards see the cached user);
 * tests create it over memory stores.
 */

export const OFFLINE_DB_NAME = 'arenile-admin-offline';
const STORES = ['queue', 'cache', 'meta'] as const;
/** Bump when the persisted query shapes change incompatibly. */
export const OFFLINE_CACHE_VERSION = 'admin-1';

/** Areas refreshed after a queued operation was applied or refused. */
const AFFECTED_AREAS: readonly EstablishmentArea[] = [
  'operations',
  'planning',
  'availability',
  'bookings',
  'season-passes',
  'payments',
  'cash',
];

export interface OfflineRuntime {
  readonly queue: OfflineQueue;
  readonly persister: QueryPersister;
  /** False when IndexedDB is unavailable (data kept in memory only). */
  readonly persistent: boolean;
  /** Restores the snapshot, loads the queue and starts saving. */
  start(): Promise<void>;
  stop(): void;
  /** Establishment in the URL: the only one whose data is kept. */
  setEstablishment(id: string | undefined): void;
  /** Queues an operation and shows its effect on the cached day. */
  enqueue(input: NewQueuedOperation<OfflineOperation>): Promise<OfflineOperation>;
  /** Sends the queue (no-op while offline). */
  flush(): Promise<FlushResult | undefined>;
  /** Called after every flush that sent or refused something (toasts). */
  onFlushed(listener: (result: FlushResult) => void): () => void;
  /** Before logout: tries to send what is pending while the session is valid. */
  beforeLogout(timeoutMs?: number): Promise<void>;
  /** Removes every offline datum of this browser (logout). */
  clearAll(): Promise<void>;
}

export interface OfflineRuntimeOptions {
  queryClient: QueryClient;
  api: ApiClient;
  db?: OfflineDatabase;
  isOnline?: () => boolean;
  now?: () => Date;
}

function userIdOf(me: Me | undefined): string | undefined {
  return me?.user?.id ?? me?.api_key?.id;
}

export function createOfflineRuntime(options: OfflineRuntimeOptions): OfflineRuntime {
  const { queryClient, api } = options;
  const db = options.db ?? openOfflineDatabase(OFFLINE_DB_NAME, STORES);
  const isOnline = options.isOnline ?? (() => navigator.onLine);
  const now = options.now ?? (() => new Date());
  const meta = db.store<string>('meta');
  let establishmentId: string | undefined;
  let unsubscribers: (() => void)[] = [];

  const invalidate = (estId: string) => {
    for (const area of AFFECTED_AREAS) {
      void queryClient.invalidateQueries({ queryKey: establishmentKeys.area(estId, area) });
    }
  };

  const queue = createOperationQueue<OfflineOperation>({
    store: db.store<OfflineOperation>('queue'),
    send: (op) => sendOfflineOperation(api, op),
    now,
    lockName: `${OFFLINE_DB_NAME}:flush`,
    onSent: (op) => invalidate(op.establishmentId),
    // The optimistic change is wrong: reload the real state.
    onRejected: (op) => invalidate(op.establishmentId),
  });

  const timezoneOf = (estId: string) => {
    const lists = queryClient.getQueriesData<EstablishmentList>({ queryKey: ['establishments'] });
    for (const [, list] of lists) {
      const found = list?.items.find((e) => e.id === estId);
      if (found) return found.timezone;
    }
    return undefined;
  };

  // The snapshot is restored before the router knows the establishment:
  // during the restore the policy accepts the (single) establishment it holds.
  let restoring = false;
  const persister = createQueryPersister({
    queryClient,
    store: db.store<PersistedSnapshot>('cache'),
    shouldPersist: createPersistPolicy({
      currentEstablishmentId: () => (restoring ? '*' : establishmentId),
      timezoneOf,
      now,
    }),
    maxAge: OFFLINE_MAX_AGE_MS,
    buster: OFFLINE_CACHE_VERSION,
    now: () => now().getTime(),
  });

  const flushListeners = new Set<(result: FlushResult) => void>();
  const flush = async () => {
    if (!isOnline()) return undefined;
    const result = await queue.flush();
    if (result.sent + result.conflicts + result.failed > 0) {
      for (const l of flushListeners) l(result);
    }
    return result;
  };

  /**
   * A different user signed in on this browser: drop the previous one's
   * snapshot (personal data of the day). The operation queue is kept: it
   * holds cash really collected and check-ins really made at the desk
   * (booking ids, labels and amounts, no personal data), and dropping it
   * silently would leave the cash ledger short. The new session sends it;
   * the indicator shows the pending count until then.
   */
  const onUser = async (me: Me | undefined) => {
    const id = userIdOf(me);
    if (!id) return;
    const owner = await meta.get('owner');
    if (owner && owner !== id) await persister.clear();
    if (owner !== id) await meta.set('owner', id);
    if (queue.isPaused()) {
      queue.resume();
      void flush();
    }
  };

  const clearAll = async () => {
    await queue.clear();
    await persister.clear().catch(() => undefined);
    await meta.clear().catch(() => undefined);
    await clearRuntimeCaches().catch(() => []);
  };

  return {
    queue,
    persister,
    persistent: db.persistent,
    async start() {
      restoring = true;
      const savedAt = await persister.restore().catch(() => undefined);
      restoring = false;
      // The restored user is the owner of what was restored.
      if (savedAt !== undefined) await onUser(queryClient.getQueryData<Me>(queryKeys.me));
      await queue.init().catch(() => undefined);
      unsubscribers.push(persister.start());
      // The throttled save must not lose the last second before a reload or
      // a tab close (e.g. a map saved and the page reloaded at once).
      if (typeof window !== 'undefined') {
        const onHide = () => void persister.persistNow().catch(() => undefined);
        window.addEventListener('pagehide', onHide);
        unsubscribers.push(() => window.removeEventListener('pagehide', onHide));
      }
      unsubscribers.push(
        queryClient.getQueryCache().subscribe((event) => {
          if (
            event.type === 'updated' &&
            event.action.type === 'success' &&
            event.query.queryKey[0] === queryKeys.me[0] &&
            event.query.queryKey[1] === queryKeys.me[1]
          ) {
            void onUser(event.query.state.data as Me | undefined);
          }
        }),
      );
      void flush();
    },
    stop() {
      for (const u of unsubscribers) u();
      unsubscribers = [];
    },
    setEstablishment(id) {
      if (id === establishmentId) return;
      establishmentId = id;
      // Rewrites the snapshot without the previous establishment's data.
      void persister.persistNow().catch(() => undefined);
    },
    async enqueue(input) {
      // A check-in queued behind other operations on the same booking
      // cannot know the version the server will have when its turn comes
      // (an earlier one may be refused): the state transition guards it
      // (confirmed -> checked_in, 409 otherwise), so If-Match is dropped.
      if (input.kind === 'check_in' && input.ifMatch) {
        const { ifMatch, ...rest } = input;
        const queued = queue.getSnapshot().some((o) => o.bookingId === input.bookingId);
        input = queued ? rest : { ...rest, ifMatch };
      }
      const op = await queue.enqueue(input);
      applyOptimistic(queryClient, op);
      // Lie-fi: the browser says online but the request failed; try again now.
      void flush();
      return op;
    },
    flush,
    onFlushed(listener) {
      flushListeners.add(listener);
      return () => flushListeners.delete(listener);
    },
    async beforeLogout(timeoutMs = 5000) {
      if (!isOnline() || !queue.getSnapshot().some((o) => o.status === 'pending')) return;
      await Promise.race([
        flush().catch(() => undefined),
        new Promise((r) => setTimeout(r, timeoutMs)),
      ]);
    },
    clearAll,
  };
}
