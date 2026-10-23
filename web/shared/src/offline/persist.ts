import {
  dehydrate,
  hydrate,
  type DehydratedState,
  type Query,
  type QueryClient,
  type QueryKey,
} from '@tanstack/react-query';
import type { KeyValueStore } from './kv';

/*
 * Persistence of selected TanStack Query data to IndexedDB, so that the
 * panel can show the last known data when it starts offline (spec 21.2).
 * Hand-written on dehydrate/hydrate (both in @tanstack/react-query) instead
 * of the persist-client packages: no new dependency, and the allowlist
 * policy is the point of the module.
 *
 * - Only successful queries accepted by `shouldPersist` are written; the
 *   policy is applied again at restore, so data that left the window (e.g.
 *   yesterday's bookings) is dropped even if it was saved.
 * - The snapshot carries a `buster` (app build) and its save time; it is
 *   discarded when older than `maxAge` or written by another build.
 * - Writes are throttled; `persistNow()` forces one (tests, page hide).
 */

export interface PersistedSnapshot {
  buster: string;
  /** Epoch milliseconds of the save. */
  savedAt: number;
  state: DehydratedState;
}

export interface QueryPersisterOptions {
  queryClient: QueryClient;
  store: KeyValueStore<PersistedSnapshot>;
  /** Allowlist: true for the queries that may be stored on the device. */
  shouldPersist: (query: Pick<Query, 'queryKey' | 'state'>) => boolean;
  /** Snapshots older than this are discarded (milliseconds). */
  maxAge: number;
  /** Changes when the data shape may change (app version). */
  buster: string;
  throttleMs?: number;
  now?: () => number;
}

export interface QueryPersister {
  /** Hydrates the client from the stored snapshot; returns its save time, if any. */
  restore(): Promise<number | undefined>;
  /** Starts saving on cache changes; returns the unsubscribe function. */
  start(): () => void;
  persistNow(): Promise<void>;
  /** Deletes the stored snapshot (logout, user or establishment change). */
  clear(): Promise<void>;
  /** Save time of the last snapshot written or restored. */
  lastSavedAt(): number | undefined;
}

const SNAPSHOT_KEY = 'queries';

export function createQueryPersister(options: QueryPersisterOptions): QueryPersister {
  const { queryClient, store, shouldPersist, maxAge, buster } = options;
  const throttleMs = options.throttleMs ?? 1000;
  const now = options.now ?? Date.now;
  let timer: ReturnType<typeof setTimeout> | null = null;
  let savedAt: number | undefined;
  let writing: Promise<void> = Promise.resolve();

  const accept = (q: Pick<Query, 'queryKey' | 'state'>) =>
    q.state.status === 'success' && q.state.data !== undefined && shouldPersist(q);

  const persistNow = () => {
    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
    // Serialise the writes so that an older snapshot never lands last.
    writing = writing
      .catch(() => undefined)
      .then(async () => {
        const state = dehydrate(queryClient, { shouldDehydrateQuery: accept });
        const at = now();
        await store.set(SNAPSHOT_KEY, { buster, savedAt: at, state });
        savedAt = at;
      });
    return writing;
  };

  return {
    async restore() {
      let snap: PersistedSnapshot | undefined;
      try {
        snap = await store.get(SNAPSHOT_KEY);
      } catch {
        return undefined;
      }
      if (!snap) return undefined;
      if (snap.buster !== buster || now() - snap.savedAt > maxAge) {
        await store.delete(SNAPSHOT_KEY).catch(() => undefined);
        return undefined;
      }
      const queries = snap.state.queries.filter((q) =>
        accept({ queryKey: q.queryKey as QueryKey, state: q.state as Query['state'] }),
      );
      hydrate(queryClient, { mutations: [], queries });
      savedAt = snap.savedAt;
      return snap.savedAt;
    },
    start() {
      const unsubscribe = queryClient.getQueryCache().subscribe((event) => {
        const relevant =
          (event.type === 'updated' && event.action.type === 'success') || event.type === 'removed';
        if (!relevant || timer) return;
        timer = setTimeout(() => void persistNow().catch(() => undefined), throttleMs);
      });
      return () => {
        unsubscribe();
        if (timer) clearTimeout(timer);
        timer = null;
      };
    },
    persistNow,
    async clear() {
      if (timer) clearTimeout(timer);
      timer = null;
      await writing.catch(() => undefined);
      savedAt = undefined;
      await store.delete(SNAPSHOT_KEY);
    },
    lastSavedAt: () => savedAt,
  };
}
