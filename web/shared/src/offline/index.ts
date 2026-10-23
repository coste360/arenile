/**
 * Offline building blocks shared by the apps (spec 21.2, ADR 0016).
 *
 * - kv.ts       KeyValueStore port + in-memory implementation (tests, fallback).
 * - idb.ts      Promise wrapper over native IndexedDB (no dependency); degrades
 *               to memory when IndexedDB is missing or refuses to open.
 * - persist.ts  Allowlisted TanStack Query persistence (dehydrate/hydrate),
 *               with max age and build buster.
 * - queue.ts    Persistent FIFO of operations with idempotency keys, ordered
 *               replay and conflict classification.
 * - caches.ts   Cache Storage cleanup on logout (keeps the SW precache).
 *
 * What may be stored and which operations may be queued is decided by the
 * app (admin/src/offline), not here: this module is policy-free.
 */
export * from './kv';
export * from './idb';
export * from './persist';
export * from './queue';
export * from './caches';
