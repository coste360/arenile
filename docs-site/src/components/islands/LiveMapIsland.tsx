import { useMemo, useState } from 'react';
import { BeachMap } from '@arenile/shared/map/BeachMap';
import type { MapGeometry, MapSpot, SpotStatus } from '@arenile/shared/map/types';
import { SPOT_STATUSES } from '@arenile/shared/map/types';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { StatusBadge } from '@arenile/shared/ui/patterns/StatusBadge';
import demoMap from '../../data/demo-map.json';
import type { DemoMapSnapshot, I18nText } from '../../data/types';
import { IslandShell } from './IslandShell';
import { statusLabel, type DocsLocale } from './i18n';

export interface LiveMapLabels {
  live: string;
  demo: string;
  filter: string;
  all: string;
  selected: string;
  hint: string;
  row: string;
  spots: string;
}

export interface LiveMapIslandProps {
  locale: DocsLocale;
  labels: LiveMapLabels;
  /** CSS max height of the drawing (default 32rem). */
  height?: string;
  /** Show the status filter chips (default true). */
  filters?: boolean;
}

// Bundled with the island (cached, shared by every page) instead of being
// serialised into each page's HTML as props.
const snapshot = demoMap as unknown as DemoMapSnapshot;

const localize = (t: I18nText | undefined, locale: DocsLocale) => (t ? (t[locale] ?? t.it ?? '') : '');

/**
 * The REAL beach map of web/shared (BeachMap + Coste360 map kit) on the demo
 * snapshot of `seed demo`: zoom, pan, keyboard navigation and legend are the
 * panel's own. The docs add only a status filter (highlight) and a detail
 * line for the activated spot.
 */
export default function LiveMapIsland(props: LiveMapIslandProps) {
  return (
    <IslandShell locale={props.locale}>
      <LiveMap {...props} />
    </IslandShell>
  );
}

function LiveMap({ locale, labels, height = '32rem', filters = true }: LiveMapIslandProps) {
  const [selected, setSelected] = useState<MapSpot | null>(null);
  const [filter, setFilter] = useState<SpotStatus | null>(null);

  const geometry = useMemo<MapGeometry>(
    () => ({
      ...snapshot.geometry,
      name: localize(snapshot.name, locale),
      decorations: (snapshot.geometry.decorations ?? []).map((d) => ({
        ...d,
        label: localize(snapshot.decorationLabels[d.id], locale) || null,
      })),
    }),
    [locale],
  );

  const counts = useMemo(() => {
    const out = new Map<SpotStatus, number>();
    for (const s of snapshot.geometry.spots) {
      const st = snapshot.statuses[s.id] ?? 'free';
      out.set(st, (out.get(st) ?? 0) + 1);
    }
    return SPOT_STATUSES.filter((s) => out.has(s)).map((s) => [s, out.get(s)!] as const);
  }, []);

  const highlighted = useMemo(
    () =>
      filter
        ? snapshot.geometry.spots.filter((s) => (snapshot.statuses[s.id] ?? 'free') === filter).map((s) => s.id)
        : [],
    [filter],
  );

  const date = locale === 'it' ? formatIsoDate(snapshot.date) : snapshot.date;
  const selectedStatus = selected ? (snapshot.statuses[selected.id] ?? 'free') : null;

  return (
    <div className="space-y-3 rounded-xl border bg-card p-3 text-card-foreground shadow-sm sm:p-4">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <Badge variant="secondary">{labels.live}</Badge>
        <span className="text-muted-foreground">
          {labels.demo} · {snapshot.establishment.name} · {geometry.name} · {date}
        </span>
      </div>

      {filters && (
        <div role="group" aria-label={labels.filter} className="flex flex-wrap gap-1.5">
          <Button
            size="sm"
            variant={filter === null ? 'default' : 'outline'}
            aria-pressed={filter === null}
            onClick={() => setFilter(null)}
          >
            {labels.all} · {snapshot.geometry.spots.length}
          </Button>
          {counts.map(([status, n]) => (
            <Button
              key={status}
              size="sm"
              variant={filter === status ? 'secondary' : 'ghost'}
              aria-pressed={filter === status}
              onClick={() => setFilter(filter === status ? null : status)}
              className="px-1"
            >
              <StatusBadge spotStatus={status}>
                {statusLabel(locale, status)} · {n}
              </StatusBadge>
            </Button>
          ))}
        </div>
      )}

      <BeachMap
        geometry={geometry}
        statuses={snapshot.statuses}
        highlightedIds={highlighted}
        selectedIds={selected ? [selected.id] : []}
        onSpotActivate={(spot) => setSelected((cur) => (cur?.id === spot.id ? null : spot))}
        showLegend={false}
        height={height}
      />

      <p aria-live="polite" className="min-h-6 text-sm text-muted-foreground">
        {selected && selectedStatus ? (
          <span className="inline-flex flex-wrap items-center gap-2">
            <span className="font-medium text-foreground">
              {labels.selected} {selected.label}
            </span>
            {selected.row_label && <span>{labels.row.replace('{{row}}', selected.row_label)}</span>}
            <StatusBadge spotStatus={selectedStatus}>{statusLabel(locale, selectedStatus)}</StatusBadge>
          </span>
        ) : (
          labels.hint
        )}
      </p>
    </div>
  );
}
