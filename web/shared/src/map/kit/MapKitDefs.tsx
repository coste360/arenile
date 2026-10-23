import { memo, useEffect, useLayoutEffect, useMemo, useState, useSyncExternalStore } from 'react';
import { createPortal } from 'react-dom';
import { KIT_VIEWBOX } from './generated/manifest';
import { kitSymbolId } from './assets';
import { kitIdsSnapshot, registerKitIds, subscribeKitIds, unregisterKitIds } from './registry';
import { badgeSymbolId, type BadgeGlyph } from './status';
import { loadKitSymbols, loadedKitSymbols, type KitSymbolBodies } from './symbols';

export interface MapKitDefsProps {
  /** Kit asset ids drawn by the caller (memoise the array: it is compared by content). */
  ids: readonly string[];
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

const KitSymbol = memo(function KitSymbol({ id, body }: { id: string; body: string }) {
  return (
    <symbol
      id={kitSymbolId(id)}
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

function useKitBodies(): KitSymbolBodies | null {
  const [bodies, setBodies] = useState(loadedKitSymbols);
  useEffect(() => {
    if (bodies) return;
    let alive = true;
    void loadKitSymbols().then((b) => alive && setBodies(b));
    return () => {
      alive = false;
    };
  }, [bodies]);
  return bodies;
}

/**
 * The shared sprite of the beach map kit: ONE hidden <svg><defs> with only
 * the symbols the page draws (union of every mounted instance, see
 * registry.ts) plus the status badge glyphs. Maps draw each element with
 * `<use href="#ark-<id>">`, so a map of 1,000 umbrellas holds the umbrella
 * drawing once. The symbol bodies load lazily (symbols.ts).
 *
 * The leader renders the sprite through a portal on <body>, outside its own
 * subtree: a leader inside a hidden tab panel, a collapsed responsive
 * column or a popover never hides the drawings of the other maps, and the
 * handover when the leader unmounts keeps the sprite at the same place.
 */
export function MapKitDefs({ ids }: MapKitDefsProps) {
  const [token] = useState(() => Symbol('map-kit-defs'));
  const key = useMemo(() => [...new Set(ids)].sort().join(' '), [ids]);

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
  const bodies = useKitBodies();

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
        {bodies &&
          union.map((id) => {
            const body = bodies[id as keyof KitSymbolBodies];
            return body ? <KitSymbol key={id} id={id} body={body} /> : null;
          })}
      </defs>
    </svg>,
    document.body,
  );
}
