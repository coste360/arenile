import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { cn } from '../lib/utils';
import { Button } from '../ui/button';
import { STATUS_CLASSES } from './colors';
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
    <section className={cn('space-y-4', className)} aria-label={t('map.listTitle')}>
      {rows.map(([row, list]) => (
        <div key={row ?? '__none'} className="space-y-2">
          <h3 className="text-sm font-medium text-muted-foreground">
            {row ? t('map.row', { row }) : t('map.noRow')}
          </h3>
          <ul className="flex flex-wrap gap-1.5">
            {list.map((spot) => {
              const status: SpotStatus =
                spot.active === false ? 'unavailable' : (statuses?.[spot.id] ?? defaultStatus);
              const extra = spotDescription?.(spot);
              const disabled = status !== 'free' || !onSpotActivate;
              const pressed = selected.has(spot.id);
              const name = `${spot.label} – ${t(`map.status.${status}`)}${extra ? ` – ${extra}` : ''}`;
              return (
                <li key={spot.id}>
                  <Button
                    variant={pressed ? 'default' : 'outline'}
                    size="sm"
                    disabled={disabled}
                    aria-pressed={pressed}
                    aria-label={name}
                    onClick={() => onSpotActivate?.(spot)}
                    className="font-normal"
                  >
                    <span
                      aria-hidden="true"
                      className={cn(
                        'size-2.5 shrink-0 rounded-full ring-1 ring-foreground/20',
                        STATUS_CLASSES[status].bg,
                      )}
                    />
                    <span className="font-medium">{spot.label}</span>
                    <span className="text-xs opacity-80">
                      {t(`map.status.${status}`)}
                      {extra ? ` – ${extra}` : ''}
                    </span>
                  </Button>
                </li>
              );
            })}
          </ul>
        </div>
      ))}
    </section>
  );
}
