import { memo, useEffect, useLayoutEffect, useMemo, useState, useSyncExternalStore } from 'react';
import { createPortal } from 'react-dom';
import { KIT_VIEWBOX, MAP_KIT_KEYS, type MapKitKey } from './generated/manifest';
import { kitSymbolId } from './assets';
import { useMapKit } from './context';
import { kitIdsSnapshot, registerKitIds, subscribeKitIds, unregisterKitIds } from './registry';
import { badgeSymbolId, type BadgeGlyph } from './status';
import { loadKitSymbols, loadedKitSymbols, type KitSymbolBodies } from './symbols';

export interface MapKitDefsProps {
  /** Kit asset ids drawn by the caller (memoise the array: it is compared by content). */
  ids: readonly string[];
  /** Kit the ids are drawn with (default: the kit of the nearest MapKitProvider). */
  kit?: MapKitKey;
}

/** Paths of the status badge glyphs, on a 24-unit grid, drawn with currentColor. */
const BADGE_GLYPHS: Record<BadgeGlyph, string> = {
  globe:
    'M12 4.5a7.5 7.5 0 1 0 0 15 7.5 7.5 0 1 0 0-15M4.5 12h15M12 4.5c-3 2.6-3 12.4 0 15M12 4.5c3 2.6 3 12.4 0 15',
  desk: 'M4.5 10.5h15M4.5 10.5l2.5-5h10l2.5 5M6 10.5V19h12v-8.5M9.5 15h5',
  check: 'M5.5 12.5l4.2 4.2L18.5 8',
  star: 'M12 4.2l2.4 4.9 5.4.8-3.9 3.8.9 5.4-4.8-2.6-4.8 2.6.9-5.4-3.9-3.8 5.4-.8z',
  released: 'M7.5 16.5l9-9M9.5 7.5h7v7',
  clock: 'M12 4.5a7.5 7.5 0 1 0 0 15 7.5 7.5 0 1 0 0-15M12 8.5v4l2.8 1.8',
  cross: 'M7.5 7.5l9 9M16.5 7.5l-9 9',
  euro: 'M16.5 7.6a5.6 5.6 0 1 0 0 8.8M5.5 10.6h8M5.5 13.4h7',
  renew: 'M18.5 12a6.5 6.5 0 1 1-1.9-4.6M18.5 5v3.8h-3.8',
};

const KitSymbol = memo(function KitSymbol({
  kit,
  id,
  body,
}: {
  kit: MapKitKey;
  id: string;
  body: string;
}) {
  return (
    <symbol
      id={kitSymbolId(kit, id)}
      viewBox={KIT_VIEWBOX}
      // Bodies are generated at build time from the Coste360 kit and
      // sanitised by web/scripts/gen-map-kit.mjs (allowlisted elements and
      // attributes only, no scripts, handlers or external references).
      dangerouslySetInnerHTML={{ __html: body }}
    />
  );
});

const BadgeSymbols = memo(function BadgeSymbols() {
  return (
    <>
      {(Object.keys(BADGE_GLYPHS) as BadgeGlyph[]).map((g) => (
        <symbol key={g} id={badgeSymbolId(g)} viewBox="0 0 24 24">
          <path
            d={BADGE_GLYPHS[g]}
            fill={g === 'star' ? 'currentColor' : 'none'}
            stroke="currentColor"
            strokeWidth={g === 'star' ? 1 : 2.6}
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </symbol>
      ))}
    </>
  );
});

type BodiesByKit = Partial<Record<MapKitKey, KitSymbolBodies>>;

function loadedBodies(): BodiesByKit {
  const out: BodiesByKit = {};
  for (const k of MAP_KIT_KEYS) {
    const b = loadedKitSymbols(k);
    if (b) out[k] = b;
  }
  return out;
}

/** Symbol bodies of the kits in use, each kit loaded the first time it is needed. */
function useKitBodies(kits: readonly MapKitKey[]): BodiesByKit {
  const [bodies, setBodies] = useState(loadedBodies);
  const missing = kits.filter((k) => !bodies[k]).join(' ');
  useEffect(() => {
    if (!missing) return;
    let alive = true;
    for (const k of missing.split(' ') as MapKitKey[]) {
      void loadKitSymbols(k).then((b) => alive && setBodies((prev) => ({ ...prev, [k]: b })));
    }
    return () => {
      alive = false;
    };
  }, [missing]);
  return bodies;
}

/** Registry key of an id drawn with a kit. */
const keyOf = (kit: MapKitKey, id: string) => `${kit}/${id}`;

/**
 * The shared sprite of the beach map kit: ONE hidden <svg><defs> with only
 * the symbols the page draws (union of every mounted instance, see
 * registry.ts, keyed by kit and id) plus the status badge glyphs. Maps draw
 * each element with `<use href="#ark-<kit>-<id>">`, so a map of 1,000
 * umbrellas holds the umbrella drawing once, and two maps with different
 * kits (the kit preview) never clash. The bodies load lazily per kit
 * (symbols.ts).
 *
 * The leader renders the sprite through a portal on <body>, outside its own
 * subtree: a leader inside a hidden tab panel, a collapsed responsive
 * column or a popover never hides the drawings of the other maps, and the
 * handover when the leader unmounts keeps the sprite at the same place.
 */
export function MapKitDefs({ ids, kit: kitProp }: MapKitDefsProps) {
  const contextKit = useMapKit();
  const kit = kitProp ?? contextKit;
  const [token] = useState(() => Symbol('map-kit-defs'));
  const key = useMemo(
    () => [...new Set(ids.map((id) => keyOf(kit, id)))].sort().join(' '),
    [ids, kit],
  );

  useLayoutEffect(() => {
    registerKitIds(token, new Set(key ? key.split(' ') : []));
  }, [token, key]);
  useLayoutEffect(() => () => unregisterKitIds(token), [token]);

  // The union is memoised by the store: a new snapshot only when a set changes.
  const { leader, ids: union } = useSyncExternalStore(
    subscribeKitIds,
    kitIdsSnapshot,
    kitIdsSnapshot,
  );
  const kits = useMemo(
    () => MAP_KIT_KEYS.filter((k) => union.some((u) => u.startsWith(`${k}/`))),
    [union],
  );
  const bodies = useKitBodies(leader === token ? kits : []);

  if (leader !== token || typeof document === 'undefined') return null;
  return createPortal(
    <svg
      aria-hidden="true"
      focusable="false"
      width="0"
      height="0"
      data-testid="map-kit-defs"
      className="pointer-events-none absolute size-0 overflow-hidden"
    >
      <defs>
        <BadgeSymbols />
        {union.map((key) => {
          const [k, id] = key.split('/') as [MapKitKey, string];
          const body = bodies[k]?.[id as keyof KitSymbolBodies];
          return body ? <KitSymbol key={key} kit={k} id={id} body={body} /> : null;
        })}
      </defs>
    </svg>,
    document.body,
  );
}
