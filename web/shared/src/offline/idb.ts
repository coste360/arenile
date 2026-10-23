import { createMemoryStore, type KeyValueStore } from './kv';

/*
 * Small promise wrapper over the native IndexedDB API (ADR 0014: no new
 * dependency). One database per app, out-of-line string keys, one object
 * store per concern ("queue", "cache", "meta"). Opening is lazy and shared;
 * a failure to open (private mode, blocked storage) degrades to memory
 * stores so the panel keeps working online.
 */

export interface OfflineDatabase {
  readonly name: string;
  /** Store bound to one object store of the database. */
  store<T>(storeName: string): KeyValueStore<T>;
  /** Closes the connection and deletes the database (logout). */
  destroy(): Promise<void>;
  /** True when backed by IndexedDB, false when degraded to memory. */
  readonly persistent: boolean;
}

/** True when the browser exposes IndexedDB (false in jsdom and some private modes). */
export function isIndexedDbAvailable(): boolean {
  return typeof indexedDB !== 'undefined' && indexedDB !== null;
}

function promisify<T>(req: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error ?? new Error('IndexedDB request failed'));
  });
}

function txDone(tx: IDBTransaction): Promise<void> {
  return new Promise((resolve, reject) => {
    tx.oncomplete = () => resolve();
    tx.onabort = () => reject(tx.error ?? new Error('IndexedDB transaction aborted'));
    tx.onerror = () => reject(tx.error ?? new Error('IndexedDB transaction failed'));
  });
}

function open(name: string, version: number, stores: readonly string[]): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(name, version);
    req.onupgradeneeded = () => {
      const db = req.result;
      for (const s of stores) if (!db.objectStoreNames.contains(s)) db.createObjectStore(s);
    };
    req.onsuccess = () => {
      const db = req.result;
      // Another tab upgrading or deleting the database: let it proceed.
      db.onversionchange = () => db.close();
      resolve(db);
    };
    req.onerror = () => reject(req.error ?? new Error('IndexedDB open failed'));
    req.onblocked = () => reject(new Error('IndexedDB open blocked'));
  });
}

function idbStore<T>(getDb: () => Promise<IDBDatabase>, storeName: string): KeyValueStore<T> {
  const run = async <R>(
    mode: IDBTransactionMode,
    fn: (s: IDBObjectStore) => IDBRequest<R>,
  ): Promise<R> => {
    const db = await getDb();
    const tx = db.transaction(storeName, mode);
    // The value is final only once the transaction commits.
    const [result] = await Promise.all([promisify(fn(tx.objectStore(storeName))), txDone(tx)]);
    return result;
  };
  return {
    get: (key) => run('readonly', (s) => s.get(key) as IDBRequest<T | undefined>),
    set: async (key, value) => {
      await run('readwrite', (s) => s.put(value, key));
    },
    delete: async (key) => {
      await run('readwrite', (s) => s.delete(key));
    },
    values: () => run('readonly', (s) => s.getAll() as IDBRequest<T[]>),
    clear: async () => {
      await run('readwrite', (s) => s.clear());
    },
  };
}

/**
 * Opens (lazily) the offline database `name` with the given object stores.
 * Without IndexedDB, or when opening fails, every store is in memory.
 */
export function openOfflineDatabase(
  name: string,
  stores: readonly string[],
  version = 1,
): OfflineDatabase {
  if (!isIndexedDbAvailable()) return memoryDatabase(name);
  let conn: Promise<IDBDatabase> | null = null;
  const getDb = () => {
    conn ??= open(name, version, stores).catch((err: unknown) => {
      conn = null;
      throw err;
    });
    return conn;
  };
  const cache = new Map<string, KeyValueStore<unknown>>();
  const fallback = memoryDatabase(name);
  return {
    name,
    persistent: true,
    store<T>(storeName: string): KeyValueStore<T> {
      let s = cache.get(storeName) as KeyValueStore<T> | undefined;
      if (!s) {
        s = withFallback(idbStore<T>(getDb, storeName), fallback.store<T>(storeName));
        cache.set(storeName, s as KeyValueStore<unknown>);
      }
      return s;
    },
    async destroy() {
      const pending = conn;
      conn = null;
      if (pending) {
        try {
          (await pending).close();
        } catch {
          // Never opened: nothing to close.
        }
      }
      await fallback.destroy();
      await new Promise<void>((resolve) => {
        const req = indexedDB.deleteDatabase(name);
        req.onsuccess = req.onerror = req.onblocked = () => resolve();
      });
    },
  };
}

/** Uses `primary` until it fails once, then `fallback` for the rest of the session. */
function withFallback<T>(primary: KeyValueStore<T>, fallback: KeyValueStore<T>): KeyValueStore<T> {
  let degraded = false;
  const pick = <K extends keyof KeyValueStore<T>>(method: K) =>
    (async (...args: unknown[]) => {
      if (!degraded) {
        try {
          return await (primary[method] as (...a: unknown[]) => Promise<unknown>)(...args);
        } catch {
          degraded = true;
        }
      }
      return (fallback[method] as (...a: unknown[]) => Promise<unknown>)(...args);
    }) as KeyValueStore<T>[K];
  return {
    get: pick('get'),
    set: pick('set'),
    delete: pick('delete'),
    values: pick('values'),
    clear: pick('clear'),
  };
}

/** Database whose stores live in memory (tests, no IndexedDB). */
export function memoryDatabase(name: string): OfflineDatabase {
  const stores = new Map<string, Map<string, unknown>>();
  const backing = (s: string) => {
    let m = stores.get(s);
    if (!m) stores.set(s, (m = new Map()));
    return m;
  };
  return {
    name,
    persistent: false,
    store: <T>(storeName: string) => createMemoryStore<T>(backing(storeName) as Map<string, T>),
    async destroy() {
      for (const m of stores.values()) m.clear();
    },
  };
}
