import type { KitAssetId } from './generated/manifest';

/**
 * Lazy loader of the kit symbol bodies (generated/symbols.ts, ~10 KB gzip):
 * the module is a separate chunk fetched the first time a map renders, so
 * pages without a map (and the first screen of the booking page) never pay
 * for it. The promise is shared by every map on the page.
 */
export type KitSymbolBodies = Readonly<Record<KitAssetId, string>>;

let loaded: KitSymbolBodies | null = null;
let pending: Promise<KitSymbolBodies> | null = null;

export function loadKitSymbols(): Promise<KitSymbolBodies> {
  if (loaded) return Promise.resolve(loaded);
  pending ??= import('./generated/symbols').then((m) => {
    loaded = m.KIT_SYMBOLS;
    return loaded;
  });
  return pending;
}

/** Bodies already loaded (synchronous), or null. */
export function loadedKitSymbols(): KitSymbolBodies | null {
  return loaded;
}
