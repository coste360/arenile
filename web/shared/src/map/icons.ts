import {
  DoorOpen,
  Footprints,
  LifeBuoy,
  Martini,
  Shapes,
  ShowerHead,
  Store,
  Tent,
  TreePalm,
  Type,
  Umbrella,
  Warehouse,
  type LucideIcon,
} from 'lucide-react';
import type { DecorationKind, SpotKind } from './types';

/** Glyph of each spot kind (palette, kind selects, legends). */
export const SPOT_KIND_ICONS: Record<SpotKind, LucideIcon> = {
  umbrella: Umbrella,
  gazebo: Store,
  tent: Tent,
  palm: TreePalm,
  custom: Shapes,
};

/** Glyph of each decoration kind (palette, map decorations). */
export const DECORATION_KIND_ICONS: Record<DecorationKind, LucideIcon> = {
  walkway: Footprints,
  shower: ShowerHead,
  lifeguard: LifeBuoy,
  bar: Martini,
  entrance: DoorOpen,
  cabin_block: Warehouse,
  text: Type,
  custom: Shapes,
};

/** Glyph of a spot kind; unknown kinds fall back to the generic shape. */
export function spotKindIcon(kind: string): LucideIcon {
  return SPOT_KIND_ICONS[kind as SpotKind] ?? Shapes;
}

/** Glyph of a decoration kind; unknown kinds fall back to the generic shape. */
export function decorationKindIcon(kind: string): LucideIcon {
  return DECORATION_KIND_ICONS[kind as DecorationKind] ?? Shapes;
}
