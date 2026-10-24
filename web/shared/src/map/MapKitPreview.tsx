import { useTranslation } from 'react-i18next';
import { cn } from '../lib/utils';
import { seaRect } from './geometry';
import {
  DecorationArt,
  MapBackdrop,
  MapKitDefs,
  MapKitProvider,
  MapLabel,
  SpotArt,
  StatusBadge,
  badgeOffset,
  spotArtSize,
  type MapKitKey,
} from './kit';
import type { MapDecoration, MapGeometry, MapSpot, SpotStatus } from './types';

/** A short row of spots in the statuses that matter most, plus a few services. */
const PREVIEW_STATUSES: readonly SpotStatus[] = [
  'free',
  'booked_online',
  'booked_desk',
  'present',
  'to_collect',
  'season',
  'blocked',
];

const SPOT_SIZE = 40;
const STEP = 58;

const SPOTS: MapSpot[] = PREVIEW_STATUSES.map((_, i) => ({
  id: `p${i + 1}`,
  label: `A${i + 1}`,
  kind: i === 5 ? 'gazebo' : 'umbrella',
  x: 36 + i * STEP,
  y: 52,
  shape:
    i === 5
      ? { type: 'rect', w: SPOT_SIZE, h: SPOT_SIZE }
      : { type: 'circle', w: SPOT_SIZE, h: SPOT_SIZE },
}));

const DECORATIONS: MapDecoration[] = [
  { id: 'walk', kind: 'walkway', x: 210, y: 112, shape: { type: 'rect', w: 400, h: 25 } },
  { id: 'bar', kind: 'bar', x: 448, y: 52, shape: { type: 'rect', w: 48, h: 48 } },
];

const GEOMETRY: Pick<MapGeometry, 'width' | 'height' | 'sea_side'> = {
  width: 480,
  height: 132,
  sea_side: 'top',
};

/** The sea band lies outside the map rectangle: the drawing includes it. */
const SEA = seaRect(GEOMETRY);
const VIEWBOX = `0 ${SEA.y} ${GEOMETRY.width} ${GEOMETRY.height + SEA.height}`;

const KIT_IDS = ['ombrellone_2_lettini', 'gazebo', 'passerella_orizzontale', 'bar'];

export interface MapKitPreviewProps {
  kit: MapKitKey;
  className?: string;
}

/**
 * Static sample of a map drawn with one kit (Configurazione → Stabilimento →
 * Mappa, the docs): the same row of spots in the main statuses, with their
 * badges, a walkway and the bar, on the sand and sea of the map. Decorative:
 * the caller names it (the kit choice it illustrates), so it is aria-hidden.
 */
export function MapKitPreview({ kit, className }: MapKitPreviewProps) {
  const { t } = useTranslation();
  return (
    <MapKitProvider kit={kit}>
      <svg
        viewBox={VIEWBOX}
        aria-hidden="true"
        focusable="false"
        data-map-kit-preview={kit}
        className={cn('block h-auto w-full rounded-md', className)}
      >
        <MapBackdrop geometry={GEOMETRY} seaLabel={t('map.sea')} />
        {DECORATIONS.map((d) => (
          <DecorationArt
            key={d.id}
            asset={d.kind === 'bar' ? 'bar' : 'passerella_orizzontale'}
            x={d.x}
            y={d.y}
            shape={d.shape}
          />
        ))}
        {SPOTS.map((s, i) => {
          const status = PREVIEW_STATUSES[i] as SpotStatus;
          const S = spotArtSize(s.shape);
          const corner = S * badgeOffset(kit);
          return (
            <g key={s.id} data-status={status}>
              <SpotArt
                x={s.x}
                y={s.y}
                shape={s.shape}
                asset={s.kind === 'gazebo' ? 'gazebo' : 'ombrellone_2_lettini'}
                status={status}
                tint={status === 'free' ? 'faint' : 'strong'}
              />
              <StatusBadge status={status} x={s.x + corner} y={s.y - corner} r={S * 0.12} />
              <MapLabel x={s.x} y={s.y + S * 0.42} text={s.label} fontSize={9} />
            </g>
          );
        })}
      </svg>
      <MapKitDefs ids={KIT_IDS} kit={kit} />
    </MapKitProvider>
  );
}
