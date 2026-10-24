import { createContext, useContext, type ReactNode } from 'react';
import {
  DEFAULT_MAP_KIT,
  MAP_KITS,
  MAP_KIT_KEYS,
  type MapKitInfo,
  type MapKitKey,
} from './generated/manifest';

/**
 * Which illustration kit the maps of a subtree draw (ADR 0024): the
 * establishment's `theme.map_kit`, provided once by the app shell, so every
 * map, palette and thumbnail follows the same choice without threading a prop.
 * A `<BeachMap kit>` (previews) overrides it for its own drawing.
 */
const MapKitContext = createContext<MapKitKey>(DEFAULT_MAP_KIT);

export function isMapKitKey(v: unknown): v is MapKitKey {
  return typeof v === 'string' && (MAP_KIT_KEYS as readonly string[]).includes(v);
}

/** The kit to draw for a stored value: unknown or missing values use the default kit. */
export function resolveMapKit(v: string | null | undefined): MapKitKey {
  return isMapKitKey(v) ? v : DEFAULT_MAP_KIT;
}

export function MapKitProvider({
  kit,
  children,
}: {
  /** Stored value (theme.map_kit); missing or unknown = default kit. */
  kit: string | null | undefined;
  children: ReactNode;
}) {
  return <MapKitContext.Provider value={resolveMapKit(kit)}>{children}</MapKitContext.Provider>;
}

/** The kit of the nearest provider (default kit outside any provider). */
export function useMapKit(): MapKitKey {
  return useContext(MapKitContext);
}

/** Generated description of a kit (version, how the status is drawn). */
export function mapKitInfo(kit: MapKitKey): MapKitInfo {
  return MAP_KITS[kit];
}
