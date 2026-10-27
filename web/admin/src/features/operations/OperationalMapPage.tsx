import {
  DateRangePicker,
  EmptyState,
  Page,
  QueryState,
  SegmentedControl,
  useMediaQuery,
} from '@arenile/shared/ui/patterns';
import { Button } from '@arenile/shared/ui/button';
import { Link } from '@tanstack/react-router';
import { MapIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useRequiredEstablishmentId } from '../../establishment';
import { useOperationsDay } from './api/operations';
import { DayToolbar } from './components/DayToolbar';
import { OperationalMapView } from './components/OperationalMapView';
import { RangeMapView } from './components/RangeMapView';
import { addDays } from './lib/dates';
import { useOperationsPermissions } from './lib/permissions';
import { useDaySearch } from './lib/useDaySearch';
import { useSelectedMap } from './lib/useSelectedMap';

/**
 * "Mappa" (spec 15.1 point 2, 11.3): the operational map for any date and
 * slot, from `?date=&slot=`. Spots are coloured by status; a tap opens the
 * quick sheet with the actions; the stream keeps it live.
 */
/**
 * On wide screens the page uses the whole width (AppShell) and the drawing
 * may grow taller too, so more spots are readable without zooming.
 */
const WIDE_QUERY = '(min-width: 1280px)';
const WIDE_MAP_HEIGHT = '78vh';

export function OperationalMapPage() {
  const wide = useMediaQuery(WIDE_QUERY);
  const { t } = useTranslation('operations');
  const establishmentId = useRequiredEstablishmentId();
  const { date, slot, today, setDay } = useDaySearch();
  const permissions = useOperationsPermissions();
  const { maps, options, mapId, setMapId, geometry, hasNoMaps } = useSelectedMap(establishmentId);
  // Filter by map only when there is a choice: one cache entry with the Oggi page otherwise.
  const [mode, setMode] = useState<'day' | 'range'>('day');
  const [range, setRange] = useState<{ from: string; to: string } | null>(null);
  const rangeShown = range ?? { from: date, to: addDays(date, 6) };
  const dayMapId = options.length > 1 ? mapId : undefined;
  const day = useOperationsDay(establishmentId, date, slot, dayMapId);

  return (
    <Page title={t('map.title')} description={t('map.description')}>
      <DayToolbar
        date={date}
        slot={slot}
        today={today}
        onChange={setDay}
        maps={options}
        mapId={mapId}
        onMapChange={setMapId}
      />
      {/* Spec 30.4: a range shows free, free in part ("N giorni su M") and taken spots. */}
      <div className="flex flex-wrap items-center gap-2">
        <SegmentedControl
          label={t('range.mode')}
          value={mode}
          onValueChange={(v) => setMode(v as 'day' | 'range')}
          options={[
            { value: 'day', label: t('range.modeDay') },
            { value: 'range', label: t('range.modeRange') },
          ]}
        />
        {mode === 'range' && (
          <DateRangePicker
            aria-label={t('range.period')}
            value={rangeShown}
            onChange={(v) => v && setRange({ from: v.from, to: v.to ?? v.from })}
            className="sm:w-72"
          />
        )}
      </div>
      {hasNoMaps ? (
        <EmptyState
          icon={MapIcon}
          title={t('map.noMapsTitle')}
          description={t('map.noMapsDescription')}
          action={
            permissions.canConfigure ? (
              <Button asChild>
                <Link to="/e/$establishmentId/maps" params={{ establishmentId }}>
                  {t('map.openEditor')}
                </Link>
              </Button>
            ) : undefined
          }
        />
      ) : (
        <>
          <QueryState
            isLoading={maps.isLoading || geometry.isLoading || day.isLoading}
            error={maps.error ?? geometry.error ?? day.error}
            onRetry={() => {
              void maps.refetch();
              void geometry.refetch();
              void day.refetch();
            }}
            skeleton={<div className="h-[65vh] w-full animate-pulse rounded-lg bg-muted" />}
          />
          {mode === 'range' && geometry.data && (
            <RangeMapView
              establishmentId={establishmentId}
              geometry={geometry.data}
              range={rangeShown}
              slot={slot}
              mapId={dayMapId}
              permissions={permissions}
              height={wide ? WIDE_MAP_HEIGHT : undefined}
            />
          )}
          {mode === 'day' && geometry.data && day.data && (
            <OperationalMapView
              establishmentId={establishmentId}
              geometry={geometry.data}
              day={day.data}
              date={date}
              slot={slot}
              permissions={permissions}
              height={wide ? WIDE_MAP_HEIGHT : undefined}
              mapId={dayMapId}
            />
          )}
        </>
      )}
    </Page>
  );
}
