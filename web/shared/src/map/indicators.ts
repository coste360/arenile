import {
  HandshakeIcon,
  LogInIcon,
  LogOutIcon,
  StickyNoteIcon,
  CirclePlusIcon,
  StarIcon,
  UsersRoundIcon,
  MapPinOffIcon,
  type LucideIcon,
} from 'lucide-react';

/**
 * Overlay indicators drawn on a spot of the operational map (spec 11.3:
 * "icona extra, nota presente, cliente VIP", plus the daily ticket parked
 * on a provisional spot, still "da assegnare", ADR 0020; spec 30.4 adds
 * arrival and departure day, agreement or partner, group). Each one is a
 * coloured dot with its own glyph, so it never depends on colour alone;
 * callers repeat them in the spot's accessible description and in a legend.
 */
export type SpotIndicator =
  'extra' | 'note' | 'vip' | 'to_assign' | 'arrival' | 'departure' | 'agreement' | 'group';

export const SPOT_INDICATORS: readonly SpotIndicator[] = [
  'extra',
  'note',
  'vip',
  'to_assign',
  'arrival',
  'departure',
  'agreement',
  'group',
];

// Written out literally so Tailwind can see every class name (theme tokens only).
export const INDICATOR_CLASSES: Record<SpotIndicator, { fill: string; bg: string; fg: string }> = {
  extra: { fill: 'fill-chart-3', bg: 'bg-chart-3', fg: 'text-background' },
  note: { fill: 'fill-primary', bg: 'bg-primary', fg: 'text-primary-foreground' },
  vip: { fill: 'fill-warning', bg: 'bg-warning', fg: 'text-foreground' },
  to_assign: { fill: 'fill-destructive', bg: 'bg-destructive', fg: 'text-background' },
  arrival: { fill: 'fill-success', bg: 'bg-success', fg: 'text-background' },
  departure: { fill: 'fill-chart-5', bg: 'bg-chart-5', fg: 'text-background' },
  agreement: { fill: 'fill-chart-1', bg: 'bg-chart-1', fg: 'text-background' },
  group: { fill: 'fill-chart-2', bg: 'bg-chart-2', fg: 'text-background' },
};

/** Glyph of each indicator, drawn inside its dot when the dot is large enough and in legends. */
export const INDICATOR_ICONS: Record<SpotIndicator, LucideIcon> = {
  extra: CirclePlusIcon,
  note: StickyNoteIcon,
  vip: StarIcon,
  to_assign: MapPinOffIcon,
  arrival: LogInIcon,
  departure: LogOutIcon,
  agreement: HandshakeIcon,
  group: UsersRoundIcon,
};
