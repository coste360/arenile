import type { MapDecoration, MapGeometry, MapSpot, SpotStatus } from './types';

/**
 * Builds a classic "lido a file" layout: `rows` rows of `perRow` umbrellas,
 * row A closest to the sea. Used for demos, tests and the panel preview.
 */
export function sampleGeometry(rows = 6, perRow = 12): MapGeometry {
  const spacing = 40;
  const margin = 40;
  const width = margin * 2 + spacing * (perRow - 1) + 40;
  const height = margin * 2 + spacing * (rows - 1) + 120;
  const spots: MapSpot[] = [];
  for (let r = 0; r < rows; r++) {
    const row = String.fromCharCode(65 + r);
    for (let n = 1; n <= perRow; n++) {
      spots.push({
        id: `${row}${n}`,
        label: `${row}${n}`,
        row_label: row,
        number: n,
        kind: 'umbrella',
        x: margin + 20 + (n - 1) * spacing,
        y: height - margin - 20 - r * spacing,
        shape: { type: 'circle', w: 30, h: 30 },
        active: !(r === rows - 1 && n === perRow),
      });
    }
  }
  const decorations: MapDecoration[] = [
    { id: 'walkway', kind: 'walkway', x: width / 2, y: 50, shape: { type: 'rect', w: 24, h: 70 } },
    { id: 'bar', kind: 'bar', x: 70, y: 40, shape: { type: 'rect', w: 90, h: 50 } },
    { id: 'shower', kind: 'shower', x: width - 60, y: 40, shape: { type: 'circle', w: 34, h: 34 } },
    {
      id: 'lifeguard',
      kind: 'lifeguard',
      x: width - 25,
      y: height - 25,
      shape: { type: 'rect', w: 30, h: 30 },
    },
  ];
  return { id: 'demo', width, height, grid_size: 10, sea_side: 'bottom', spots, decorations };
}

/** Deterministic pseudo-random statuses for the demo map. */
export function sampleStatuses(geometry: MapGeometry): Record<string, SpotStatus> {
  const cycle: SpotStatus[] = [
    'free',
    'booked_online',
    'free',
    'booked_desk',
    'season',
    'free',
    'present',
    'to_collect',
    'free',
    'hold',
    'season_released',
    'blocked',
    'renewal_hold',
  ];
  const out: Record<string, SpotStatus> = {};
  geometry.spots.forEach((s, i) => {
    out[s.id] = cycle[(i * 7) % cycle.length] as SpotStatus;
  });
  return out;
}
