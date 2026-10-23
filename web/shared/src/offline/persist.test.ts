import { QueryClient } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { openOfflineDatabase } from './idb';
import { createMemoryStore } from './kv';
import { createQueryPersister, type PersistedSnapshot } from './persist';

const HOUR = 3_600_000;

function setup(backing = new Map<string, PersistedSnapshot>(), now = () => 1_000 * HOUR) {
  const queryClient = new QueryClient();
  const persister = createQueryPersister({
    queryClient,
    store: createMemoryStore(backing),
    shouldPersist: (q) => q.queryKey[0] === 'keep',
    maxAge: 48 * HOUR,
    buster: 'v1',
    throttleMs: 10,
    now,
  });
  return { queryClient, persister, backing };
}

afterEach(() => {
  vi.useRealTimers();
});

describe('QueryPersister_Policy', () => {
  it('stores only the allowlisted successful queries and restores them', async () => {
    const { queryClient, persister, backing } = setup();
    queryClient.setQueryData(['keep', 'day'], { spots: 3 });
    queryClient.setQueryData(['secret', 'customers'], [{ email: 'x@example.com' }]);
    await persister.persistNow();

    const keys = backing.get('queries')?.state.queries.map((q) => q.queryKey);
    expect(keys).toEqual([['keep', 'day']]);

    const reloaded = setup(backing);
    expect(await reloaded.persister.restore()).toBe(1_000 * HOUR);
    expect(reloaded.queryClient.getQueryData(['keep', 'day'])).toEqual({ spots: 3 });
    expect(reloaded.queryClient.getQueryData(['secret', 'customers'])).toBeUndefined();
  });

  it('restores entries as stale so that online pages refetch them', async () => {
    const { queryClient, persister, backing } = setup();
    queryClient.setQueryData(['keep', 'map'], 'copy');
    await persister.persistNow();

    const reloaded = setup(backing);
    await reloaded.persister.restore();
    const q = reloaded.queryClient.getQueryCache().find({ queryKey: ['keep', 'map'] });
    expect(q?.state.data).toBe('copy');
    expect(q?.state.dataUpdatedAt).toBe(0);
    expect(q?.isStaleByTime(5 * 60_000)).toBe(true);
  });

  it('applies the policy again at restore', async () => {
    const backing = new Map<string, PersistedSnapshot>();
    const { queryClient, persister } = setup(backing);
    queryClient.setQueryData(['keep', 'old'], 1);
    await persister.persistNow();

    // Next day: the same entry is now outside the window.
    const qc = new QueryClient();
    const reloaded = createQueryPersister({
      queryClient: qc,
      store: createMemoryStore(backing),
      shouldPersist: (q) => q.queryKey[1] !== 'old',
      maxAge: 48 * HOUR,
      buster: 'v1',
      now: () => 1_000 * HOUR,
    });
    await reloaded.restore();
    expect(qc.getQueryData(['keep', 'old'])).toBeUndefined();
  });

  it('drops snapshots older than maxAge or from another build', async () => {
    const backing = new Map<string, PersistedSnapshot>();
    const { queryClient, persister } = setup(backing, () => 0);
    queryClient.setQueryData(['keep', 'x'], 1);
    await persister.persistNow();

    const late = setup(new Map(backing), () => 49 * HOUR);
    expect(await late.persister.restore()).toBeUndefined();
    expect(late.queryClient.getQueryData(['keep', 'x'])).toBeUndefined();

    const otherBuild = createQueryPersister({
      queryClient: new QueryClient(),
      store: createMemoryStore(backing),
      shouldPersist: () => true,
      maxAge: 48 * HOUR,
      buster: 'v2',
      now: () => 1,
    });
    expect(await otherBuild.restore()).toBeUndefined();
    expect(backing.size).toBe(0);
  });

  it('saves on cache changes, throttled', async () => {
    const { queryClient, persister, backing } = setup();
    const stop = persister.start();
    queryClient.setQueryData(['keep', 'a'], 1);
    queryClient.setQueryData(['keep', 'b'], 2);
    await vi.waitFor(() => expect(backing.get('queries')?.state.queries).toHaveLength(2));
    stop();
  });

  it('clear deletes the snapshot', async () => {
    const { queryClient, persister, backing } = setup();
    queryClient.setQueryData(['keep', 'a'], 1);
    await persister.persistNow();
    await persister.clear();
    expect(backing.size).toBe(0);
    expect(persister.lastSavedAt()).toBeUndefined();
  });
});

describe('openOfflineDatabase', () => {
  it('degrades to memory stores without IndexedDB (jsdom)', async () => {
    const db = openOfflineDatabase('test', ['queue']);
    expect(db.persistent).toBe(false);
    const s = db.store<number>('queue');
    await s.set('a', 1);
    expect(await s.get('a')).toBe(1);
    await db.destroy();
    expect(await s.values()).toEqual([]);
  });
});
