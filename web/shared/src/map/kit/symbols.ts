import type { KitAssetId, MapKitKey } from './generated/manifest';

/**
 * Lazy loader of the kit symbol bodies (generated/symbols-<kit>.ts, ~10 KB
 * gzip each): every kit is a separate chunk fetched the first time a map
 * draws with it, so pages without a map (and the first screen of the booking
 * page) never pay for it, and an establishment pays only for its own kit.
 * The promise is shared by every map on the page.
 */
export type KitSymbolBodies = Readonly<Record<KitAssetId, string>>;

// Literal imports, one per kit, so the bundler can split each into its own chunk.
const IMPORTERS: Record<MapKitKey, () => Promise<{ KIT_SYMBOLS: KitSymbolBodies }>> = {
  modern: () => import('./generated/symbols-modern'),
  classic: () => import('./generated/symbols-classic'),
};

const loaded = new Map<MapKitKey, KitSymbolBodies>();
const pending = new Map<MapKitKey, Promise<KitSymbolBodies>>();

export function loadKitSymbols(kit: MapKitKey): Promise<KitSymbolBodies> {
  const done = loaded.get(kit);
  if (done) return Promise.resolve(done);
  let p = pending.get(kit);
  if (!p) {
    p = IMPORTERS[kit]().then(
      (m) => {
        loaded.set(kit, m.KIT_SYMBOLS);
        return m.KIT_SYMBOLS;
      },
      (err: unknown) => {
        // A failed chunk (flaky connection) is not cached: the next map retries.
        pending.delete(kit);
        throw err;
      },
    );
    pending.set(kit, p);
  }
  return p;
}

/** Bodies of a kit already loaded (synchronous), or null. */
export function loadedKitSymbols(kit: MapKitKey): KitSymbolBodies | null {
  return loaded.get(kit) ?? null;
}
