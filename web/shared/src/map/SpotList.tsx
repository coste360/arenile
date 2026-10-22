import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { MapSpot, SpotStatus } from './types';

export interface SpotListProps {
  spots: readonly MapSpot[];
  statuses?: Readonly<Record<string, SpotStatus>>;
  defaultStatus?: SpotStatus;
  selectedIds?: readonly string[];
  onSpotActivate?: (spot: MapSpot) => void;
  spotDescription?: (spot: MapSpot) => string | undefined;
  className?: string;
}

/** Groups spots by row label, keeping a natural order of rows and numbers. */
export function groupByRow(spots: readonly MapSpot[]): [string | null, MapSpot[]][] {
  const groups = new Map<string | null, MapSpot[]>();
  for (const s of spots) {
    const key = s.row_label ?? null;
    const list = groups.get(key) ?? [];
    list.push(s);
    groups.set(key, list);
  }
  const collator = new Intl.Collator(undefined, { numeric: true });
  return [...groups.entries()]
    .sort(([a], [b]) => (a === null ? 1 : b === null ? -1 : collator.compare(a, b)))
    .map(([row, list]) => [
      row,
      [...list].sort((a, b) =>
        a.number != null && b.number != null
          ? a.number - b.number
          : collator.compare(a.label, b.label),
      ),
    ]);
}

/**
 * List alternative to the map, grouped by row (spec 11.1), for screen
 * readers and small screens.
 */
export function SpotList({
  spots,
  statuses,
  defaultStatus = 'free',
  selectedIds,
  onSpotActivate,
  spotDescription,
  className,
}: SpotListProps) {
  const { t } = useTranslation();
  const rows = useMemo(() => groupByRow(spots), [spots]);
  const selected = new Set(selectedIds ?? []);
  return (
    <section className={className} aria-label={t('map.listTitle')}>
      {rows.map(([row, list]) => (
        <div key={row ?? '__none'}>
          <h3>{row ? t('map.row', { row }) : t('map.noRow')}</h3>
          <ul>
            {list.map((spot) => {
              const status: SpotStatus =
                spot.active === false ? 'unavailable' : (statuses?.[spot.id] ?? defaultStatus);
              const extra = spotDescription?.(spot);
              const disabled = status !== 'free' || !onSpotActivate;
              return (
                <li key={spot.id}>
                  <button
                    type="button"
                    disabled={disabled}
                    aria-pressed={selected.has(spot.id)}
                    onClick={() => onSpotActivate?.(spot)}
                  >
                    {spot.label} – {t(`map.status.${status}`)}
                    {extra ? ` – ${extra}` : ''}
                  </button>
                </li>
              );
            })}
          </ul>
        </div>
      ))}
    </section>
  );
}
