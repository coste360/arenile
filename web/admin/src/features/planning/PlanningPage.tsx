import { MapLegend } from '@arenile/shared/map';
import { Button } from '@arenile/shared/ui/button';
import { ButtonGroup } from '@arenile/shared/ui/button-group';
import { DatePicker, EmptyState, Page, QueryState } from '@arenile/shared/ui/patterns';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import { useNavigate } from '@tanstack/react-router';
import { CalendarDaysIcon, ChevronLeftIcon, ChevronRightIcon, PlusIcon } from 'lucide-react';
import { lazy, Suspense, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useCurrentEstablishment, useRequiredEstablishmentId } from '../../establishment';
import { addDays, todayIn } from '../operations/lib/dates';
import { useOperationsPermissions } from '../operations/lib/permissions';
import { useDaySearch } from '../operations/lib/useDaySearch';
import { usePlanning, usePlanningZones } from './api';
import { PlanningGrid } from './components/PlanningGrid';
import { PlanningMoveDialog, type PlanningMove } from './components/PlanningMoveDialog';
import { PLANNING_LEGEND } from './lib/colors';
import {
  DEFAULT_METRICS,
  fitCellWidth,
  buildLayout,
  type MoveProposal,
  type SelectionDefaults,
} from './lib/grid';

const NewBookingSheet = lazy(() =>
  import('../bookings').then((m) => ({ default: m.NewBookingSheet })),
);

const RANGE_LENGTHS = [14, 21, 31] as const;
type RangeLength = (typeof RANGE_LENGTHS)[number];

/**
 * "Calendario" (spec 15.1 point 4): spot × day planning to place long
 * stays. 14–31 days from a start date; bars are bookings, holds, season
 * passes, released days and blocks. Dragging a booking proposes a PATCH
 * (dates and/or spot) confirmed in a dialog with If-Match; selecting free
 * cells opens the new-booking sheet prefilled. Live via the SSE stream.
 */
export function PlanningPage() {
  const { t } = useTranslation('planning');
  const establishmentId = useRequiredEstablishmentId();
  const navigate = useNavigate();
  const { establishment } = useCurrentEstablishment();
  const today = todayIn(establishment?.timezone);
  const permissions = useOperationsPermissions();
  const [gridBox, gridWidth] = useElementWidth<HTMLDivElement>();
  // The start day lives in the URL (?date=), like the map and the day view.
  const { date: start, setDay } = useDaySearch();
  const setFrom = (date: string | undefined) => setDay({ date });
  const [length, setLength] = useState<RangeLength>(14);
  const end = addDays(start, length - 1);

  const planning = usePlanning(establishmentId, start, end);
  const zones = usePlanningZones(establishmentId);
  const layout = useMemo(
    () =>
      planning.data
        ? buildLayout(planning.data, zones.data?.names ?? new Map(), zones.data?.order)
        : null,
    [planning.data, zones.data],
  );

  const [move, setMove] = useState<PlanningMove | null>(null);
  const [newBooking, setNewBooking] = useState<SelectionDefaults | { none: true } | null>(null);

  const onMove = (p: MoveProposal) => {
    if (!layout || !p.bar.entry.booking_id) return;
    const src = layout.rows[p.bar.row];
    const dst = layout.rows[p.targetRow];
    setMove({
      bookingId: p.bar.entry.booking_id,
      code: p.bar.entry.code ?? '',
      label: p.bar.entry.label,
      dayDelta: p.dayDelta,
      sourceSpotId: p.sourceSpotId,
      sourceSpotLabel: src?.kind === 'spot' ? src.spot.label : '',
      targetSpotId: p.targetSpotId,
      targetSpotLabel: dst?.kind === 'spot' ? dst.spot.label : '',
    });
  };

  const openBooking = (bookingId: string) =>
    void navigate({
      to: '/e/$establishmentId/prenotazioni/$bookingId',
      params: { establishmentId, bookingId },
    });

  return (
    <Page
      title={t('title')}
      description={t('description')}
      actions={
        permissions.canWriteBookings && (
          <Button onClick={() => setNewBooking({ none: true })}>
            <PlusIcon aria-hidden="true" />
            {t('newBooking')}
          </Button>
        )
      }
    >
      <div
        role="toolbar"
        aria-label={t('toolbar.label')}
        className="flex flex-wrap items-center gap-2"
      >
        <ButtonGroup>
          <Button
            variant="outline"
            size="icon"
            aria-label={t('toolbar.previous', { count: length })}
            onClick={() => setFrom(addDays(start, -length))}
          >
            <ChevronLeftIcon aria-hidden="true" />
          </Button>
          <Button variant="outline" disabled={start === today} onClick={() => setFrom(undefined)}>
            {t('toolbar.today')}
          </Button>
          <Button
            variant="outline"
            size="icon"
            aria-label={t('toolbar.next', { count: length })}
            onClick={() => setFrom(addDays(start, length))}
          >
            <ChevronRightIcon aria-hidden="true" />
          </Button>
        </ButtonGroup>
        <DatePicker
          value={start}
          onChange={(v) => v && setFrom(v)}
          aria-label={t('toolbar.from')}
          className="sm:w-44"
        />
        <ToggleGroup
          type="single"
          variant="outline"
          value={String(length)}
          onValueChange={(v) => v && setLength(Number(v) as RangeLength)}
          aria-label={t('toolbar.length')}
        >
          {RANGE_LENGTHS.map((n) => (
            <ToggleGroupItem key={n} value={String(n)}>
              {t('toolbar.days', { count: n })}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      </div>

      <QueryState
        isLoading={planning.isLoading}
        error={planning.error}
        onRetry={() => void planning.refetch()}
        skeleton={<Skeleton className="h-[70vh] w-full" />}
      />
      {layout && layout.rows.length === 0 && (
        <EmptyState
          icon={CalendarDaysIcon}
          title={t('emptyTitle')}
          description={t('emptyDescription')}
        />
      )}
      {layout && layout.rows.length > 0 && (
        <>
          <div ref={gridBox}>
            <PlanningGrid
              layout={layout}
              metrics={{
                ...DEFAULT_METRICS,
                cellWidth: fitCellWidth(
                  gridWidth,
                  length,
                  DEFAULT_METRICS.labelWidth,
                  length === 31 ? 32 : DEFAULT_METRICS.cellWidth,
                ),
              }}
              today={today}
              canMove={permissions.canWriteBookings}
              canCreate={permissions.canWriteBookings}
              onMove={onMove}
              onSelect={setNewBooking}
              onOpenBooking={openBooking}
            />
          </div>
          <MapLegend statuses={PLANNING_LEGEND} />
        </>
      )}

      {move && (
        <PlanningMoveDialog
          establishmentId={establishmentId}
          move={move}
          onClose={() => setMove(null)}
          onReload={() => void planning.refetch()}
        />
      )}
      {newBooking && (
        <Suspense fallback={null}>
          <NewBookingSheet
            establishmentId={establishmentId}
            open
            onOpenChange={(v) => !v && setNewBooking(null)}
            defaults={'none' in newBooking ? undefined : { ...newBooking, slots: 'FULL' }}
          />
        </Suspense>
      )}
    </Page>
  );
}

/**
 * Width of an element, tracked with ResizeObserver (0 where unsupported).
 * A callback ref, so an element mounted later (after loading) is observed.
 */
function useElementWidth<T extends HTMLElement>() {
  const [el, setEl] = useState<T | null>(null);
  const [width, setWidth] = useState(0);
  useEffect(() => {
    if (!el || typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver(([entry]) => {
      if (entry) setWidth(Math.round(entry.contentRect.width));
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, [el]);
  return [setEl, width] as const;
}
