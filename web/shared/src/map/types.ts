/** Side of the map facing the sea (spec 6.2, maps.sea_side). */
export type SeaSide = 'top' | 'bottom' | 'left' | 'right';

export type SpotKind = 'umbrella' | 'gazebo' | 'tent' | 'palm' | 'custom';

export type DecorationKind =
  'walkway' | 'shower' | 'lifeguard' | 'bar' | 'entrance' | 'cabin_block' | 'text' | 'custom';

export interface Shape {
  type: 'circle' | 'rect';
  /** Width in logical map units (diameter for circles). */
  w: number;
  /** Height in logical map units (ignored for circles). */
  h: number;
}

/**
 * A bookable spot. `x`/`y` are the centre of the shape in logical map units,
 * `rotation` is in degrees around the centre.
 */
export interface MapSpot {
  id: string;
  label: string;
  row_label?: string | null;
  number?: number | null;
  kind: SpotKind | (string & {});
  x: number;
  y: number;
  rotation?: number | null;
  shape: Shape;
  zone_id?: string | null;
  active?: boolean | null;
}

export interface MapDecoration {
  id: string;
  kind: DecorationKind | (string & {});
  x: number;
  y: number;
  rotation?: number | null;
  shape: Shape;
  /** Already localised label, if any. */
  label?: string | null;
}

/** Read-only geometry of a map (spec 6.2, public `GET /maps/{id}`). */
export interface MapGeometry {
  id?: string;
  name?: string | null;
  width: number;
  height: number;
  grid_size?: number | null;
  sea_side: SeaSide;
  spots: MapSpot[];
  decorations?: MapDecoration[] | null;
}

/** Operational spot status (spec 11.3) plus `unavailable` for the public page. */
export type SpotStatus =
  | 'free'
  | 'booked_online'
  | 'booked_desk'
  | 'season'
  | 'season_released'
  | 'hold'
  | 'to_collect'
  | 'present'
  | 'blocked'
  | 'renewal_hold'
  | 'unavailable';

export const SPOT_STATUSES: readonly SpotStatus[] = [
  'free',
  'booked_online',
  'booked_desk',
  'season',
  'season_released',
  'hold',
  'to_collect',
  'present',
  'blocked',
  'renewal_hold',
  'unavailable',
];
