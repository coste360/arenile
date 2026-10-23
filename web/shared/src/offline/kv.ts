/**
 * Minimal asynchronous key-value port used by the offline cache and the
 * operation queue. Production code uses the IndexedDB implementation
 * (idb.ts); tests and browsers without IndexedDB use the in-memory one.
 *
 * Values must be structured-cloneable (plain JSON-like data): IndexedDB
 * stores a structured clone, and the memory store clones too, so both
 * behave the same (no shared references between writer and reader).
 */
export interface KeyValueStore<T> {
  get(key: string): Promise<T | undefined>;
  set(key: string, value: T): Promise<void>;
  delete(key: string): Promise<void>;
  /** Every value, in key order. */
  values(): Promise<T[]>;
  clear(): Promise<void>;
}

function clone<T>(value: T): T {
  return typeof structuredClone === 'function'
    ? structuredClone(value)
    : (JSON.parse(JSON.stringify(value)) as T);
}

/**
 * In-memory store with the same copy semantics as IndexedDB. Pass the same
 * `backing` map to two stores to simulate a reload over persisted data.
 */
export function createMemoryStore<T>(backing: Map<string, T> = new Map()): KeyValueStore<T> {
  return {
    async get(key) {
      const v = backing.get(key);
      return v === undefined ? undefined : clone(v);
    },
    async set(key, value) {
      backing.set(key, clone(value));
    },
    async delete(key) {
      backing.delete(key);
    },
    async values() {
      return [...backing.entries()]
        .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
        .map(([, v]) => clone(v));
    },
    async clear() {
      backing.clear();
    },
  };
}
