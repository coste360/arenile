import {
  BeachMap,
  INDICATOR_CLASSES,
  MapLegend,
  SPOT_INDICATORS,
  SpotList,
  MAP_LABEL_MODES,
  type MapGeometry,
  type MapLabelMode,
  type MapSpot,
} from '@arenile/shared/map';
import { cn } from '@arenile/shared/lib/utils';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from '@arenile/shared/ui/input-group';
import { useMe } from '@arenile/shared/api';
import { Tabs, TabsContent } from '@arenile/shared/ui/tabs';
import { ResponsiveTabsList, SegmentedControl } from '@arenile/shared/ui/patterns';
import { ArrowRightLeftIcon, SearchIcon, XIcon } from 'lucide-react';
import { lazy, Suspense, useCallback, useDeferredValue, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { occupantCaptions, occupantFullName, useLabelMode } from '../lib/occupant';
import type { OperationsPermissions } from '../lib/permissions';
import {
  OPERATIONAL_LEGEND,
  indicatorsBySpot,
  isBookable,
  searchSpots,
  slotStatus,
  statusesBySpot,
  type OperationalSpot,
  type OperationsDay,
  type SlotChoice,
} from '../lib/status';
import { useTicketAssignment } from '../../daily-sale';
import { MoveDialog } from './MoveDialog';
import { SpotSheet } from './SpotSheet';

// The booking sheet belongs to the bookings feature: load it on first use.
const NewBookingSheet = lazy(() =>
  import('../../bookings').then((m) => ({ default: m.NewBookingSheet })),
);
// Quick walk-in sale ("Vendi giornata", daily-sale feature, ADR 0020).
const VenditaGiornalieraSheet = lazy(() =>
  import('../../daily-sale').then((m) => ({ default: m.VenditaGiornalieraSheet })),
);

export interface OperationalMapViewProps {
  establishmentId: string;
  geometry: MapGeometry;
  day: OperationsDay | undefined;
  date: string;
  slot: SlotChoice;
  permissions: OperationsPermissions;
  /** CSS height of the drawing (default 65vh). */
  height?: string;
  className?: string;
}

interface NewBookingDefaults {
  spotIds: string[];
  startsOn: string;
  endsOn: string;
  slots: SlotChoice;
}

/**
 * Operational map of one day and slot (spec 11.3): status colours with an
 * always-visible legend, overlay indicators, search highlighting, the quick
 * sheet of a spot and the move flow (pick the booking, then the free target
 * spot, then confirm a dry-run preview). A list view grouped by row is the
 * alternative for screen readers and small screens. Live updates come from
 * the SSE stream invalidating the day query; this component only renders.
 */
export function OperationalMapView({
  establishmentId,
  geometry,
  day,
  date,
  slot,
  permissions,
  height = '65vh',
  className,
}: OperationalMapViewProps) {
  const { t } = useTranslation('operations');
  const [view, setView] = useState('map');
  const [query, setQuery] = useState('');
  const deferredQuery = useDeferredValue(query);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);
  const [moveSource, setMoveSource] = useState<OperationalSpot | null>(null);
  const [moveTarget, setMoveTarget] = useState<{ spotId: string; label: string } | null>(null);
  const [newBooking, setNewBooking] = useState<NewBookingDefaults | null>(null);
  const [dailySaleSpot, setDailySaleSpot] = useState<string | null>(null);

  const spots = useMemo(() => day?.spots ?? [], [day]);
  const byId = useMemo(() => new Map(spots.map((s) => [s.spot_id, s])), [spots]);
  const statuses = useMemo(() => statusesBySpot(spots, slot), [spots, slot]);
  const indicators = useMemo(() => indicatorsBySpot(spots), [spots]);
  // Who occupies each spot (spec 11.3), only for users who may see customers.
  const canSeeCustomers = permissions.canSeeCustomers;
  const captions = useMemo(
    () => occupantCaptions(spots, slot, t, canSeeCustomers),
    [spots, slot, t, canSeeCustomers],
  );
  const me = useMe();
  const [labelMode, setLabelMode] = useLabelMode(me.data?.user?.id);
  const matches = useMemo(() => searchSpots(spots, deferredQuery), [spots, deferredQuery]);
  // "Da assegnare": tap a zone-first ticket, then a free spot of its zone.
  const assignment = useTicketAssignment({
    establishmentId,
    date,
    slot,
    spots,
    canWrite: permissions.canWriteBookings,
  });
  const selectedIds = useMemo(
    () => [selectedId, moveSource?.spot_id].filter((v): v is string => !!v),
    [selectedId, moveSource],
  );

  const spotDescription = useCallback(
    (spot: MapSpot) => {
      const op = byId.get(spot.id);
      const b = op?.booking;
      if (!b) return undefined;
      const parts: string[] = [];
      const name = canSeeCustomers ? occupantFullName(b, t) : undefined;
      if (name) parts.push(captions[spot.id] ? t('occupant.aria', { name }) : name);
      for (const kind of indicators[spot.id] ?? []) parts.push(t(`indicator.${kind}`));
      return parts.join(', ');
    },
    [byId, indicators, t, canSeeCustomers, captions],
  );

  const activate = useCallback(
    (spot: MapSpot) => {
      const op = byId.get(spot.id);
      if (!moveSource && assignment.handleSpot(spot.id)) return;
      if (moveSource) {
        if (spot.id === moveSource.spot_id) return;
        if (!op || !isBookable(slotStatus(op, slot))) return;
        setMoveTarget({ spotId: spot.id, label: spot.label });
        return;
      }
      setSelectedId(spot.id);
      setSheetOpen(true);
    },
    [byId, moveSource, slot, assignment],
  );

  const startMove = (spot: OperationalSpot) => {
    setSheetOpen(false);
    setMoveSource(spot);
  };
  const endMove = () => {
    setMoveSource(null);
    setMoveTarget(null);
  };
  const openDailySale = (spot: OperationalSpot) => {
    setSheetOpen(false);
    setDailySaleSpot(spot.spot_id);
  };
  const openNewBooking = (spot: OperationalSpot) => {
    setSheetOpen(false);
    setNewBooking({ spotIds: [spot.spot_id], startsOn: date, endsOn: date, slots: slot });
  };

  const selected = selectedId ? (byId.get(selectedId) ?? null) : null;
  const searching = deferredQuery.trim().length > 0;

  return (
    <div className={cn('space-y-3', className)}>
      <div className="flex flex-wrap items-center gap-3">
        <InputGroup className="w-full sm:max-w-sm">
          <InputGroupAddon>
            <SearchIcon aria-hidden="true" />
          </InputGroupAddon>
          <InputGroupInput
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t('search.placeholder')}
            aria-label={t('search.label')}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && matches[0]) {
                setSelectedId(matches[0]);
                setSheetOpen(true);
              }
            }}
          />
          {query && (
            <InputGroupAddon align="inline-end">
              <InputGroupButton
                size="icon-xs"
                aria-label={t('search.clear')}
                onClick={() => setQuery('')}
              >
                <XIcon aria-hidden="true" />
              </InputGroupButton>
            </InputGroupAddon>
          )}
        </InputGroup>
        <p role="status" aria-live="polite" className="text-sm text-muted-foreground">
          {searching ? t('search.results', { count: matches.length }) : ''}
        </p>
      </div>

      {moveSource && (
        <Alert>
          <ArrowRightLeftIcon aria-hidden="true" />
          <AlertTitle>{t('move.pickTitle', { label: moveSource.label })}</AlertTitle>
          <AlertDescription className="flex flex-wrap items-center gap-3">
            <span>{t('move.pickDescription')}</span>
            <Button size="sm" variant="outline" onClick={endMove}>
              {t('actions.cancel')}
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {!moveSource && assignment.panel}

      {day?.closed && (
        <Alert>
          <AlertTitle>{t('map.closedTitle')}</AlertTitle>
          <AlertDescription>{t('map.closedDescription')}</AlertDescription>
        </Alert>
      )}

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
        <TabsContent value="map" className="space-y-2">
          {/* Legend first: readable before scrolling past the drawing (spec 11.3). */}
          <div className="flex flex-wrap items-start gap-x-6 gap-y-2">
            <MapLegend statuses={OPERATIONAL_LEGEND} />
            <div className="space-y-1.5">
              <p className="text-xs font-medium text-muted-foreground">{t('indicator.legend')}</p>
              <ul className="flex flex-wrap gap-3 text-xs">
                {SPOT_INDICATORS.map((kind) => (
                  <li key={kind} className="flex items-center gap-1.5" data-indicator-legend={kind}>
                    <span
                      aria-hidden="true"
                      className={cn('size-2.5 rounded-full', INDICATOR_CLASSES[kind].bg)}
                    />
                    {t(`indicator.${kind}`)}
                  </li>
                ))}
              </ul>
            </div>
          </div>
          <BeachMap
            geometry={geometry}
            statuses={statuses}
            selectedIds={selectedIds}
            highlightedIds={assignment.active ? assignment.highlightIds : matches}
            spotIndicators={indicators}
            onSpotActivate={activate}
            spotDescription={spotDescription}
            spotCaptions={canSeeCustomers ? captions : undefined}
            labelMode={labelMode}
            toolbar={
              canSeeCustomers ? (
                <div className="flex min-w-0 items-center gap-2">
                  <span className="hidden text-xs font-medium text-muted-foreground sm:inline">
                    {t('labels.label')}
                  </span>
                  <SegmentedControl
                    label={t('labels.label')}
                    value={labelMode}
                    onValueChange={(v) => setLabelMode(v as MapLabelMode)}
                    options={MAP_LABEL_MODES.map((m) => ({ value: m, label: t(`labels.${m}`) }))}
                    collapseOnNarrow
                    className="sm:w-auto"
                  />
                </div>
              ) : undefined
            }
            legendStatuses={OPERATIONAL_LEGEND}
            showLegend={false}
            title={t('map.aria', { name: geometry.name ?? '', date, slot: t(`slot.${slot}`) })}
            height={height}
          />
        </TabsContent>
        <TabsContent value="list">
          <SpotList
            spots={geometry.spots}
            statuses={statuses}
            selectedIds={selectedIds}
            onSpotActivate={activate}
            spotDescription={spotDescription}
            activateAny
          />
        </TabsContent>
      </Tabs>

      <SpotSheet
        establishmentId={establishmentId}
        spot={selected}
        date={date}
        slot={slot}
        permissions={permissions}
        open={sheetOpen}
        onOpenChange={setSheetOpen}
        onNewBooking={openNewBooking}
        onStartMove={startMove}
        onDailySale={openDailySale}
      />
      {moveSource && moveTarget && (
        <MoveDialog
          establishmentId={establishmentId}
          source={moveSource}
          target={moveTarget}
          date={date}
          open
          onOpenChange={(v) => !v && setMoveTarget(null)}
          onDone={endMove}
        />
      )}
      {dailySaleSpot && (
        <Suspense fallback={null}>
          <VenditaGiornalieraSheet
            establishmentId={establishmentId}
            open
            onOpenChange={(v) => !v && setDailySaleSpot(null)}
            date={date}
            spotId={dailySaleSpot}
            slots={slot}
          />
        </Suspense>
      )}
      {newBooking && (
        <Suspense fallback={null}>
          <NewBookingSheet
            establishmentId={establishmentId}
            open
            onOpenChange={(v) => !v && setNewBooking(null)}
            defaults={newBooking}
          />
        </Suspense>
      )}
    </div>
  );
}
