/**
 * Overlay indicators drawn on a spot of the operational map (spec 11.3:
 * "icona extra, nota presente, cliente VIP"). They are a visual hint only:
 * callers repeat them in the spot's accessible description and in a legend.
 */
export type SpotIndicator = 'extra' | 'note' | 'vip';

export const SPOT_INDICATORS: readonly SpotIndicator[] = ['extra', 'note', 'vip'];

// Written out literally so Tailwind can see every class name (theme tokens only).
export const INDICATOR_CLASSES: Record<SpotIndicator, { fill: string; bg: string }> = {
  extra: { fill: 'fill-chart-3', bg: 'bg-chart-3' },
  note: { fill: 'fill-primary', bg: 'bg-primary' },
  vip: { fill: 'fill-warning', bg: 'bg-warning' },
};
