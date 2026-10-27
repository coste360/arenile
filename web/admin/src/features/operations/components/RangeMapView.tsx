import { formatIsoDate } from '@arenile/shared/lib/dates';
import { BeachMap, SpotList, type MapGeometry, type MapSpot } from '@arenile/shared/map';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { DialogFooter } from '@arenile/shared/ui/dialog';
import { FormDialog, QueryState, ResponsiveTabsList } from '@arenile/shared/ui/patterns';
import { Tabs, TabsContent } from '@arenile/shared/ui/tabs';
import { PlusIcon } from 'lucide-react';
import { lazy, Suspense, useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useRangeAvailability } from '../api/m8';
import type { OperationsPermissions } from '../lib/permissions';
import { RANGE_STATES, RANGE_STATUS, dateRuns, rangeSpots, type RangeSpot } from '../lib/rangeView';
import type { SlotChoice } from '../lib/status';
import { StatusGlyph } from './StatusGlyph';

const NewBookingSheet = lazy(() =>
  import('../../bookings').then((m) => ({ default: m.NewBookingSheet })),
);

export interface RangeMapViewProps {
  establishmentId: string;
  geometry: MapGeometry;
  range: { from: string; to: string };
  slot: SlotChoice;
  mapId?: string;
  permissions: OperationsPermissions;
  height?: string;
}

/**
 * Range view of the operational map (spec 30.4 "vista a intervallo"):
 * every spot is free on the whole range, free in part ("N giorni su M",
 * drawn as a caption) or taken; each state keeps its glyph. A click shows
 * which days are free and starts a booking on a free stretch.
 */
export function RangeMapView({
  establishmentId,
  geometry,
  range,
  slot,
  mapId,
  permissions,
  height = '65vh',
}: RangeMapViewProps) {
  const { t } = useTranslation('operations');
  const [view, setView] = useState('map');
  const [picked, setPicked] = useState<{ spot: MapSpot; state: RangeSpot } | null>(null);
  const [booking, setBooking] = useState<{
    spotId: string;
    startsOn: string;
    endsOn: string;
  } | null>(null);
  const query = useRangeAvailability(establishmentId, range, slot, mapId);
  const ids = useMemo(() => geometry.spots.map((s) => s.id), [geometry.spots]);
  const states = useMemo(() => rangeSpots(query.data, ids), [query.data, ids]);
  const statuses = useMemo(
    () => Object.fromEntries([...states].map(([id, s]) => [id, RANGE_STATUS[s.state]])),
    [states],
  );
  const captions = useMemo(() => {
    const out: Record<string, string> = {};
    for (const [id, s] of states) {
      if (s.state === 'partial')
        out[id] = t('range.partialShort', { free: s.freeDays, total: s.totalDays });
    }
    return out;
  }, [states, t]);
  // The palette borrows map statuses: the names say what they mean here.
  const statusLabels = useMemo(
    () => Object.fromEntries(RANGE_STATES.map((s) => [RANGE_STATUS[s], t(`range.state.${s}`)])),
    [t],
  );
  const description = useCallback(
    (spot: MapSpot) => {
      const s = states.get(spot.id);
      if (!s) return undefined;
      return s.state === 'partial'
        ? t('range.partialAria', { free: s.freeDays, total: s.totalDays })
        : undefined;
    },
    [states, t],
  );
  const activate = (spot: MapSpot) => {
    const s = states.get(spot.id);
    if (s) setPicked({ spot, state: s });
  };

  return (
    <div className="space-y-3">
      <ul
        className="flex flex-wrap gap-x-4 gap-y-1.5 text-xs"
        aria-label={t('range.legend')}
        data-testid="range-legend"
      >
        {RANGE_STATES.map((s) => (
          <li key={s} className="flex items-center gap-1.5">
            <StatusGlyph status={RANGE_STATUS[s]} />
            {t(`range.state.${s}`)}
          </li>
        ))}
      </ul>
      <QueryState
        isLoading={query.isLoading}
        error={query.error}
        onRetry={() => void query.refetch()}
        skeleton={<div className="h-[65vh] w-full animate-pulse rounded-lg bg-muted" />}
      />
      {query.data && (
        <Tabs value={view} onValueChange={setView}>
          <ResponsiveTabsList
            label={t('map.views')}
            value={view}
            onValueChange={setView}
            options={[
              { value: 'map', label: t('map.viewMap') },
              { value: 'list', label: t('map.viewList') },
            ]}
          />
          <TabsContent value="map">
            <BeachMap
              geometry={geometry}
              statuses={statuses}
              onSpotActivate={activate}
              spotDescription={description}
              statusLabels={statusLabels}
              spotCaptions={captions}
              labelMode="both"
              showLegend={false}
              title={t('range.aria', {
                name: geometry.name ?? '',
                from: formatIsoDate(range.from),
                to: formatIsoDate(range.to),
              })}
              height={height}
            />
          </TabsContent>
          <TabsContent value="list">
            <SpotList
              spots={geometry.spots}
              statuses={statuses}
              onSpotActivate={activate}
              spotDescription={description}
              statusLabels={statusLabels}
              activateAny
            />
          </TabsContent>
        </Tabs>
      )}
      {picked && (
        <FormDialog
          open
          onOpenChange={(v) => !v && setPicked(null)}
          title={t('range.spotTitle', { label: picked.spot.label })}
          description={
            picked.state.state === 'partial'
              ? t('range.partialAria', {
                  free: picked.state.freeDays,
                  total: picked.state.totalDays,
                })
              : t(`range.state.${picked.state.state}`)
          }
        >
          <div className="space-y-4">
            {picked.state.state === 'partial' && (
              <ul className="space-y-2 text-sm" aria-label={t('range.freeDays')}>
                {dateRuns(picked.state.freeDates).map((r) => (
                  <li key={r.from} className="flex flex-wrap items-center justify-between gap-2">
                    <Badge variant="outline">
                      {r.from === r.to
                        ? formatIsoDate(r.from)
                        : `${formatIsoDate(r.from)} → ${formatIsoDate(r.to)}`}
                    </Badge>
                    {permissions.canWriteBookings && (
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => {
                          setPicked(null);
                          setBooking({ spotId: picked.spot.id, startsOn: r.from, endsOn: r.to });
                        }}
                      >
                        <PlusIcon aria-hidden="true" />
                        {t('actions.newBooking')}
                      </Button>
                    )}
                  </li>
                ))}
              </ul>
            )}
            <DialogFooter className="border-t pt-4">
              <Button variant="outline" onClick={() => setPicked(null)}>
                {t('actions.close')}
              </Button>
              {picked.state.state === 'free' && permissions.canWriteBookings && (
                <Button
                  onClick={() => {
                    setPicked(null);
                    setBooking({ spotId: picked.spot.id, startsOn: range.from, endsOn: range.to });
                  }}
                >
                  <PlusIcon aria-hidden="true" />
                  {t('actions.newBooking')}
                </Button>
              )}
            </DialogFooter>
          </div>
        </FormDialog>
      )}
      {booking && (
        <Suspense fallback={null}>
          <NewBookingSheet
            establishmentId={establishmentId}
            open
            onOpenChange={(v) => !v && setBooking(null)}
            defaults={{
              spotIds: [booking.spotId],
              startsOn: booking.startsOn,
              endsOn: booking.endsOn,
              slots: slot,
            }}
          />
        </Suspense>
      )}
    </div>
  );
}
