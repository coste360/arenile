import { useId, useMemo, useState, type KeyboardEvent } from 'react';
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
  /**
   * Allow activating spots in any status (operational view: open the quick
   * sheet of a booked spot); default false = only free spots (public page).
   * Inactive spots are never activatable.
   */
  activateAny?: boolean;
  /** Status names overriding the defaults (see BeachMap `statusLabels`). */
  statusLabels?: Partial<Record<SpotStatus, string>>;
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

const NAV_KEYS: Record<string, 'prev' | 'next' | 'first' | 'last'> = {
  ArrowLeft: 'prev',
  ArrowUp: 'prev',
  ArrowRight: 'next',
  ArrowDown: 'next',
  Home: 'first',
  End: 'last',
};

/**
 * List alternative to the map, grouped by row (spec 11.1), for screen
 * readers and small screens.
 *
 * Each row is a toolbar of toggle buttons (WAI-ARIA toolbar pattern, the
 * selection is multiple, so not a radio group) labelled by its heading, with
 * a roving tabindex: one tab stop per row (the first selected spot, else the
 * first activatable one, else the first), arrows move within the row,
 * Home/End jump to its ends. Spots that cannot be activated stay focusable
 * (aria-disabled) so their status can still be read. The accessible name of
 * a spot is its visible text ("A1 Libero – da 30,00 €", WCAG 2.5.3).
 */
export function SpotList({
  spots,
  statuses,
  defaultStatus = 'free',
  selectedIds,
  onSpotActivate,
  spotDescription,
  activateAny = false,
  statusLabels,
  className,
}: SpotListProps) {
  const { t } = useTranslation();
  const baseId = useId();
  const rows = useMemo(() => groupByRow(spots), [spots]);
  const selected = new Set(selectedIds ?? []);
  // Spot id last focused in each row (key: row index), the row's tab stop.
  const [active, setActive] = useState<Record<number, string>>({});
  const statusOf = (spot: MapSpot): SpotStatus =>
    spot.active === false ? 'unavailable' : (statuses?.[spot.id] ?? defaultStatus);
  const isDisabled = (spot: MapSpot) =>
    (activateAny ? spot.active === false : statusOf(spot) !== 'free') || !onSpotActivate;

  const onKeyDown = (rowIndex: number, list: MapSpot[]) => (e: KeyboardEvent<HTMLElement>) => {
    const move = NAV_KEYS[e.key];
    if (!move) return;
    const id = (e.target as Element | null)
      ?.closest?.('[data-spot-id]')
      ?.getAttribute('data-spot-id');
    const at = list.findIndex((s) => s.id === id);
    if (at < 0) return;
    e.preventDefault();
    const to =
      move === 'first'
        ? 0
        : move === 'last'
          ? list.length - 1
          : Math.min(list.length - 1, Math.max(0, at + (move === 'next' ? 1 : -1)));
    const next = list[to];
    if (!next || to === at) return;
    setActive((prev) => ({ ...prev, [rowIndex]: next.id }));
    e.currentTarget.querySelector<HTMLElement>(`[data-spot-id="${CSS.escape(next.id)}"]`)?.focus();
  };

  return (
    <section className={cn('space-y-4', className)} aria-label={t('map.listTitle')}>
      {rows.map(([row, list], rowIndex) => {
        const headingId = `${baseId}-row-${rowIndex}`;
        const remembered = active[rowIndex];
        const tabStop =
          (remembered && list.some((s) => s.id === remembered) ? remembered : undefined) ??
          list.find((s) => selected.has(s.id))?.id ??
          list.find((s) => !isDisabled(s))?.id ??
          list[0]?.id;
        return (
          <div key={row ?? '__none'} className="space-y-2">
            <h3 id={headingId} className="text-sm font-medium text-muted-foreground">
              {row ? t('map.row', { row }) : t('map.noRow')}
            </h3>
            <div role="toolbar" aria-labelledby={headingId} onKeyDown={onKeyDown(rowIndex, list)}>
              <ul className="flex flex-wrap gap-1.5">
                {list.map((spot) => {
                  const status = statusOf(spot);
                  const extra = spotDescription?.(spot);
                  const disabled = isDisabled(spot);
                  const pressed = selected.has(spot.id);
                  return (
                    <li key={spot.id}>
                      <Button
                        variant={pressed ? 'default' : 'outline'}
                        size="sm"
                        data-spot-id={spot.id}
                        tabIndex={spot.id === tabStop ? 0 : -1}
                        aria-disabled={disabled || undefined}
                        aria-pressed={pressed}
                        onClick={() => {
                          if (disabled) return;
                          setActive((prev) => ({ ...prev, [rowIndex]: spot.id }));
                          onSpotActivate?.(spot);
                        }}
                        onFocus={() =>
                          active[rowIndex] !== spot.id &&
                          setActive((prev) => ({ ...prev, [rowIndex]: spot.id }))
                        }
                        className="font-normal aria-disabled:pointer-events-none aria-disabled:opacity-50"
                      >
                        <span
                          aria-hidden="true"
                          className={cn(
                            'size-2.5 shrink-0 rounded-full ring-1 ring-foreground/20',
                            STATUS_CLASSES[status].bg,
                          )}
                        />
                        {/* The spaces keep words apart in the computed accessible name. */}
                        <span className="font-medium">{spot.label}</span>{' '}
                        <span className="text-xs opacity-80">
                          {statusLabels?.[status] ?? t(`map.status.${status}`)}
                          {extra ? ` – ${extra}` : ''}
                        </span>
                      </Button>
                    </li>
                  );
                })}
              </ul>
            </div>
          </div>
        );
      })}
    </section>
  );
}
