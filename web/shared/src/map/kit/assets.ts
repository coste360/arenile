import { KIT_ASSET_IDS, KIT_MANIFEST, type KitAsset, type KitAssetId } from './generated/manifest';

/**
 * Beach map kit catalogue helpers (docs/adr/0012-kit-mappa.md). The data is
 * generated from assets/beach-map-kit by `make gen`; this file only adds
 * lookups and the default illustration of each kind.
 */

export const KIT_BY_ID: ReadonlyMap<string, KitAsset> = new Map(KIT_MANIFEST.map((a) => [a.id, a]));

const IDS: ReadonlySet<string> = new Set(KIT_ASSET_IDS);

export function isKitAssetId(id: string | null | undefined): id is KitAssetId {
  return !!id && IDS.has(id);
}

/** `id` of the <symbol> of an asset in the shared sprite. */
export function kitSymbolId(id: string): string {
  return `ark-${id}`;
}

/** Illustration of a spot whose `asset` is empty, by kind (custom: none, plain shape). */
export const KIT_SPOT_DEFAULTS: Readonly<Record<string, KitAssetId>> = {
  umbrella: 'ombrellone_2_lettini',
  gazebo: 'gazebo',
  tent: 'tenda_righe',
  palm: 'ombrellone_paglia',
  sunbed: 'lettino',
  daybed: 'daybed',
  cabana: 'cabana',
  hammock: 'amaca',
};

/** Illustration of a decoration whose `asset` is empty, by kind (text, custom: none). */
export const KIT_DECORATION_DEFAULTS: Readonly<Record<string, KitAssetId>> = {
  walkway: 'passerella_orizzontale',
  shower: 'doccia',
  lifeguard: 'torretta_bagnino',
  bar: 'bar',
  entrance: 'ingresso',
  cabin_block: 'cabine_multiple',
};

/** The illustration a spot is drawn with, or null for a plain shape. */
export function spotAsset(spot: { kind: string; asset?: string | null }): KitAssetId | null {
  if (isKitAssetId(spot.asset) && KIT_BY_ID.get(spot.asset)?.type === 'bookable') return spot.asset;
  return KIT_SPOT_DEFAULTS[spot.kind] ?? null;
}

/**
 * The illustration a decoration is drawn with, or null (text, custom without
 * asset). The default straight walkway follows the long side of the shape.
 */
export function decorationAsset(d: {
  kind: string;
  asset?: string | null;
  shape: { w: number; h: number };
}): KitAssetId | null {
  if (isKitAssetId(d.asset)) return d.asset;
  if (d.kind === 'walkway' && d.shape.h > d.shape.w) return 'passerella_verticale';
  return KIT_DECORATION_DEFAULTS[d.kind] ?? null;
}

/** Assets of a type family, in manifest order. */
export function kitAssets(filter: (a: KitAsset) => boolean): readonly KitAsset[] {
  return KIT_MANIFEST.filter(filter);
}

export const isBookableAsset = (a: KitAsset) => a.type === 'bookable';
export const isDecorationAsset = (a: KitAsset) => a.type !== 'bookable';

const STRAIGHT_TILES = ['passerella_orizzontale', 'passerella_verticale'] as const;

/**
 * Kit ids drawn by a map (for <MapKitDefs ids>): the illustration of every
 * spot and decoration, defaults included; a straight walkway may be drawn in
 * either orientation, so both are listed.
 */
export function geometryKitIds(geometry: {
  spots: readonly { kind: string; asset?: string | null }[];
  decorations?:
    readonly { kind: string; asset?: string | null; shape: { w: number; h: number } }[] | null;
}): string[] {
  const ids = new Set<string>();
  for (const s of geometry.spots) {
    const a = spotAsset(s);
    if (a) ids.add(a);
  }
  for (const d of geometry.decorations ?? []) {
    const a = decorationAsset(d);
    if (!a) continue;
    ids.add(a);
    if ((STRAIGHT_TILES as readonly string[]).includes(a))
      for (const t of STRAIGHT_TILES) ids.add(t);
  }
  return [...ids].sort();
}
