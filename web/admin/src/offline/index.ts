// Offline mode of the panel (spec 21.2, M5A). Owner: PWA/offline builder.
// See types.ts for the decisions; implementation files to add here:
// db.ts (IndexedDB wrapper), cache.ts (map + bookings of today/tomorrow),
// queue.ts (OfflineQueue), useConnection.ts and ConnectionIndicator.tsx
// (shadcn Badge in the shell header).
export * from './types';
