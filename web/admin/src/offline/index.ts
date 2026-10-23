/**
 * Offline mode of the panel (spec 21.2, ADR 0014).
 *
 * - types.ts            contract: operation kinds, queue type, connection state.
 * - cache.ts            what may be stored on the device (allowlist, 48 h).
 * - runtime.ts          composition: IndexedDB, queue, query persister, user change.
 * - bootstrap.ts        creation + restore before the first render (main.tsx).
 * - senders.ts          replay of each operation with its Idempotency-Key.
 * - optimistic.ts       effect of a queued operation on the cached operations day.
 * - prefetch.ts         warm-up of maps and today/tomorrow (lazy chunk).
 * - OfflineProvider     context, replay on reconnect, retry timer, toasts.
 * - OfflineShell        establishment tracking, warm-up, banners (in AppShell).
 * - ConnectionIndicator status badge in the top bar (via components/OfflineIndicator).
 * - OfflineQueueSheet   queued/refused operations: retry, discard.
 * - OnlineOnlyNotice    explanation in the booking creation/move forms.
 * - useQueueableCheckIn check-in that is queued when offline.
 * - OfflineSpotActions  note and cash payment from the spot card while offline.
 * - useOfflineLogout    logout: send the queue, then wipe the device copy.
 *
 * Generic pieces (IndexedDB wrapper, persistent queue, persister) are in
 * @arenile/shared/offline. Feature code imports the specific files, not
 * this barrel, to keep the lazy chunks small.
 */
export * from './types';
