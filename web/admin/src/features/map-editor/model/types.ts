import type { Schemas } from '@arenile/shared/api';

export type ApiMap = Schemas['Map'];
export type ApiMapInput = Schemas['MapInput'];
export type ApiMapDocument = Schemas['MapDocument'];
export type ApiMapSaveRequest = Schemas['MapSaveRequest'];
export type ApiSpot = Schemas['Spot'];
export type ApiSpotInput = Schemas['SpotInput'];
export type ApiDecoration = Schemas['Decoration'];
export type ApiDecorationInput = Schemas['DecorationInput'];
export type ApiZone = Schemas['Zone'];
export type ApiComponent = Schemas['Component'];
export type RowGeneratorRequest = Schemas['RowGeneratorRequest'];
export type RowGeneratorResult = Schemas['RowGeneratorResult'];
export type MapReorientRequest = Schemas['MapReorientRequest'];
export type MapReorientResult = Schemas['MapReorientResult'];
export type SpotKind = Schemas['SpotKind'];
export type DecorationKind = Schemas['DecorationKind'];
export type SeaSide = Schemas['SeaSide'];
export type ShapeDef = Schemas['Shape'];
export type ComponentQuantities = Schemas['ComponentQuantities'];
export type I18nText = Record<string, string>;

/** Every SpotKind of the API, as a tuple so that zod enums derive from it. */
export const SPOT_KINDS = [
  'umbrella',
  'gazebo',
  'tent',
  'palm',
  'sunbed',
  'daybed',
  'cabana',
  'hammock',
  'custom',
] as const satisfies readonly SpotKind[];
export const DECORATION_KINDS: readonly DecorationKind[] = [
  'walkway',
  'shower',
  'lifeguard',
  'bar',
  'entrance',
  'cabin_block',
  'text',
  'custom',
];
export const SEA_SIDES: readonly SeaSide[] = ['top', 'bottom', 'left', 'right'];

/**
 * A spot in the editor draft. `key` is the stable client identity: the
 * server id for persisted spots, `new:<n>` for spots created in this session
 * (sent without `id`, so the server creates them).
 */
export interface DraftSpot {
  key: string;
  id?: string;
  zone_id: string;
  label: string;
  row_label: string;
  number: number | null;
  kind: SpotKind;
  /** Kit illustration id; '' = default of the kind (ADR 0012). */
  asset: string;
  x: number;
  y: number;
  rotation: number;
  shape: ShapeDef;
  capacity: ComponentQuantities;
  max_extra: ComponentQuantities;
  price_category_override: string;
  attributes: string[];
  active: boolean;
  /** Sold on the public page (spec 30.11); undefined = true (the default). */
  online_bookable?: boolean;
}

export interface DraftDecoration {
  key: string;
  id?: string;
  kind: DecorationKind;
  /** Kit illustration id; '' = default of the kind (text, custom: none). */
  asset: string;
  /** Colour variant of the illustration (sea flag); '' = default. */
  variant: string;
  x: number;
  y: number;
  rotation: number;
  shape: ShapeDef;
  label: I18nText;
}

export interface DraftMapSettings {
  name: I18nText;
  width: number;
  height: number;
  grid_size: number;
  sea_side: SeaSide;
  sort_order: number;
  background?: Schemas['MapBackground'];
}

/** The whole editable map document, immutable: every edit returns a new one. */
export interface DraftDocument {
  map: DraftMapSettings;
  spots: DraftSpot[];
  decorations: DraftDecoration[];
}

export type ElementType = 'spot' | 'decoration';

/** Selection is a set of element keys (spots and decorations share the key space). */
export type Selection = ReadonlySet<string>;

export type DraftElement = DraftSpot | DraftDecoration;

export function isSpot(el: DraftElement): el is DraftSpot {
  return 'label' in el && typeof (el as DraftSpot).label === 'string' && 'zone_id' in el;
}
